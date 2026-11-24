package address

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"xorm.io/xorm"
)

var (
	ErrInvalid   = errors.New("invalid mailbox address")
	ErrNotFound  = errors.New("mailbox address or user not found")
	ErrForbidden = errors.New("address is not an enabled sender for this user")
	ErrExists    = errors.New("mailbox address already exists")
	ErrPrimary   = errors.New("a primary address cannot be deleted or made receive-only")
)

type View struct {
	models.MailboxAddress `xorm:"extends"`
	Domain                string `json:"domain"`
	Address               string `json:"address"`
}

func NormalizeLocal(raw string) (string, error) {
	local := strings.ToLower(strings.TrimSpace(raw))
	if local == "" || len(local) > 64 || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") || strings.ContainsAny(local, "@\r\n\t \"()<>[]\\,;:") {
		return "", ErrInvalid
	}
	addr, err := mail.ParseAddress(local + "@example.invalid")
	if err != nil || addr.Address != local+"@example.invalid" {
		return "", ErrInvalid
	}
	return local, nil
}

func Split(raw string) (string, string, error) {
	local, host, ok := strings.Cut(strings.TrimSpace(raw), "@")
	if !ok {
		return "", "", ErrInvalid
	}
	local, err := NormalizeLocal(local)
	if err != nil {
		return "", "", err
	}
	host, err = domain.Normalize(host)
	if err != nil {
		return "", "", ErrInvalid
	}
	return local, host, nil
}

// Backfill only users without any addresses. Deleted aliases and changed primaries
// are never recreated on restart. Call after domain bootstrap and admin creation.
func Backfill() error {
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	query := "SELECT u.id, u.account FROM " + db.Instance.Quote("user") + " u WHERE NOT EXISTS (SELECT 1 FROM mailbox_address a WHERE a.user_id = u.id) ORDER BY u.id"
	var missing []models.User
	if err := db.Instance.SQL(query).Find(&missing); err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		for _, u := range missing {
			if err := Seed(tx, u); err != nil {
				return fmt.Errorf("backfill mailbox addresses for user %d: %w", u.ID, err)
			}
		}
		return nil
	})
}

// Seed must be called in a transaction with MailboxRegistry held.
func Seed(tx *xorm.Session, user models.User) error {
	local, err := NormalizeLocal(user.Account)
	if err != nil {
		return err
	}
	domains := domain.List()
	if len(domains) == 0 {
		return errors.New("no mail domains available")
	}
	for _, d := range domains {
		row := models.MailboxAddress{LocalPart: local, DomainID: d.ID, UserID: user.ID, IsPrimary: d.IsDefault, CanSend: true}
		if err := insert(tx, &row); err != nil {
			return err
		}
	}
	return nil
}

// SeedPrimary adds just the default address of a newly registered user.
// The caller owns the transaction and holds MailboxRegistry.
func SeedPrimary(tx *xorm.Session, user models.User) error {
	d := domain.Default()
	if d.ID == 0 {
		return domain.ErrNotFound
	}
	return insert(tx, &models.MailboxAddress{LocalPart: user.Account, DomainID: d.ID, UserID: user.ID, IsPrimary: true, CanSend: true})
}

func ToView(row models.MailboxAddress) View {
	d, _ := domain.GetByID(row.DomainID)
	return makeView(row, d.Name)
}

func makeView(row models.MailboxAddress, name string) View {
	if row.Status != models.AddressActive {
		row.CanSend = false
	}
	return View{MailboxAddress: row, Domain: name, Address: row.LocalPart + "@" + name}
}

func List(userID int) ([]View, error) {
	rows, err := ListByUsers([]int{userID})
	return rows[userID], err
}

