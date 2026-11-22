package models

import "time"

type APIToken struct {
	ID            int        `xorm:"id int notnull pk autoincr" json:"id"`
	UserID        int        `xorm:"user_id int notnull index" json:"user_id"`
	Name          string     `xorm:"name varchar(100) notnull" json:"name"`
	TokenHash     string     `xorm:"token_hash char(64) notnull unique" json:"-"`
	Prefix        string     `xorm:"prefix char(8) notnull index" json:"prefix"`
	AuthVersion   int64      `xorm:"auth_version bigint notnull default(0)" json:"-"`
	Scope         string     `xorm:"scope varchar(16) notnull default('legacy')" json:"scope"`
	LastUsedEpoch int64      `xorm:"last_used_epoch bigint notnull default(0)" json:"-"`
	LastUsedAt    *time.Time `xorm:"last_used_at" json:"last_used_at"`
	ExpiresAt     *time.Time `xorm:"expires_at" json:"expires_at"`
	Created       time.Time  `xorm:"created" json:"created"`
}

func (APIToken) TableName() string { return "api_token" }
