package db

import (
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

// Stamp legacy Trash links once. Do not infer their original location or
// extend retention when PMail restarts. Each update is bounded to 500 rows.
func backfillTrash(engine *xorm.Engine, deletedAt int64) error {
	const batchSize = 500
	last := 0
	for {
		var ids []int
		err := Transaction(engine, func(tx *xorm.Session) error {
			ids = nil
			if err := tx.Table(&models.UserEmail{}).Cols("id").
				Where("id>? AND deleted_at IS NULL AND (status=? OR group_id=?)", last, consts.EmailStatusDel, models.Deleted).
				Asc("id").Limit(batchSize).Find(&ids); err != nil || len(ids) == 0 {
				return err
			}
			_, err := tx.Table(&models.UserEmail{}).In("id", ids).
				Where("deleted_at IS NULL AND (status=? OR group_id=?)", consts.EmailStatusDel, models.Deleted).
				Update(map[string]any{"status": consts.EmailStatusDel, "group_id": 0, "deleted_at": deletedAt, "prev_status": nil, "prev_group_id": nil})
			return err
		})
		if err != nil || len(ids) < batchSize {
			return err
		}
		last = ids[len(ids)-1]
	}
}