// ListByUsers keeps address and domain lookup batched and preserves the
// primary-first, then ID ordering used by the single-user view.
func ListByUsers(ids []int) (map[int][]View, error) {
	if db.Instance == nil {
		return nil, errors.New("database is not initialized")
	}
	out := make(map[int][]View, len(ids))
	for _, id := range ids {
		out[id] = []View{}
	}
	domainNames := map[int]string{}
	for start := 0; start < len(ids); start += 400 {
		var rows []models.MailboxAddress
		if err := db.Instance.In("user_id", ids[start:min(start+400, len(ids))]).Desc("is_primary").Asc("id").Find(&rows); err != nil {
			return nil, err
		}
		var missing []int
		for _, a := range rows {
			if _, ok := domainNames[a.DomainID]; !ok {
				domainNames[a.DomainID] = ""
				missing = append(missing, a.DomainID)
			}
		}
		for n := 0; n < len(missing); n += 400 {
			var ds []models.Domain
			if err := db.Instance.Cols("id", "name").In("id", missing[n:min(n+400, len(missing))]).Find(&ds); err != nil {
				return nil, err
			}
			for _, d := range ds {
				domainNames[d.ID] = d.Name
			}
		}
		for _, a := range rows {
			name := domainNames[a.DomainID]
			out[a.UserID] = append(out[a.UserID], makeView(a, name))
		}
	}
	return out, nil
}

func primary(userID int) (models.MailboxAddress, error) {
	var row models.MailboxAddress
	found, err := db.Instance.Where("user_id = ? AND is_primary = ? AND status=?", userID, true, models.AddressActive).Get(&row)
	if err != nil {
		return row, err
	}
	if !found {
		return row, ErrNotFound
	}
	return row, nil
}

func Primary(userID int) (View, error) {
	row, err := primary(userID)
	if err != nil {
		return View{}, err
	}
	if _, ok := domain.GetByID(row.DomainID); !ok {
		return View{}, ErrNotFound
	}
	return ToView(row), nil
}

func find(raw string) (models.MailboxAddress, bool, error) {
	local, host, err := Split(raw)
	if err != nil {
		return models.MailboxAddress{}, false, nil
	}
	d, ok := domain.Get(host)
	if !ok {
		return models.MailboxAddress{}, false, nil
	}
	var row models.MailboxAddress
	found, err := db.Instance.Where("local_part = ? AND domain_id = ? AND reservation_key=0 AND status=?", local, d.ID, models.AddressActive).Get(&row)
	return row, found, err
}

func Resolve(raw string) (models.User, bool, error) {
	row, found, err := find(raw)
	if !found || err != nil {
		return models.User{}, false, err
	}
	var user models.User
	found, err = db.Instance.ID(row.UserID).Get(&user)
	return user, found, err
}

func OwnerForLogin(login string) (models.User, bool, error) {
	login = strings.TrimSpace(login)
	if strings.Contains(login, "@") {
		return Resolve(login)
	}
	var u models.User
	// Prefer the exact legacy account before lower-case fallback.
	found, err := db.Instance.Where("account = ?", login).Get(&u)
	if err != nil || found {
		return u, found, err
	}
	found, err = db.Instance.Where("LOWER(account) = ?", strings.ToLower(login)).Get(&u)
	return u, found, err
}

func SendAddress(userID int, value string) (string, error) {
	if value == "" {
		p, err := Primary(userID)
		if err != nil {
			return "", err
		}
		if !p.CanSend {
			return "", ErrForbidden
		}
		return p.Address, nil
	}
	row, ok, err := find(value)
	if err != nil {
		return "", err
	}
	if !ok || row.UserID != userID || !row.CanSend {
		return "", ErrForbidden
	}
	return ToView(row).Address, nil
}

func insert(tx *xorm.Session, row *models.MailboxAddress) error {
	if row.Status == "" {
		row.Status = models.AddressActive
	}
	ok, err := tx.Where("local_part = ? AND domain_id = ? AND reservation_key=0", row.LocalPart, row.DomainID).Exist(&models.MailboxAddress{})
	if err != nil {
		return err
	}
	if ok {
		return ErrExists
	}
	_, err = tx.Insert(row)
	if db.IsUniqueViolation(err) {
		return ErrExists
	}
	return err
}

