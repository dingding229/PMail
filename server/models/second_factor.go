package models

import "time"

// The user row carries the enabled flag and generation; this row holds the
// encrypted setup secret and the last successfully used TOTP counter.
type TOTPConfig struct {
	UserID       int    `xorm:"user_id pk" json:"-"`
	Ciphertext   string `xorm:"ciphertext varchar(512) notnull" json:"-"`
	SetupExpires int64  `xorm:"setup_expires bigint notnull" json:"-"`
	LastStep     int64  `xorm:"last_step bigint notnull" json:"-"`
}

func (TOTPConfig) TableName() string { return "totp_config" }

type RecoveryCode struct {
	UserID int    `xorm:"user_id pk" json:"-"`
	Digest string `xorm:"digest char(64) pk" json:"-"`
}

func (RecoveryCode) TableName() string { return "recovery_code" }

type AppPassword struct {
	ID         int        `xorm:"id pk autoincr" json:"id"`
	UserID     int        `xorm:"user_id notnull index(app_owner_selector)" json:"-"`
	Name       string     `xorm:"name varchar(80) notnull" json:"name"`
	Selector   string     `xorm:"selector char(4) notnull index(app_owner_selector)" json:"-"`
	Hash       string     `xorm:"password_hash varchar(100) notnull" json:"-"`
	CreatedAt  time.Time  `xorm:"created_at created" json:"created_at"`
	LastUsedAt *time.Time `xorm:"last_used_at" json:"last_used_at"`
}

func (AppPassword) TableName() string { return "app_password" }
