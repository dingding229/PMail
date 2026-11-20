package models

import "time"

type Domain struct {
	ID             int       `xorm:"id int notnull pk autoincr" json:"id"`
	Name           string    `xorm:"name varchar(253) notnull unique" json:"name"`
	IsDefault      bool      `xorm:"is_default bool notnull default(false)" json:"is_default"`
	DkimSelector   string    `xorm:"dkim_selector varchar(63) notnull default('default')" json:"dkim_selector"`
	DkimPrivateKey string    `xorm:"dkim_private_key text notnull" json:"-"`
	Created        time.Time `xorm:"created created" json:"created"`
	Updated        time.Time `xorm:"updated updated" json:"updated"`
}

func (Domain) TableName() string { return "domain" }
