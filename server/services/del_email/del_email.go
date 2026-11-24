package del_email

import (
	"cmp"
	"slices"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

func DelEmail(ctx *context.Context, ids []int, forced bool) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if ctx.Context != nil {
			tx.Context(ctx.Context)
		}
		if _, err := quota.Lock(tx, ctx.UserID); err != nil {
			return err
		}
		if !forced {
			return moveToTrash(tx, ctx.UserID, ids, false)
		}

		ids = slices.Clone(ids)
		slices.Sort(ids) // message locks are always taken in ascending email ID order
		for batch := range slices.Chunk(slices.Compact(ids), BatchSize) {
			var links []models.UserEmail
			if err := tx.Cols("id", "email_id").Where("user_id=?", ctx.UserID).In("email_id", batch).Find(&links); err != nil {
				return err
			}
			if _, err := PurgeLinks(tx, ctx.UserID, links); err != nil {
				return err
			}
		}
		return nil
	})
}

// PurgeLinks permanently removes the selected memberships. Its caller must
// hold the owner's quota lock before selecting them. Shared content is only
// deleted after the last link disappears, under the message lock.
func PurgeLinks(tx *xorm.Session, owner int, links []models.UserEmail) (int64, error) {
	slices.SortFunc(links, func(a, b models.UserEmail) int { return cmp.Compare(a.EmailID, b.EmailID) })
	var total int64
	for start := 0; start < len(links); {
		id := links[start].EmailID
		var ids []int
		for start < len(links) && links[start].EmailID == id {
			ids = append(ids, links[start].ID)
			start++
		}
		size, _, err := quota.LockMessage(tx, id)
		if err != nil {
			return 0, err
		}
		var removed int64
		for batch := range slices.Chunk(ids, BatchSize) {
			n, err := tx.Where("email_id=? AND user_id=?", id, owner).In("id", batch).Delete(&models.UserEmail{})
			if err != nil {
				return 0, err
			}
			if n != 0 {
				if err := mailaddress.DeleteLinks(tx, batch); err != nil {
					return 0, err
				}
				if err := ForgetUIDs(tx, batch); err != nil {
					return 0, err
				}
			}
			removed += n
		}
		if err := afterUnlink(tx, owner, id, removed, size); err != nil {
			return 0, err
		}
		total += removed
	}
	return total, nil
}

func afterUnlink(tx *xorm.Session, owner, id int, n, size int64) error {
	if n == 0 {
		return nil
	}
	if err := quota.Adjust(tx, owner, -n*size); err != nil {
		return err
	}
	// The last reference check must see any concurrent deletion that won the
	// message lock earlier, even on MySQL's default REPEATABLE READ.
	q := tx.Cols("id").Where("email_id=?", id)
	if tx.Engine().DriverName() != "sqlite" {
		q.ForUpdate()
	}
	linked, err := q.Get(&models.UserEmail{})
	if err != nil || linked {
		return err
	}
	deleted, err := tx.ID(id).Delete(&models.Email{})
	if err != nil || deleted == 0 {
		return err
	}
	return mailstore.DeleteDerived(tx, id)
}

func DelByUID(ctx *context.Context, ids []int) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if _, err := quota.Lock(tx, ctx.UserID); err != nil {
			return err
		}
		var links []models.UserEmail
		for batch := range slices.Chunk(ids, BatchSize) {
			var part []models.UserEmail
			if err := tx.Where("user_id=?", ctx.UserID).In("id", batch).Cols("id", "email_id").Find(&part); err != nil {
				return err
			}
			links = append(links, part...)
		}
		_, err := PurgeLinks(tx, ctx.UserID, links)
		return err
	})
}
