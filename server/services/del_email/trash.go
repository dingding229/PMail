package del_email

import (
	stdctx "context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailevents"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

const BatchSize = 500
const DefaultRetentionDays = 30
const MaxRetentionDays = 3650
const DayMillis = int64(24 * time.Hour / time.Millisecond)

var ErrRetention = errors.New("invalid Trash retention")

// TrashCopy records where this link came from. IMAP COPY/MOVE may create a
// new membership, whereas web/POP3/rules modify the existing link in place.
func TrashCopy(dst *models.UserEmail, from models.UserEmail, now time.Time) {
	dst.Status, dst.GroupId = consts.EmailStatusDel, 0
	if from.Status == consts.EmailStatusDel || from.GroupId == models.Deleted {
		dst.DeletedAt, dst.PrevStatus, dst.PrevGroupID = from.DeletedAt, from.PrevStatus, from.PrevGroupID
		if dst.DeletedAt != nil {
			return
		}
	} else {
		dst.PrevStatus, dst.PrevGroupID = &from.Status, &from.GroupId
	}
	at := now.UnixMilli()
	dst.DeletedAt = &at
}

// ForgetUIDs removes stale projections without resetting mailbox UIDNEXT.
// A reappearing link receives a fresh UID even if no client observed its absence.
// Call from the same owner-locked transaction as the membership change.
func ForgetUIDs(tx *xorm.Session, linkIDs []int) error {
	for batch := range slices.Chunk(linkIDs, BatchSize) {
		if _, err := tx.In("link_id", batch).Delete(&models.IMAPMailboxUID{}); err != nil {
			return err
		}
	}
	return nil
}

func moveToTrash(tx *xorm.Session, owner int, ids []int, inboxOnly bool) error {
	now := time.Now().UnixMilli()
	for batch := range slices.Chunk(ids, BatchSize) {
		var links []int
		q := tx.Table(&models.UserEmail{}).Cols("id").Where("user_id=? AND status<>? AND group_id<>?", owner, consts.EmailStatusDel, models.Deleted).In("email_id", batch)
		if inboxOnly {
			q.Where("status=? AND (group_id=0 OR group_id=?)", consts.EmailStatusWait, models.INBOX)
		}
		if err := q.Find(&links); err != nil {
			return err
		}
		for chunk := range slices.Chunk(links, BatchSize) {
			// Copy original columns in SQL before assigning new values (MySQL
			// evaluates single-table UPDATE assignments left to right).
			if _, err := tx.Table(&models.UserEmail{}).In("id", chunk).
				SetExpr("prev_status", "status").SetExpr("prev_group_id", "group_id").
				Cols("prev_status", "prev_group_id").Update(&models.UserEmail{}); err != nil {
				return err
			}
			if _, err := tx.Table(&models.UserEmail{}).In("id", chunk).
				Update(map[string]any{"deleted_at": now, "status": consts.EmailStatusDel, "group_id": 0}); err != nil {
				return err
			}
			if err := ForgetUIDs(tx, chunk); err != nil {
				return err
			}
		}
	}
	return nil
}

// TrashInbox is the POP3 QUIT path; only links still in Inbox can be moved.
func TrashInbox(ctx *context.Context, ids []int) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if _, err := quota.Lock(tx, ctx.UserID); err != nil {
			return err
		}
		return moveToTrash(tx, ctx.UserID, ids, true)
	})
}

