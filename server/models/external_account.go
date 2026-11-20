package models

import "time"

type ExternalAccount struct {
	ID                 int        `xorm:"id int notnull pk autoincr" json:"id"`
	UserID             int        `xorm:"user_id int notnull index" json:"user_id"`
	Name               string     `xorm:"name varchar(100) notnull" json:"name"`
	Protocol           string     `xorm:"protocol varchar(8) notnull" json:"protocol"`
	Host               string     `xorm:"host varchar(255) notnull" json:"host"`
	Port               int        `xorm:"port int notnull" json:"port"`
	TLSMode            string     `xorm:"tls_mode varchar(16) notnull" json:"tls_mode"`
	Username           string     `xorm:"username varchar(255) notnull" json:"username"`
	PasswordEnc        string     `xorm:"password_enc text notnull" json:"-"`
	Folder             string     `xorm:"folder varchar(1000) notnull" json:"folder"`
	IntervalMinutes    int        `xorm:"interval_minutes int notnull default(10)" json:"interval_minutes"`
	InitialDays        int        `xorm:"initial_days int notnull default(30)" json:"initial_days"`
	DeleteRemote       bool       `xorm:"delete_remote bool notnull default(false)" json:"delete_remote"`
	TargetGroupID      int        `xorm:"target_group_id int notnull default(0)" json:"target_group_id"`
	Enabled            bool       `xorm:"enabled bool notnull default(true)" json:"enabled"`
	InsecureSkipVerify bool       `xorm:"insecure_skip_verify bool notnull default(false)" json:"insecure_skip_verify"`
	AuthFailures       int        `xorm:"auth_failures int notnull default(0)" json:"-"`
	LastSyncAt         *time.Time `xorm:"last_sync_at" json:"last_sync_at"`
	LastSuccessAt      *time.Time `xorm:"last_success_at" json:"last_success_at"`
	LastError          string     `xorm:"last_error text" json:"last_error"`
	LastUIDValidity    string     `xorm:"last_uidvalidity varchar(128)" json:"-"`
	LastUID            int64      `xorm:"last_uid bigint notnull default(0)" json:"-"`
	Created            time.Time  `xorm:"created created" json:"created"`
	Updated            time.Time  `xorm:"updated updated" json:"updated"`
}

func (ExternalAccount) TableName() string { return "external_account" }
