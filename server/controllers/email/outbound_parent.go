package email

import (
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

func persistOutboundParent(modelEmail *models.Email, userID int) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		modelEmail.Status = consts.EmailStatusDeliveryPending
		if _, err := tx.Insert(modelEmail); err != nil {
			return err
		}
		_, err := tx.Insert(&models.UserEmail{UserID: userID, EmailID: modelEmail.Id, Status: consts.EmailStatusDeliveryPending, IsRead: 1})
		return err
	})
}
