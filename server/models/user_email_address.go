package models

// UserEmailAddress describes one real mailbox address associated with a
// user's folder membership. Catch-all recipients are stored on UserEmail.
type UserEmailAddress struct {
	UserEmailID int `xorm:"user_email_id int notnull pk"`
	AddressID   int `xorm:"address_id int notnull pk index"`
}

func (UserEmailAddress) TableName() string { return "user_email_address" }

// UserEmailAddressProgress keeps the highest fully scanned membership ID.
type UserEmailAddressProgress struct {
	ID        int `xorm:"id pk"`
	ThroughID int `xorm:"through_id int notnull"`
}

func (UserEmailAddressProgress) TableName() string { return "user_email_address_progress" }
