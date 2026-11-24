package models

import "time"

const (
	AddressActive   = "active"
	AddressPending  = "pending"
	AddressRejected = "rejected"
)

type MailboxAddress struct {
	ID           int        `xorm:"id int notnull pk autoincr" json:"id"`
	UserID       int        `xorm:"user_id bigint notnull index" json:"user_id"`
	LocalPart    string     `xorm:"local_part varchar(64) notnull unique(address_claim)" json:"local_part"`
	DomainID     int        `xorm:"domain_id int notnull index unique(address_claim)" json:"domain_id"`
	IsPrimary    bool       `xorm:"is_primary bool notnull default(false)" json:"is_primary"`
	CanSend      bool       `xorm:"can_send bool notnull default(true)" json:"can_send"`
	Created      time.Time  `xorm:"created" json:"created"`
	Status       string     `xorm:"status varchar(8) notnull default('active') index" json:"status"`
	RequestedAt  *time.Time `xorm:"requested_at" json:"requested_at"`
	ReviewedAt   *time.Time `xorm:"reviewed_at" json:"reviewed_at"`
	ReviewedBy   *int       `xorm:"reviewed_by bigint" json:"reviewed_by"`
	RejectReason string     `xorm:"reject_reason varchar(200) notnull default('')" json:"reject_reason"`
	// Active/pending addresses share the reserved key 0. Rejection changes
	// the key to the row's ID, keeping history without reserving the address.
	ReservationKey int `xorm:"reservation_key int notnull default(0) unique(address_claim)" json:"-"`
}

func (MailboxAddress) TableName() string { return "mailbox_address" }

func (a *MailboxAddress) BeforeInsert() {
	if a.Status == "" {
		a.Status = AddressActive
	}
}

type AddressSettings struct {
	ID                      int  `xorm:"id pk" json:"-"`
	AddressApprovalRequired bool `xorm:"address_approval_required bool notnull default(true)" json:"address_approval_required"`
}

func (AddressSettings) TableName() string { return "address_settings" }
