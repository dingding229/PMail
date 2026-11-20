package models

import "time"

type ExternalSeen struct {
	ID          int       `xorm:"id int notnull pk autoincr" json:"id"`
	AccountID   int       `xorm:"account_id int notnull index unique(seen_key)" json:"account_id"`
	Folder      string    `xorm:"folder varchar(255) notnull index unique(seen_key)" json:"folder"`
	UIDValidity int64     `xorm:"uidvalidity bigint notnull default(0) index unique(seen_key)" json:"uidvalidity"`
	UID         int64     `xorm:"uid bigint notnull default(0) index unique(seen_key)" json:"uid"`
	UIDL        string    `xorm:"uidl varchar(255) notnull default('') index unique(seen_key)" json:"uidl"`
	MessageID   string    `xorm:"message_id varchar(512)" json:"message_id"`
	Status      string    `xorm:"status varchar(16) notnull default('pending')" json:"status"`
	Created     time.Time `xorm:"created created" json:"created"`
}

func (ExternalSeen) TableName() string { return "external_seen" }
