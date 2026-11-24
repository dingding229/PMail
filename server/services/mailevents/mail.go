package mailevents

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
)

type Item struct {
	ID          int    `json:"id" xorm:"id"`
	Subject     string `json:"subject"`
	FromName    string `json:"from_name"`
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Created     int64  `json:"created"`
	GroupID     int    `json:"group_id" xorm:"group_id"`
}

func CredentialsCurrent(ctx context.Context, owner int, authVersion, factorVersion int64) (bool, error) {
	return db.Instance.Context(ctx).ID(owner).Where("disabled=0 AND auth_version=? AND factor_version=?", authVersion, factorVersion).Exist(&models.User{})
}

func VisibleMax(ctx context.Context, owner int) (int64, error) {
	var id int64
	_, err := db.Instance.Context(ctx).SQL(`SELECT ue.email_id FROM user_email ue JOIN email e ON e.id=ue.email_id WHERE ue.user_id=? ORDER BY ue.email_id DESC LIMIT 1`, owner).Get(&id)
	return id, err
}

// NewMail only considers the owner's live incoming copies. Use one row per
// email (IMAP COPY can make several links) and count before limiting the preview.
// The upper bound is captured first and is exactly the ID returned in the cursor.
func NewMail(ctx context.Context, owner int, after, through int64) (int64, []Item, error) {
	items := []Item{}
	if through <= after {
		return 0, items, nil
	}
	q := `SELECT e.id, e.subject, e.from_name, e.from_address, e.` + db.Instance.Quote("to") + ` AS recipients,
		e.create_time, ue.group_id, COUNT(*) OVER () AS new_count
		FROM (
		 SELECT u.email_id, MIN(u.id) AS link_id FROM user_email u JOIN email em ON em.id=u.email_id
		 WHERE u.user_id=? AND u.email_id>? AND u.email_id<=? AND u.status=? AND em.type=?
		 AND (u.group_id IN (0,?) OR EXISTS (SELECT 1 FROM ` + db.Instance.Quote("group") + ` g WHERE g.id=u.group_id AND g.user_id=u.user_id))
		 GROUP BY u.email_id
		) candidates JOIN email e ON e.id=candidates.email_id JOIN user_email ue ON ue.id=candidates.link_id
		ORDER BY e.id DESC LIMIT 5`
	type row struct {
		Item       `xorm:"extends"`
		Recipients string
		CreateTime time.Time
		NewCount   int64
	}
	var rows []row
	if err := db.Instance.Context(ctx).SQL(q, owner, after, through, consts.EmailStatusWait, consts.EmailTypeReceive, models.INBOX).Find(&rows); err != nil {
		return 0, items, err
	}
	var count int64
	for _, r := range rows {
		count = r.NewCount
		var addresses []parsemail.User
		_ = json.Unmarshal([]byte(r.Recipients), &addresses)
		for _, addr := range addresses {
			if addr.EmailAddress != "" {
				r.ToAddress = addr.EmailAddress
				break
			}
		}
		if !r.CreateTime.IsZero() {
			r.Created = r.CreateTime.UnixMilli()
		}
		items = append(items, r.Item)
	}
	return count, items, nil
}
