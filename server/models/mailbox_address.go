package models

import "time"

type MailboxAddress struct {
	ID        int       `xorm:"id int notnull pk autoincr" json:"id"`
	UserID    int       `xorm:"user_id bigint notnull index" json:"user_id"`
	LocalPart string    `xorm:"local_part varchar(64) notnull unique(address)" json:"local_part"`
	DomainID  int       `xorm:"domain_id int notnull index unique(address)" json:"domain_id"`
	IsPrimary bool      `xorm:"is_primary bool notnull default(false)" json:"is_primary"`
	CanSend   bool      `xorm:"can_send bool notnull default(true)" json:"can_send"`
	Created   time.Time `xorm:"created" json:"created"`
}

func (MailboxAddress) TableName() string { return "mailbox_address" }
