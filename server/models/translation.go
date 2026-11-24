package models

import "time"

type TranslationSettings struct {
	ID       int    `xorm:"id pk"`
	Settings string `xorm:"settings text notnull"`
}

func (TranslationSettings) TableName() string { return "translation_settings" }

type AIConfig struct {
	UserID   int    `xorm:"user_id pk"`
	Settings string `xorm:"settings text notnull"`
}

func (AIConfig) TableName() string { return "user_ai_config" }

type TranslationPreference struct {
	UserID   int    `xorm:"user_id pk"`
	Settings string `xorm:"settings text notnull"`
}

func (TranslationPreference) TableName() string { return "translation_preference" }

type TranslationUsage struct {
	UserID   int   `xorm:"user_id pk"`
	Day      int64 `xorm:"day bigint notnull"`
	Requests int   `xorm:"requests int notnull"`
}

func (TranslationUsage) TableName() string { return "translation_usage" }

type EmailTranslation struct {
	EmailID    int       `xorm:"email_id pk index"`
	UserID     int       `xorm:"user_id pk index"` // zero for web, user ID for AI
	TargetLang string    `xorm:"target_lang varchar(12) pk"`
	Engine     string    `xorm:"engine varchar(8) pk"`
	Subject    string    `xorm:"subject text notnull"`
	Text       string    `xorm:"text mediumtext notnull"`
	Provider   string    `xorm:"provider varchar(32) notnull"`
	SourceLang string    `xorm:"source_lang varchar(24) notnull"`
	CacheKey   string    `xorm:"cache_key char(64) notnull"`
	Created    time.Time `xorm:"created"`
}

func (EmailTranslation) TableName() string { return "email_translation" }

type EmailAIResult struct {
	UserID     int       `xorm:"user_id pk"`
	EmailID    int       `xorm:"email_id pk index"`
	Action     string    `xorm:"action varchar(12) pk"`
	Lang       string    `xorm:"lang varchar(12) pk"`
	ResultJSON string    `xorm:"result_json mediumtext notnull"`
	Model      string    `xorm:"model varchar(200) notnull"`
	CacheKey   string    `xorm:"cache_key char(64) notnull"`
	Created    time.Time `xorm:"created"`
}

func (EmailAIResult) TableName() string { return "email_ai_result" }
