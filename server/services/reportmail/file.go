package reportmail

import (
	"context"
	"errors"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/quota"
	"xorm.io/xorm"
)

var ErrGroup = errors.New("invalid report mailbox")

const inbox = "status=0 AND (group_id=0 OR group_id=?)"

// InitialState is persisted with SMTP DATA, including classifications that
// depend on wire headers not retained by the message store.
func InitialState(k Kind) string {
	if k == DMARCAggregate || k == TLSReport {
		return string(k)
	}
	return ":" + string(k)
}

func getPreference(q *xorm.Session, uid int) (models.ReportMailPreference, error) {
	p := models.ReportMailPreference{UserID: uid}
	ok, err := q.Get(&p)
	if !ok && err == nil {
		p.Enabled, p.MarkRead = true, true
	}
	return p, err
}

func Preference(ctx context.Context, uid int) (models.ReportMailPreference, error) {
	q := db.Instance.Context(ctx)
	defer q.Close()
	return getPreference(q, uid)
}

// UpdatePreference accepts partial writes so the current U8 client, which
// doesn't send forward_reports, cannot reset a stored forwarding preference.
func UpdatePreference(ctx context.Context, uid int, enabled *bool, groupID *int, read, forward *bool) (p models.ReportMailPreference, err error) {
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if _, err := quota.Lock(tx, uid); err != nil {
			return err
		}
		var err error
		p, err = getPreference(tx, uid)
		if err != nil {
			return err
		}
		if groupID != nil {
			if *groupID != 0 {
				ok, err := ownedFolder(tx, uid, *groupID)
				if err != nil {
					return err
				}
				if !ok {
					return ErrGroup
				}
			}
			p.GroupID = *groupID
		}
		if enabled != nil {
			p.Enabled = *enabled
		}
		if read != nil {
			p.MarkRead = *read
		}
		if forward != nil {
			p.ForwardReports = *forward
		}
		return savePreference(tx, p)
	})
	return
}

func savePreference(tx *xorm.Session, p models.ReportMailPreference) error {
	found, err := tx.ID(p.UserID).Exist(&models.ReportMailPreference{})
	if err != nil {
		return err
	}
	if !found {
		_, err = tx.Insert(&p)
	} else {
		_, err = tx.ID(p.UserID).Cols("enabled", "group_id", "mark_read", "forward_reports").Update(&p)
	}
	return err
}

func ownedFolder(tx *xorm.Session, uid, id int) (bool, error) {
	if id <= 0 || models.GroupCodeToName[id] != "" {
		return false, nil
	}
	q := tx.ID(id).Where("user_id=?", uid).Cols("id")
	if tx.Engine().DriverName() != config.DBTypeSQLite {
		q.ForUpdate()
	}
	return q.Get(&models.Group{})
}

func ensureFolder(tx *xorm.Session, p *models.ReportMailPreference) error {
	ok, err := ownedFolder(tx, p.UserID, p.GroupID)
	if err != nil || ok {
		return err
	}
	name := i18n.Text(config.DefaultLang(), "report_mail_folder")
	var group models.Group
	q := tx.Where("user_id=? AND name=? AND parent_id=0", p.UserID, name).Asc("id")
	if tx.Engine().DriverName() != config.DBTypeSQLite {
		q.ForUpdate()
	}
	found, err := q.Get(&group)
	if err != nil {
		return err
	}
	if !found {
		group = models.Group{Name: name, UserId: p.UserID, FullPath: name}
		if _, err := tx.Insert(&group); err != nil {
			return err
		}
	}
	p.GroupID = group.ID
	return savePreference(tx, *p)
}

// File only moves matching links that are still in Inbox at write time.
// It is called after rules, so a rule's move/delete is never undone.
func File(ctx context.Context, uid int, emailIDs []int) (moved int64, p models.ReportMailPreference, err error) {
	if len(emailIDs) == 0 {
		return
	}
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		moved = 0
		tx.Context(ctx)
		if _, err := quota.Lock(tx, uid); err != nil {
			return err
		}
		var err error
		p, err = getPreference(tx, uid)
		if err != nil || !p.Enabled {
			return err
		}
		var links []int
		if err := tx.Table(&models.UserEmail{}).Cols("id").Where("user_id=? AND "+inbox, uid, models.INBOX).In("email_id", emailIDs).Find(&links); err != nil || len(links) == 0 {
			return err
		}
		if err := ensureFolder(tx, &p); err != nil {
			return err
		}
		values := map[string]any{"group_id": p.GroupID}
		if p.MarkRead {
			values["is_read"] = 1
		}
		moved, err = tx.Table(&models.UserEmail{}).Where("user_id=? AND "+inbox, uid, models.INBOX).In("id", links).Update(values)
		return err
	})
	return
}

type ApplyResult struct {
	Moved   int64 `json:"moved"`
	GroupID int   `json:"group_id,omitempty"`
}

func Apply(ctx context.Context, uid int) (out ApplyResult, err error) {
	p, err := Preference(ctx, uid)
	if err != nil || !p.Enabled {
		return out, err
	}
	out.GroupID = p.GroupID
	var boundary struct {
		LastID int `xorm:"last_id"`
	}
	if _, err := db.Instance.Context(ctx).SQL("SELECT COALESCE(MAX(id),0) AS last_id FROM user_email WHERE user_id=? AND "+inbox, uid, models.INBOX).Get(&boundary); err != nil {
		return out, err
	}
	var last int
	for ctx.Err() == nil {
		var links []models.UserEmail
		if err := db.Instance.Context(ctx).Cols("id", "email_id").Where("user_id=? AND id>? AND id<=? AND "+inbox, uid, last, boundary.LastID, models.INBOX).Asc("id").Limit(batchSize).Find(&links); err != nil {
			return out, err
		}
		if len(links) == 0 {
			return out, nil
		}
		last = links[len(links)-1].ID
		ids := make([]int, 0, len(links))
		for _, link := range links {
			ids = append(ids, link.EmailID)
		}
		var rows []models.Email
		if err := db.Instance.Context(ctx).Cols("id", "subject", "report_mail_state", "projection_ready").Where("type=?", consts.EmailTypeReceive).In("id", ids).Find(&rows); err != nil {
			return out, err
		}
		meta, err := metadata(ctx, rows)
		if err != nil {
			return out, err
		}
		ids = ids[:0]
		for _, row := range rows {
			k, err := classifyStored(ctx, row, meta[row.Id])
			if err != nil {
				return out, err
			}
			if k != None {
				ids = append(ids, row.Id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		n, p, err := File(ctx, uid, ids)
		if err != nil {
			return out, err
		}
		out.Moved += n
		if !p.Enabled {
			return out, nil
		}
		out.GroupID = p.GroupID
	}
	return out, ctx.Err()
}

func classifyStored(ctx context.Context, row models.Email, parts []*parsemail.Attachment) (Kind, error) {
	if row.ReportMailState != "" {
		return Kind(strings.TrimPrefix(row.ReportMailState, ":")), nil
	}
	if len(parts) == 0 && !row.ProjectionReady {
		var err error
		parts, err = storedParts(ctx, row.Id)
		if err != nil && !errors.Is(err, errPayload) {
			return None, err
		}
	}
	return Detect(&parsemail.Email{Subject: row.Subject, Attachments: parts}, nil), nil
}
