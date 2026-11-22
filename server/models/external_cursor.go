package models

// ExternalCursor scopes an IMAP high-water mark to one remote mailbox and its UID generation.
type ExternalCursor struct {
	AccountID   int    `xorm:"account_id pk notnull"`
	Folder      string `xorm:"folder varchar(255) pk notnull"`
	UIDValidity int64  `xorm:"uidvalidity bigint notnull"`
	LastUID     int64  `xorm:"last_uid bigint notnull"`
}

func (ExternalCursor) TableName() string { return "external_cursor" }
