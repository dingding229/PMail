package models

// ReportMailPreference stores a user's override; an absent row uses service defaults.
type ReportMailPreference struct {
	UserID         int  `xorm:"user_id pk notnull" json:"-"`
	Enabled        bool `xorm:"enabled notnull default(true)" json:"enabled"`
	GroupID        int  `xorm:"group_id notnull default(0)" json:"group_id"`
	MarkRead       bool `xorm:"mark_read notnull default(true)" json:"mark_read"`
	ForwardReports bool `xorm:"forward_reports notnull default(false)" json:"forward_reports"`
}

func (ReportMailPreference) TableName() string { return "report_mail_preference" }

// Report times are Unix seconds (UTC) in all three database engines.
type DMARCReport struct {
	ID         int    `xorm:"id pk autoincr"`
	OrgName    string `xorm:"org_name varchar(160) notnull unique(identity)"`
	ReportID   string `xorm:"report_id varchar(256) notnull unique(identity)"`
	Domain     string `xorm:"domain varchar(253) notnull unique(identity) index(domain_begin)"`
	BeginAt    int64  `xorm:"begin_at bigint notnull index(domain_begin)"`
	EndAt      int64  `xorm:"end_at bigint notnull"`
	BeginDay   string `xorm:"begin_day char(10) notnull"`
	P          string `xorm:"p varchar(16) notnull"`
	SP         string `xorm:"sp varchar(16) notnull"`
	Pct        int    `xorm:"pct notnull"`
	ADKIM      string `xorm:"adkim char(1) notnull"`
	ASPF       string `xorm:"aspf char(1) notnull"`
	EmailID    int    `xorm:"email_id notnull index"`
	ReceivedAt int64  `xorm:"received_at bigint notnull index"`
}

func (DMARCReport) TableName() string { return "dmarc_report" }

type DMARCRecord struct {
	ID           int    `xorm:"id pk autoincr"`
	ReportID     int    `xorm:"report_id notnull index"` // Foreign key to dmarc_report.id; installed by db.SyncSchema.
	SourceIP     string `xorm:"source_ip varchar(45) notnull index"`
	Count        int64  `xorm:"mail_count bigint notnull"`
	Disposition  string `xorm:"disposition varchar(16) notnull"`
	DKIMResult   string `xorm:"dkim_result varchar(16) notnull"`
	SPFResult    string `xorm:"spf_result varchar(16) notnull"`
	HeaderFrom   string `xorm:"header_from varchar(253) notnull"`
	EnvelopeFrom string `xorm:"envelope_from varchar(320) notnull"`
	DKIMDomain   string `xorm:"dkim_domain varchar(253) notnull"`
	DKIMSelector string `xorm:"dkim_selector varchar(253) notnull"`
	DKIMAuth     string `xorm:"dkim_auth varchar(16) notnull"`
	DKIMDetails  string `xorm:"dkim_details text notnull"`
	SPFDomain    string `xorm:"spf_domain varchar(253) notnull"`
	SPFAuth      string `xorm:"spf_auth varchar(16) notnull"`
	SPFDetails   string `xorm:"spf_details text notnull"`
}

func (DMARCRecord) TableName() string { return "dmarc_record" }

type TLSReport struct {
	ID             int    `xorm:"id pk autoincr"`
	OrgName        string `xorm:"org_name varchar(160) notnull unique(identity)"`
	ReportID       string `xorm:"report_id varchar(256) notnull unique(identity)"`
	Domain         string `xorm:"domain varchar(253) notnull unique(identity) index(domain_begin)"`
	BeginAt        int64  `xorm:"begin_at bigint notnull index(domain_begin)"`
	EndAt          int64  `xorm:"end_at bigint notnull"`
	PolicyType     string `xorm:"policy_type varchar(64) notnull"`
	SuccessCount   int64  `xorm:"success_count bigint notnull"`
	FailureCount   int64  `xorm:"failure_count bigint notnull"`
	FailureDetails string `xorm:"failure_details text notnull"`
	FailureTypes   string `xorm:"failure_types text notnull"`
	EmailID        int    `xorm:"email_id notnull index"`
	ReceivedAt     int64  `xorm:"received_at bigint notnull index"`
}

func (TLSReport) TableName() string { return "tls_report" }
