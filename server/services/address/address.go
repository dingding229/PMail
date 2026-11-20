package address

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"

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

func ToView(row models.MailboxAddress) View {
	d, _ := domain.GetByID(row.DomainID)
	return View{MailboxAddress: row, Domain: d.Name, Address: row.LocalPart + "@" + d.Name}
}

func List(userID int) ([]View, error) {
	var rows []models.MailboxAddress
	if err := db.Instance.Where("user_id = ?", userID).Desc("is_primary").Asc("id").Find(&rows); err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToView(row))
	}
	return out, nil
}

func primary(userID int) (models.MailboxAddress, error) {
	var row models.MailboxAddress
	found, err := db.Instance.Where("user_id = ? AND is_primary = ?", userID, true).Get(&row)
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
	found, err := db.Instance.Where("local_part = ? AND domain_id = ?", local, d.ID).Get(&row)
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
	ok, err := tx.Where("local_part = ? AND domain_id = ?", row.LocalPart, row.DomainID).Exist(&models.MailboxAddress{})
	if err != nil {
		return err
	}
	if ok {
		return ErrExists
	}
	_, err = tx.Insert(row)
	return err
}

func Create(userID, domainID int, raw string, canSend bool) (View, error) {
	local, err := NormalizeLocal(raw)
	if err != nil {
		return View{}, err
	}
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	if _, ok := domain.GetByID(domainID); !ok {
		return View{}, domain.ErrNotFound
	}
	var row models.MailboxAddress
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		ok, err := tx.ID(userID).Exist(&models.User{})
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		row = models.MailboxAddress{LocalPart: local, DomainID: domainID, UserID: userID, CanSend: canSend}
		if err := insert(tx, &row); err != nil {
			return err
		}
		// Xorm's insert can omit zero bool fields with defaults. Persist explicit false.
		if !canSend {
			_, err = tx.ID(row.ID).Cols("can_send").Update(&row)
		}
		return err
	})
	if err != nil {
		return View{}, err
	}
	return ToView(row), nil
}

func owned(tx *xorm.Session, id, actor int, admin bool) (models.MailboxAddress, error) {
	var row models.MailboxAddress
	if !admin {
		tx.Where("user_id = ?", actor)
	}
	found, err := tx.ID(id).Get(&row)
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
		_, err = tx.ID(id).Delete(&models.MailboxAddress{})
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
		if row.IsPrimary && !canSend {
			return ErrPrimary
		}
		_, err = tx.ID(id).Cols("can_send").Update(&models.MailboxAddress{CanSend: canSend})
		return err
	})
}
