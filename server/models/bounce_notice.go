package models

import "time"

// BounceNotice records the last failure notice emitted for a forwarding target.
// NoticeKey makes the 24-hour window safe when multiple workers race.
type BounceNotice struct {
	ID        int       `xorm:"id int notnull pk autoincr" json:"id"`
	NoticeKey string    `xorm:"notice_key varchar(128) notnull unique" json:"-"`
	UserID    int       `xorm:"user_id int notnull index" json:"user_id"`
	RcptTo    string    `xorm:"rcpt_to varchar(320) notnull index" json:"rcpt_to"`
	Created   time.Time `xorm:"created datetime notnull index" json:"created"`
}

func (BounceNotice) TableName() string { return "bounce_notice" }
