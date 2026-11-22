package models

// Scores live on mailbox links; shared content evidence lives on Email.
type SpamPreference struct {
	UserID       int     `xorm:"user_id pk" json:"-"`
	Enabled      bool    `xorm:"enabled bool notnull" json:"enabled"`
	Threshold    float64 `xorm:"threshold double notnull" json:"threshold"`
	BayesEnabled bool    `xorm:"bayes_enabled bool notnull" json:"bayes_enabled"`
}

func (SpamPreference) TableName() string { return "spam_preference" }

type SpamRule struct {
	ID        int    `xorm:"id pk autoincr" json:"id"`
	UserID    int    `xorm:"user_id notnull unique(rule_key)" json:"-"`
	Action    string `xorm:"action varchar(5) notnull unique(rule_key)" json:"action"`
	Kind      string `xorm:"kind varchar(8) notnull unique(rule_key)" json:"kind"`
	Value     string `xorm:"value varchar(512) notnull" json:"value"`
	ValueHash string `xorm:"value_hash char(64) notnull unique(rule_key)" json:"-"`
}

func (SpamRule) TableName() string { return "spam_rule" }

type InboundSettings struct {
	ID                   int     `xorm:"id pk" json:"-"`
	DNSBLEnabled         bool    `xorm:"dnsbl_enabled bool notnull" json:"dnsbl_enabled"`
	ZonesJSON            string  `xorm:"dnsbl_zones text notnull" json:"-"`
	DefaultSpamThreshold float64 `xorm:"default_spam_threshold double notnull" json:"default_spam_threshold"`
	ClamdEnabled         bool    `xorm:"clamd_enabled bool notnull" json:"clamd_enabled"`
	ClamdAddr            string  `xorm:"clamd_addr varchar(255) notnull" json:"clamd_addr"`
	RiskAudit            bool    `xorm:"risk_block_dangerous_attachments bool notnull" json:"risk_block_dangerous_attachments"`
}

func (InboundSettings) TableName() string { return "inbound_settings" }

type SpamTraining struct {
	UserID  int    `xorm:"user_id pk"`
	EmailID int    `xorm:"email_id pk"`
	IsSpam  bool   `xorm:"is_spam bool notnull"`
	Tokens  string `xorm:"tokens mediumtext notnull"`
}

func (SpamTraining) TableName() string { return "spam_training" }

type SpamTotals struct {
	UserID int   `xorm:"user_id pk"`
	Spam   int64 `xorm:"spam bigint notnull"`
	Ham    int64 `xorm:"ham bigint notnull"`
}

func (SpamTotals) TableName() string { return "spam_totals" }

type SpamToken struct {
	UserID int    `xorm:"user_id pk"`
	Key    string `xorm:"token_key char(64) pk"`
	Spam   int64  `xorm:"spam bigint notnull"`
	Ham    int64  `xorm:"ham bigint notnull"`
}

func (SpamToken) TableName() string { return "spam_token" }
