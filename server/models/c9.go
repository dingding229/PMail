package models

import "time"

type TelegramConfig struct {
	ID         int    `xorm:"id pk"`
	Token      string `xorm:"token text notnull"`
	Proxy      string `xorm:"proxy text notnull"`
	Username   string `xorm:"username varchar(64) notnull"`
	Generation string `xorm:"generation varchar(40) notnull"`
	Offset     int64  `xorm:"next_offset bigint notnull default(0)"`
}

func (TelegramConfig) TableName() string { return "telegram_config" }

type TelegramBinding struct {
	UserID   int       `xorm:"user_id pk"`
	ChatID   int64     `xorm:"chat_id bigint notnull unique"`
	Title    string    `xorm:"title varchar(255) notnull"`
	Identity string    `xorm:"identity varchar(40) notnull"`
	Bot      string    `xorm:"bot_generation varchar(40) notnull"`
	BoundAt  time.Time `xorm:"bound_at"`
}

func (TelegramBinding) TableName() string { return "telegram_binding" }

type TelegramCode struct {
	UserID  int    `xorm:"user_id pk"`
	Hash    string `xorm:"code_hash char(64) notnull unique"`
	Expires int64  `xorm:"expires bigint notnull index"`
	Bot     string `xorm:"bot_generation varchar(40) notnull"`
}

func (TelegramCode) TableName() string { return "telegram_code" }

type TelegramPreference struct {
	UserID  int    `xorm:"user_id pk"`
	Filters string `xorm:"filters text notnull"`
}

func (TelegramPreference) TableName() string { return "telegram_preference" }

type TelegramPush struct {
	ID       int    `xorm:"id pk autoincr"`
	UserID   int    `xorm:"user_id notnull index unique(mail)"`
	EmailID  int    `xorm:"email_id notnull unique(mail)"`
	Binding  string `xorm:"binding varchar(40) notnull"`
	Attempts int    `xorm:"attempts int notnull"`
	Due      int64  `xorm:"due bigint notnull index"`
	Expires  int64  `xorm:"expires bigint notnull index"`
}

func (TelegramPush) TableName() string { return "telegram_push" }

type ComposeImage struct {
	CID         string `xorm:"cid varchar(80) pk"`
	UserID      int    `xorm:"user_id notnull index"`
	Filename    string `xorm:"filename varchar(255) notnull"`
	ContentType string `xorm:"content_type varchar(32) notnull"`
	Size        int    `xorm:"size notnull"`
	Content     []byte `xorm:"content longblob notnull"`
	Touched     int64  `xorm:"touched bigint notnull index"`
}

func (ComposeImage) TableName() string { return "compose_image" }

type DraftInlineImage struct {
	DraftID int    `xorm:"draft_id pk"`
	CID     string `xorm:"cid varchar(80) pk index"`
}

func (DraftInlineImage) TableName() string { return "draft_inline_image" }

type Signature struct {
	ID        int    `xorm:"id pk autoincr" json:"id"`
	UserID    int    `xorm:"user_id notnull index" json:"-"`
	Name      string `xorm:"name varchar(100) notnull" json:"name"`
	HTML      string `xorm:"html mediumtext notnull" json:"html"`
	Addresses string `xorm:"addresses text notnull" json:"-"`
	Default   bool   `xorm:"is_default bool notnull" json:"is_default"`
}

func (Signature) TableName() string { return "signature" }
