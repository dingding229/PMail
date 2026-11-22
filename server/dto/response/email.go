package response

import (
	"database/sql"
	"github.com/Jinnrry/pmail/models"
)

type EmailResponseData struct {
	models.Email  `xorm:"extends"`
	IsRead        int8           `json:"is_read"`
	IsStarred     bool           `json:"is_starred"`
	IMAPDeleted   bool           `json:"-"`
	MailboxStatus int8           `xorm:"mailbox_status" json:"-"`
	SerialNumber  int            `json:"serial_number"`
	LinkID        int            `xorm:"-" json:"-"`
	InboundSpam   sql.NullString `xorm:"inbound_spam" json:"-"`
	CursorDate    string         `xorm:"cursor_date" json:"-"`
	UeId          int            `json:"ue_id"`
}

type UserEmailUIDData struct {
	models.UserEmail `xorm:"extends"`
	SerialNumber     int `json:"serial_number"`
	LinkID           int `xorm:"-" json:"-"`
}
