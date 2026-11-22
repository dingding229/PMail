package del_email

import (
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

func DelEmail(ctx *context.Context, ids []int, forced bool) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if forced {
			if _, err := quota.Lock(tx, ctx.UserID); err != nil {
				return err
			}
		}
		for _, id := range ids {
			if !forced {
				if _, err := tx.Where("email_id=? AND user_id=?", id, ctx.UserID).Table(&models.UserEmail{}).Update(map[string]any{"status": consts.EmailStatusDel, "group_id": 0}); err != nil {
					return err
				}
				continue
			}
			size, _, err := quota.LockMessage(tx, id)
			if err != nil {
				return err
			}
			n, err := tx.Where("email_id=? AND user_id=?", id, ctx.UserID).Delete(&models.UserEmail{})
			if err != nil {
				return err
			}
			if err = afterUnlink(tx, ctx.UserID, id, n, size); err != nil {
				return err
			}
		}
		return nil
	})
}

func afterUnlink(tx *xorm.Session, owner, id int, n, size int64) error {
	if n == 0 {
		return nil
	}
	if err := quota.Adjust(tx, owner, -n*size); err != nil {
		return err
	}
	// Delete only if the message no longer has any links.
	deleted, err := tx.ID(id).Where("NOT EXISTS (SELECT 1 FROM user_email WHERE email_id=?)", id).Delete(&models.Email{})
	if err != nil || deleted == 0 {
		return err
	}
	return mailstore.DeleteParts(tx, id)
}

func DelByUID(ctx *context.Context, ids []int) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if _, err := quota.Lock(tx, ctx.UserID); err != nil {
			return err
		}
		for _, id := range ids {
			var row models.UserEmail
			ok, err := tx.ID(id).Where("user_id=?", ctx.UserID).Cols("email_id").Get(&row)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			size, _, err := quota.LockMessage(tx, row.EmailID)
			if err != nil {
				return err
			}
			n, err := tx.ID(id).Where("user_id=?", ctx.UserID).Delete(&models.UserEmail{})
			if err != nil {
				return err
			}
			if err = afterUnlink(tx, ctx.UserID, row.EmailID, n, size); err != nil {
				return err
			}
		}
		return nil
	})
}
