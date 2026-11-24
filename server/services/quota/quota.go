package quota

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/services/maintenance"
	log "github.com/sirupsen/logrus"
	"xorm.io/builder"
	"xorm.io/xorm"
)

const MaxMB = math.MaxInt64 >> 20

var ErrExceeded = errors.New("mailbox storage quota exceeded")
var ErrUnavailable = errors.New("recipient mailbox unavailable")

type usage struct {
	UserID int   `xorm:"user_id"`
	Bytes  int64 `xorm:"used_bytes"`
}

const sumQuery = "SELECT COALESCE(SUM(e.size),0) FROM user_email ue JOIN email e ON e.id=ue.email_id WHERE ue.user_id=?"

// Usage counts retained links, including Sent and Trash. An uninitialized
// account uses the legacy exact sum until its first mutation or reconciliation.
func Usage(ids []int) (map[int]int64, error) {
	out := make(map[int]int64, len(ids))
	for start := 0; start < len(ids); start += 400 {
		var users []models.User
		if err := db.Instance.Cols("id", "used_bytes").In("id", ids[start:min(start+400, len(ids))]).Find(&users); err != nil {
			return nil, err
		}
		var missing []int
		for _, u := range users {
			if u.UsedBytes == nil {
				missing = append(missing, u.ID)
			} else {
				out[u.ID] = *u.UsedBytes
			}
		}
		if len(missing) == 0 {
			continue
		}
		where, args, err := builder.ToSQL(builder.In("ue.user_id", missing))
		if err != nil {
			return nil, err
		}
		var rows []usage
		if err = db.Instance.SQL("SELECT ue.user_id,COALESCE(SUM(e.size),0) AS used_bytes FROM user_email ue JOIN email e ON e.id=ue.email_id WHERE "+where+" GROUP BY ue.user_id", args...).Find(&rows); err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.UserID] = r.Bytes
		}
	}
	return out, nil
}

func exceeds(mb, used, size int64) bool {
	if mb <= 0 {
		return false
	}
	limit := min(mb, int64(MaxMB)) << 20
	return used >= limit || size > limit-used
}

// Check is an RCPT preflight only; writes check once more under the user lock.
func Check(user models.User, size int64) error {
	if user.Disabled != 0 {
		return ErrUnavailable
	}
	if user.QuotaMB <= 0 {
		return nil
	}
	use, err := Usage([]int{user.ID})
	if err != nil {
		return err
	}
	if exceeds(user.QuotaMB, use[user.ID], size) {
		return ErrExceeded
	}
	return nil
}

func lockUser(tx *xorm.Session, id int) (models.User, error) {
	q := tx.ID(id).Cols("id", "disabled", "quota_mb", "used_bytes")
	if db.Instance.DriverName() != config.DBTypeSQLite {
		q.ForUpdate()
	}
	var u models.User
	ok, err := q.Get(&u)
	if err != nil {
		return u, err
	}
	if !ok {
		return u, ErrUnavailable
	}
	return u, nil
}

func reconcileUser(tx *xorm.Session, id int) (int64, error) {
	var n int64
	if _, err := tx.SQL(sumQuery, id).Get(&n); err != nil {
		return 0, err
	}
	_, err := tx.Table(&models.User{}).ID(id).Cols("used_bytes").Update(map[string]any{"used_bytes": n})
	return n, err
}

// Lock serializes all link/size mutations for an owner on PG/MySQL. For
// SQLite the single connection serializes writes. Call before altering links.
func Lock(tx *xorm.Session, id int) (models.User, error) {
	u, err := lockUser(tx, id)
	if err != nil || u.UsedBytes != nil {
		return u, err
	}
	n, err := reconcileUser(tx, id)
	if err == nil {
		u.UsedBytes = &n
	}
	return u, err
}

func LockAndCheck(tx *xorm.Session, id int, size int64) error {
	u, err := Lock(tx, id)
	if err != nil {
		return err
	}
	if u.Disabled != 0 {
		return ErrUnavailable
	}
	if exceeds(u.QuotaMB, *u.UsedBytes, size) {
		return ErrExceeded
	}
	return nil
}

// Adjust runs after the mutation, within the same owner-locked transaction.
// If a legacy inconsistency underflows, recompute from the now-current links.
func Adjust(tx *xorm.Session, id int, delta int64) error {
	if delta == 0 {
		return nil
	}
	name := db.Instance.Quote("user")
	res, err := tx.Exec("UPDATE "+name+" SET used_bytes=used_bytes+? WHERE id=? AND used_bytes IS NOT NULL AND used_bytes+?>=0", delta, id, delta)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 0 {
		return err
	}
	_, err = reconcileUser(tx, id)
	return err
}

func MessageSize(tx *xorm.Session, emailID int) (int64, error) {
	var e models.Email
	_, err := tx.ID(emailID).Cols("size").Get(&e)
	return int64(e.Size), err
}

// LockMessage serializes permanent unlink/orphan removal with a cross-owner
// legacy forward that is creating a new link to the same message.
func LockMessage(tx *xorm.Session, id int) (int64, bool, error) {
	q := tx.ID(id).Cols("size")
	if db.Instance.DriverName() != config.DBTypeSQLite {
		q.ForUpdate()
	}
	var row models.Email
	found, err := q.Get(&row)
	return int64(row.Size), found, err
}

// EnsureLink is used by legacy forwarding paths that share an existing email.
func EnsureLink(owner, emailID int, status int8, targets ...string) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if _, err := Lock(tx, owner); err != nil {
			return err
		}
		size, present, err := LockMessage(tx, emailID)
		if err != nil {
			return err
		}
		if !present {
			return ErrUnavailable
		}
		exists, err := tx.Where("user_id=? AND email_id=?", owner, emailID).Exist(&models.UserEmail{})
		if err != nil || exists {
			return err
		}
		link := models.UserEmail{UserID: owner, EmailID: emailID, Status: status, AddressAttributed: true}
		if _, err := tx.Insert(&link); err != nil {
			return err
		}
		if err := mailaddress.Add(tx, []models.UserEmail{link}, targets); err != nil {
			return err
		}
		return Adjust(tx, owner, size)
	})
}

func Reconcile(ctx context.Context) error {
	cursor := 0
	for ctx.Err() == nil {
		var us []models.User
		if err := db.Instance.Context(ctx).Cols("id").Where("id>?", cursor).Asc("id").Limit(100).Find(&us); err != nil {
			return err
		}
		if len(us) == 0 {
			return nil
		}
		for _, u := range us {
			cursor = u.ID
			if err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
				tx.Context(ctx)
				if _, err := lockUser(tx, u.ID); err != nil {
					if errors.Is(err, ErrUnavailable) {
						return nil
					}
					return err
				}
				_, err := reconcileUser(tx, u.ID)
				return err
			}); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

func RunReconciliation(ctx context.Context) {
	for ctx.Err() == nil {
		maintenance.Work(ctx, func(ctx context.Context) {
			if err := Reconcile(ctx); err != nil && ctx.Err() == nil {
				log.Errorf("quota reconciliation failed: %v", err)
			}
		})
		t := time.NewTimer(time.Hour)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

func MessageSizes(tx *xorm.Session, ids []int) (map[int]int64, error) {
	out := make(map[int]int64, len(ids))
	for start := 0; start < len(ids); start += 400 {
		var rows []models.Email
		if err := tx.Cols("id", "size").In("id", ids[start:min(start+400, len(ids))]).Find(&rows); err != nil {
			return nil, err
		}
		for _, row := range rows {
			out[row.Id] = int64(row.Size)
		}
	}
	return out, nil
}
