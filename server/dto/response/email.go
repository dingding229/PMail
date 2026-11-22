package response

import "github.com/Jinnrry/pmail/models"

type EmailResponseData struct {
	models.Email  `xorm:"extends"`
	IsRead        int8 `json:"is_read"`
	IsStarred     bool `json:"is_starred"`
	MailboxStatus int8 `xorm:"mailbox_status" json:"-"`
	SerialNumber  int  `json:"serial_number"`
	UeId          int  `json:"ue_id"`
}

type UserEmailUIDData struct {
	models.UserEmail `xorm:"extends"`
	SerialNumber     int `json:"serial_number"`
}
