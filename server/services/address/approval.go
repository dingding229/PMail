package address

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"xorm.io/xorm"
)

const pendingLimit = 10

var (
	ErrPendingLimit    = errors.New("address_pending_limit")
	ErrInactive        = errors.New("address_inactive")
	ErrAlreadyReviewed = errors.New("address_already_reviewed")
	ErrRejectReason    = errors.New("address_reject_reason_invalid")
)

func forUpdate(q *xorm.Session) *xorm.Session {
	if q.Engine().DriverName() != "sqlite" {
		q.ForUpdate()
	}
	return q
}
func lockApplicant(tx *xorm.Session, uid int) error {
	ok, err := forUpdate(tx.ID(uid).Cols("id")).Get(&models.User{})
	if err == nil && !ok {
		return ErrNotFound
	}
	return err
}
func lockDomain(tx *xorm.Session, id int) (models.Domain, error) {
	var d models.Domain
	ok, err := forUpdate(tx.ID(id).Cols("id", "name")).Get(&d)
	if err != nil {
		return d, err
	}
	if !ok {
		return d, domain.ErrNotFound
	}
	if name, err := domain.Normalize(d.Name); err != nil || name != d.Name {
		return d, domain.ErrInvalid
	}
	return d, nil
}

func approvalRequired(s *xorm.Session) (bool, error) {
	var row models.AddressSettings
	ok, err := s.ID(1).Get(&row)
	return !ok || row.AddressApprovalRequired, err
}
func ApprovalRequired(ctx context.Context) (bool, error) {
	s := db.Instance.NewSession()
	defer s.Close()
	return approvalRequired(s.Context(ctx))
}
func SetApprovalRequired(ctx context.Context, required bool) error {
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		q := "INSERT INTO address_settings (id, address_approval_required) VALUES (?, ?) ON CONFLICT (id) DO UPDATE SET address_approval_required=excluded.address_approval_required"
		if tx.Engine().DriverName() == "mysql" {
			q = "INSERT INTO address_settings (id, address_approval_required) VALUES (?, ?) ON DUPLICATE KEY UPDATE address_approval_required=VALUES(address_approval_required)"
		}
		_, err := tx.Exec(q, 1, required)
		return err
	})
}

type Request struct {
	View
	Applicant string `json:"applicant"`
}
type RequestPage struct {
	Items    []Request `json:"items"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
	Total    int64     `json:"total"`
	Pending  int64     `json:"pending_address_requests"`
}

func PendingCount(ctx context.Context) (int64, error) {
	return db.Instance.Context(ctx).Where("status=?", models.AddressPending).Count(&models.MailboxAddress{})
}
func Requests(ctx context.Context, status string, page, size int) (RequestPage, error) {
	out := RequestPage{Items: []Request{}, Page: page, PageSize: size}
	if (status != models.AddressPending && status != models.AddressRejected && status != models.AddressActive) || size < 1 || size > 100 || page < 1 || page > 1000000 {
		return out, ErrInvalid
	}
	var err error
	if out.Pending, err = PendingCount(ctx); err != nil {
		return out, err
	}
	out.Total = out.Pending
	if status != models.AddressPending {
		// Active here means approved requests, not provisioned primary aliases.
		out.Total, err = db.Instance.Context(ctx).Where("status=? AND requested_at IS NOT NULL", status).Count(&models.MailboxAddress{})
		if err != nil {
			return out, err
		}
	}
	var rows []models.MailboxAddress
	if err = db.Instance.Context(ctx).Where("status=? AND requested_at IS NOT NULL", status).Desc("requested_at", "id").Limit(size, (page-1)*size).Find(&rows); err != nil {
		return out, err
	}
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	if len(ids) == 0 {
		return out, nil
	}
	var owners []models.User
	if err = db.Instance.Context(ctx).In("id", ids).Cols("id", "account").Find(&owners); err != nil {
		return out, err
	}
	names := make(map[int]string, len(owners))
	for _, u := range owners {
		names[u.ID] = u.Account
	}
	for _, r := range rows {
		out.Items = append(out.Items, Request{ToView(r), names[r.UserID]})
	}
	return out, nil
}

func PendingAddress(ctx context.Context, id int) (View, bool, error) {
	var row models.MailboxAddress
	ok, err := db.Instance.Context(ctx).ID(id).Where("status=?", models.AddressPending).Get(&row)
	if err != nil || !ok {
		return View{}, ok, err
	}
	return ToView(row), true, nil
}

func Review(ctx context.Context, id, reviewer int, approve bool, reason string) (View, error) {
	reason = strings.TrimSpace(reason)
	if !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > 200 || strings.ContainsRune(reason, 0) {
		return View{}, ErrRejectReason
	}
	if id <= 0 || reviewer <= 0 {
		return View{}, ErrInvalid
	}
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	var row models.MailboxAddress
	var d models.Domain
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		var err error
		if approve {
			var found models.MailboxAddress
			ok, e := tx.ID(id).Cols("domain_id", "status").Get(&found)
			if e != nil {
				return e
			}
			if !ok {
				return ErrNotFound
			}
			if found.Status != models.AddressPending {
				return ErrAlreadyReviewed
			}
			// Use the same domain-before-address order as creation. Do not use
			// the cached domain snapshot for a security decision.
			d, err = lockDomain(tx, found.DomainID)
			if err != nil {
				return err
			}
		}
		row, err = owned(tx, id, reviewer, true)
		if err != nil {
			return err
		}
		if row.Status != models.AddressPending {
			return ErrAlreadyReviewed
		}
		reviewed := time.Now().UTC()
		row.ReviewedAt, row.ReviewedBy = &reviewed, &reviewer
		if approve {
			if local, e := NormalizeLocal(row.LocalPart); e != nil || local != row.LocalPart {
				return ErrInvalid
			}
			if row.ReservationKey != 0 || row.DomainID != d.ID {
				return ErrExists
			}
			var collision models.MailboxAddress
			ok, e := forUpdate(tx.Where("local_part=? AND domain_id=? AND id<>? AND (reservation_key=0 OR status IN (?,?))", row.LocalPart, row.DomainID, id, models.AddressActive, models.AddressPending).Cols("id")).Get(&collision)
			if e != nil {
				return e
			}
			if ok {
				return ErrExists
			}
			row.Status, row.RejectReason = models.AddressActive, ""
		} else {
			row.Status, row.ReservationKey, row.RejectReason = models.AddressRejected, row.ID, reason
		}
		n, e := tx.ID(id).Where("status=?", models.AddressPending).Cols("status", "reservation_key", "reviewed_by", "reviewed_at", "reject_reason").Update(&row)
		if db.IsUniqueViolation(e) {
			return ErrExists
		}
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrAlreadyReviewed
		}
		return nil
	})
	if err != nil {
		return View{}, err
	}
	if approve {
		return makeView(row, d.Name), nil
	}
	return ToView(row), nil
}
