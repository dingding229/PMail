package models

// MailAttachmentChunk stores bounded binary reads separately from the legacy
// MIME reconstruction source. Web downloads never fetch sibling attachments.
type MailAttachmentChunk struct {
	EmailID int    `xorm:"email_id pk"`
	Part    int    `xorm:"part_id pk"`
	Chunk   int    `xorm:"chunk_no pk"`
	Data    []byte `xorm:"data BLOB notnull"`
}

func (MailAttachmentChunk) TableName() string { return "mail_attachment_chunk" }

type MailAttachmentInfo struct {
	ID          int    `xorm:"id pk autoincr" json:"-"`
	EmailID     int    `xorm:"email_id notnull unique('part')" json:"-"`
	Index       int    `xorm:"part notnull unique('part')" json:"index"`
	Filename    string `xorm:"filename text" json:"filename"`
	ContentType string `xorm:"content_type text" json:"content_type"`
	ContentID   string `xorm:"content_id text" json:"content_id"`
	Charset     string `xorm:"charset text" json:"-"`
	Size        int    `xorm:"size notnull" json:"size"`
	Inline      bool   `xorm:"inline_image notnull" json:"inline"`
	UTF8        bool   `xorm:"valid_utf8 notnull default(false)" json:"-"`
	Previewable bool   `xorm:"previewable notnull" json:"previewable"`
}

func (MailAttachmentInfo) TableName() string { return "mail_attachment_info" }