func Restore(ctx *context.Context, ids []int) (int64, error) {
	var restored int64
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		restored = 0
		if _, err := quota.Lock(tx, ctx.UserID); err != nil {
			return err
		}
		// Email IDs are public identifiers, but only the caller's Trash links
		// are candidates. Live copies and foreign links are never changed.
		for batch := range slices.Chunk(ids, BatchSize) {
			var links []models.UserEmail
			if err := tx.Where("user_id=? AND status=?", ctx.UserID, consts.EmailStatusDel).In("email_id", batch).Find(&links); err != nil {
				return err
			}
			for _, link := range links {
				status, groupID, err := restoreTarget(tx, ctx.UserID, link)
				if err != nil {
					return err
				}
				n, err := tx.Table(&models.UserEmail{}).ID(link.ID).Where("user_id=? AND status=?", ctx.UserID, consts.EmailStatusDel).
					Update(restoredFields(status, groupID))
				if err != nil {
					return err
				}
				if n != 0 {
					if err := ForgetUIDs(tx, []int{link.ID}); err != nil {
						return err
					}
					restored += n
				}
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return restored, nil
}

func restoredFields(status int8, groupID int) map[string]any {
	return map[string]any{"status": status, "group_id": groupID, "deleted_at": nil, "prev_status": nil, "prev_group_id": nil, "imap_deleted": false}
}

func restoreTarget(tx *xorm.Session, owner int, link models.UserEmail) (int8, int, error) {
	if link.PrevStatus == nil {
		var mail models.Email
		_, err := tx.ID(link.EmailID).Cols("type").Get(&mail)
		if mail.Type == consts.EmailTypeSend {
			return consts.EmailStatusSent, 0, err
		}
		return consts.EmailStatusWait, 0, err
	}
	groupID := 0
	if link.PrevGroupID != nil {
		groupID = *link.PrevGroupID
	}
	if groupID != 0 {
		if _, builtin := models.GroupCodeToName[groupID]; !builtin {
			q := tx.ID(groupID).Cols("id").Where("user_id=?", owner)
			if tx.Engine().DriverName() != "sqlite" {
				q.ForUpdate()
			}
			ok, err := q.Get(&models.Group{})
			if err != nil || !ok {
				return consts.EmailStatusWait, 0, err
			}
		}
	}
	return *link.PrevStatus, groupID, nil
}

func Retention(ctx stdctx.Context) (int, error) {
	var row models.TrashSettings
	ok, err := db.Instance.Context(ctx).ID(1).Get(&row)
	if err != nil {
		return 0, err
	}
	if !ok {
		return DefaultRetentionDays, nil
	}
	return row.RetentionDays, nil
}

func SetRetention(ctx stdctx.Context, days int) error {
	if days < 0 || days > MaxRetentionDays {
		return ErrRetention
	}
	query := "INSERT INTO trash_settings (id,trash_retention_days) VALUES (1,?) ON CONFLICT (id) DO UPDATE SET trash_retention_days=excluded.trash_retention_days"
	if db.Instance.DriverName() == "mysql" {
		query = "INSERT INTO trash_settings (id,trash_retention_days) VALUES (1,?) ON DUPLICATE KEY UPDATE trash_retention_days=VALUES(trash_retention_days)"
	}
	_, err := db.Instance.Context(ctx).Exec(query, days)
	return err
}

// Empty permanently deletes the caller's Trash copies, not other copies of
// the same message. The returned count is the number of committed link deletes.
func Empty(ctx stdctx.Context, owner int) (int64, error) {
	var last sql.NullInt64
	if _, err := db.Instance.Context(ctx).Table(&models.UserEmail{}).Select("MAX(id)").Where("user_id=? AND status=?", owner, consts.EmailStatusDel).Get(&last); err != nil || !last.Valid {
		return 0, err
	}
	return purgeTrash(ctx, owner, "id<=?", last.Int64)
}

// PurgeExpired uses a strict cutoff: a link exactly at the deadline will be
// eligible on a subsequent run. A zero retention period disables the sweep.
func PurgeExpired(ctx stdctx.Context, now time.Time) (int64, error) {
	days, err := Retention(ctx)
	if err != nil || days == 0 {
		return 0, err
	}
	cutoff := now.UnixMilli() - int64(days)*DayMillis
	var total int64
	lastOwner := 0
	for {
		var owners []int
		if err := db.Instance.Context(ctx).Table(&models.UserEmail{}).Distinct("user_id").
			Where("status=? AND deleted_at<? AND user_id>?", consts.EmailStatusDel, cutoff, lastOwner).Asc("user_id").Limit(100).Find(&owners); err != nil || len(owners) == 0 {
			return total, err
		}
		for _, owner := range owners {
			n, err := purgeTrash(ctx, owner, "deleted_at<?", cutoff)
			total += n
			if err != nil {
				return total, err
			}
			lastOwner = owner
		}
	}
}

func purgeTrash(ctx stdctx.Context, owner int, condition string, arg int64) (int64, error) {
	var total int64
	defer func() {
		if total > 0 {
			mailevents.Publish(owner)
		}
	}()
	for ctx.Err() == nil {
		var deleted int64
		err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
			tx.Context(ctx)
			deleted = 0
			if _, err := quota.Lock(tx, owner); err != nil {
				return err
			}
			var links []models.UserEmail
			if err := tx.Cols("id", "email_id").Where("user_id=? AND status=?", owner, consts.EmailStatusDel).
				Where(condition, arg).Asc("id").Limit(BatchSize).Find(&links); err != nil {
				return err
			}
			var err error
			deleted, err = PurgeLinks(tx, owner, links)
			return err
		})
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < BatchSize {
			return total, nil
		}
	}
	return total, ctx.Err()
}

func RunCleanup(ctx stdctx.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := PurgeExpired(ctx, time.Now()); err != nil && ctx.Err() == nil {
				log.Warnf("Trash cleanup after deleting %d copies: %v", n, err)
			} else if err == nil && n > 0 {
				log.Infof("Trash cleanup deleted %d expired copies", n)
			}
		}
	}
}

// ClearTrashMetadata is shared by explicit moves or spam reclassification
// out of Trash. The caller holds the owner lock and supplies Trash link IDs.
func ClearTrashMetadata(tx *xorm.Session, links []int) error {
	for batch := range slices.Chunk(links, BatchSize) {
		if _, err := tx.Table(&models.UserEmail{}).In("id", batch).Update(map[string]any{
			"deleted_at": nil, "prev_status": nil, "prev_group_id": nil, "imap_deleted": false,
		}); err != nil {
			return err
		}
	}
	return ForgetUIDs(tx, links)
}

// MoveToDefault also covers a direct move out of Trash (distinct from Restore,
// which uses the tracked previous folder). Other moves retain existing flags.
func MoveToDefault(ctx *context.Context, ids []int, status int8) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if _, err := quota.Lock(tx, ctx.UserID); err != nil {
			return err
		}
		for batch := range slices.Chunk(ids, BatchSize) {
			var trashed []int
			if err := tx.Table(&models.UserEmail{}).Cols("id").Where("user_id=? AND status=?", ctx.UserID, consts.EmailStatusDel).In("email_id", batch).Find(&trashed); err != nil {
				return err
			}
			if err := ClearTrashMetadata(tx, trashed); err != nil {
				return err
			}
			if _, err := tx.Table(&models.UserEmail{}).Where("user_id=?", ctx.UserID).In("email_id", batch).
				Update(map[string]any{"status": status, "group_id": 0}); err != nil {
				return err
			}
		}
		return nil
	})
}
