package email

import (
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/services/quota"
	"xorm.io/xorm"
)

func persistOutboundParent(modelEmail *models.Email, userID int) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if _, err := quota.Lock(tx, userID); err != nil {
			return err
		}
		modelEmail.Status = consts.EmailStatusDeliveryPending
		modelEmail.Id = 0
		mailstore.Prepare(modelEmail)
		if _, err := tx.Insert(modelEmail); err != nil {
			return err
		}
		if err := mailstore.WriteParts(tx, modelEmail, false); err != nil {
			return err
		}
		_, err := tx.Insert(&models.UserEmail{UserID: userID, EmailID: modelEmail.Id, Status: consts.EmailStatusDeliveryPending, IsRead: 1})
		if err != nil {
			return err
		}
		return quota.Adjust(tx, userID, int64(modelEmail.Size))
	})
}
