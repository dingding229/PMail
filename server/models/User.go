package models

import "time"

type User struct {
	ID          int        `xorm:"id unsigned int not null pk autoincr" json:"id"`
	Account     string     `xorm:"account varchar(20) notnull unique comment('账号登陆名')" json:"account"`
	Name        string     `xorm:"name varchar(50) notnull comment('用户名')" json:"name"`
	Password    string     `xorm:"password varchar(100) notnull comment('bcrypt password hash')" json:"-"`
	Disabled    int        `xorm:"disabled unsigned int not null default(0) comment('0启用，1禁用')" json:"disabled"`
	IsAdmin     int        `xorm:"is_admin unsigned int not null default(0) comment('0不是管理员，1是管理员')" json:"is_admin"`
	AuthVersion int64      `xorm:"auth_version bigint notnull default(0)" json:"-"`
	QuotaMB     int64      `xorm:"quota_mb bigint notnull default(0)" json:"quota_mb"`
	Created     time.Time  `xorm:"created" json:"created"`
	LastLoginAt *time.Time `xorm:"last_login_at" json:"last_login_at"`
}

func (User) TableName() string { return "user" }
