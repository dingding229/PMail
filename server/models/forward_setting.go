package models

import "time"

type ForwardSetting struct {
	UserID   int       `xorm:"user_id int notnull pk" json:"user_id"`
	Enabled  bool      `xorm:"enabled bool notnull default(false)" json:"enabled"`
	Targets  string    `xorm:"targets text notnull" json:"targets"`
	KeepCopy bool      `xorm:"keep_copy bool notnull default(true)" json:"keep_copy"`
	Updated  time.Time `xorm:"updated updated" json:"updated"`
}

func (ForwardSetting) TableName() string { return "forward_setting" }
