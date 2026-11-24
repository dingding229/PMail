package models

type TrashSettings struct {
	ID            int `xorm:"id pk"`
	RetentionDays int `xorm:"trash_retention_days int notnull default(30)"`
}

func (TrashSettings) TableName() string { return "trash_settings" }