func Create(ctx context.Context, userID, domainID int, raw string, canSend, bypassApproval bool) (View, error) {
	local, err := NormalizeLocal(raw)
	if err != nil {
		return View{}, err
	}
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	var row models.MailboxAddress
	var d models.Domain
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		var err error
		d, err = lockDomain(tx, domainID)
		if err != nil {
			return err
		}
		if err = lockApplicant(tx, userID); err != nil {
			return err
		}
		row = models.MailboxAddress{LocalPart: local, DomainID: domainID, UserID: userID, CanSend: canSend, Status: models.AddressActive}
		if !bypassApproval {
			required, err := approvalRequired(tx)
			if err != nil {
				return err
			}
			if required {
				n, err := tx.Where("user_id=? AND status=?", userID, models.AddressPending).Count(&models.MailboxAddress{})
				if err != nil {
					return err
				}
				if n >= pendingLimit {
					return ErrPendingLimit
				}
				requested := time.Now().UTC()
				row.Status, row.RequestedAt = models.AddressPending, &requested
			}
		}
		if err := insert(tx, &row); err != nil {
			return err
		}
		// Preserve an explicit receive-only setting even when Xorm applies a default.
		if !canSend {
			_, err = tx.ID(row.ID).Cols("can_send").Update(&row)
		}
		return err
	})
	if err != nil {
		return View{}, err
	}
	return makeView(row, d.Name), nil
}

// SMTP administrators retain their existing relay permissions, but cannot
// submit from an inactive local address when there is no active claimant.
func CheckSubmission(userID int, raw string, admin bool) error {
	if !admin {
		_, err := SendAddress(userID, raw)
		return err
	}
	local, host, err := Split(raw)
	if err != nil {
		return nil
	}
	d, ok := domain.Get(host)
	if !ok {
		return nil
	}
	var row models.MailboxAddress
	ok, err = db.Instance.Where("local_part=? AND domain_id=?", local, d.ID).Asc("reservation_key").Cols("status").Get(&row)
	if err == nil && ok && row.Status != models.AddressActive {
		return ErrInactive
	}
	return err
}

func owned(tx *xorm.Session, id, actor int, admin bool) (models.MailboxAddress, error) {
	var row models.MailboxAddress
	if !admin {
		tx.Where("user_id = ?", actor)
	}
	found, err := forUpdate(tx.ID(id)).Get(&row)
	if err != nil {
		return row, err
	}
	if !found {
		return row, ErrNotFound
	}
	return row, nil
}

func Delete(id, actor int, admin bool) error {
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		row, err := owned(tx, id, actor, admin)
		if err != nil {
			return err
		}
		if row.IsPrimary {
			return ErrPrimary
		}
		if _, err = tx.ID(id).Delete(&models.MailboxAddress{}); err != nil {
			return err
		}
		_, err = tx.Where("address_id=?", id).Delete(&models.UserEmailAddress{})
		return err
	})
}

func SetPrimary(id, actor int, admin bool) error {
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		row, err := owned(tx, id, actor, admin)
		if err != nil {
			return err
		}
		if row.Status != models.AddressActive {
			return ErrInactive
		}
		if row.IsPrimary {
			return nil
		}
		if !row.CanSend {
			return ErrForbidden
		}
		if _, err := tx.Table(&models.MailboxAddress{}).Where("user_id = ? AND is_primary = ?", row.UserID, true).Update(map[string]any{"is_primary": false}); err != nil {
			return err
		}
		_, err = tx.ID(id).Cols("is_primary").Update(&models.MailboxAddress{IsPrimary: true})
		return err
	})
}

func SetCanSend(id, actor int, admin, canSend bool) error {
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		row, err := owned(tx, id, actor, admin)
		if err != nil {
			return err
		}
		if row.Status != models.AddressActive {
			return ErrInactive
		}
		if row.IsPrimary && !canSend {
			return ErrPrimary
		}
		_, err = tx.ID(id).Cols("can_send").Update(&models.MailboxAddress{CanSend: canSend})
		return err
	})
}
