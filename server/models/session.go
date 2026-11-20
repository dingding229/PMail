package models

import "time"

type Sessions struct {
	Token  string    `xorm:"token char(43) not null pk " json:"token"`
	Data   []byte    `xorm:"data blob" json:"data"`
	Expiry time.Time `xorm:"expiry timestamp index" json:"expiry"`
}

func (p *Sessions) TableName() string {
	return "sessions"
}
