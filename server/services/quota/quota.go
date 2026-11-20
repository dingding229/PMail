package quota

import (
	"errors"
	"math"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
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

// Usage counts retained links, including Sent and Trash. Removing a link frees its bytes immediately.
func Usage(ids []int) (map[int]int64, error) {
	out := make(map[int]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	where, params, err := builder.ToSQL(builder.In("ue.user_id", ids))
	if err != nil {
		return nil, err
	}
	var rows []usage
	query := "SELECT ue.user_id, COALESCE(SUM(e.size), 0) AS used_bytes FROM user_email ue JOIN email e ON e.id = ue.email_id WHERE " + where + " GROUP BY ue.user_id"
	if err := db.Instance.SQL(query, params...).Find(&rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = row.Bytes
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

// Check is a RCPT preflight; the authoritative check happens again inside the write transaction.
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

// LockAndCheck serializes inbound quota admission for the same user across PG/MySQL transactions.
// SQLite writes already use a single connection and db.Transaction retries BUSY/LOCKED.
func LockAndCheck(tx *xorm.Session, id int, size int64) error {
	query := tx.ID(id).Cols("id", "disabled", "quota_mb")
	if db.Instance.DriverName() != config.DBTypeSQLite {
		query.ForUpdate()
	}
	var user models.User
	found, err := query.Get(&user)
	if err != nil {
		return err
	}
	if !found || user.Disabled != 0 {
		return ErrUnavailable
	}
	if user.QuotaMB <= 0 {
		return nil
	}
	var used int64
	_, err = tx.SQL("SELECT COALESCE(SUM(e.size), 0) FROM user_email ue JOIN email e ON e.id = ue.email_id WHERE ue.user_id = ?", id).Get(&used)
	if err != nil {
		return err
	}
	if exceeds(user.QuotaMB, used, size) {
		return ErrExceeded
	}
	return nil
}
