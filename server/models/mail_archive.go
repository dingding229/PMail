package models

type MailArchiveSettings struct {
	ID       int   `xorm:"id pk"`
	MaxBytes int64 `xorm:"max_upload_bytes bigint notnull"`
}

func (MailArchiveSettings) TableName() string { return "mail_archive_settings" }
