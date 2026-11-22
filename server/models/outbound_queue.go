package models

import "time"

// OutboundQueue is one externally delivered recipient. RawData contains the
// exact message bytes to deliver (including forwarding headers/signatures).
type OutboundQueue struct {
	ID          int        `xorm:"id int notnull pk autoincr" json:"id"`
	Kind        string     `xorm:"kind varchar(16) notnull index index(srs_lookup)" json:"kind"`
	UserID      int        `xorm:"user_id int notnull index" json:"user_id"`
	EmailID     *int       `xorm:"email_id int index" json:"email_id,omitempty"`
	MailFrom    string     `xorm:"mail_from varchar(320) notnull index(srs_lookup)" json:"mail_from"`
	RcptTo      string     `xorm:"rcpt_to varchar(320) notnull index" json:"rcpt_to"`
	RawData     []byte     `xorm:"raw_data blob notnull" json:"-"`
	Attempts    int        `xorm:"attempts int notnull default(0)" json:"attempts"`
	MaxAttempts int        `xorm:"max_attempts int notnull default(8)" json:"max_attempts"`
	NextRetryAt time.Time  `xorm:"next_retry_at datetime index" json:"next_retry_at"`
	Status      string     `xorm:"status varchar(16) notnull index" json:"status"`
	LastError   string     `xorm:"last_error text" json:"last_error"`
	LeaseEpoch  int64      `xorm:"lease_epoch bigint notnull default(0) index" json:"-"`
	LeaseFence  int64      `xorm:"lease_fence bigint notnull default(0)" json:"-"`
	LeaseUntil  *time.Time `xorm:"lease_until datetime index" json:"-"`
	Created     time.Time  `xorm:"created created" json:"created"`
	Updated     time.Time  `xorm:"updated updated" json:"updated"`
}

func (OutboundQueue) TableName() string { return "outbound_queue" }
