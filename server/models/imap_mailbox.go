package models

// IMAPMailbox stores a separate UID namespace for a logical mailbox.
// Its persisted ID, with the high bit set, is the UIDVALIDITY.
type IMAPMailbox struct {
	ID      int    `xorm:"id pk autoincr"`
	UserID  int    `xorm:"user_id notnull unique(owner_key)"`
	BoxKey  string `xorm:"box_key varchar(32) notnull unique(owner_key)"`
	NextUID int64  `xorm:"next_uid bigint notnull default(1)"`
}

func (IMAPMailbox) TableName() string { return "imap_mailbox" }

type IMAPMailboxUID struct {
	BoxID  int   `xorm:"box_id pk notnull index(uid_key)"`
	LinkID int   `xorm:"link_id pk notnull index"`
	UID    int64 `xorm:"uid bigint notnull index(uid_key)"`
}

func (IMAPMailboxUID) TableName() string { return "imap_mailbox_uid" }
