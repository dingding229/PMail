package db

import (
	"fmt"
	"github.com/Jinnrry/pmail/config"
	"xorm.io/xorm"
)

func syncMailIndexes(e *xorm.Engine) error {
	for _, idx := range []struct{ name, columns string }{
		{"idx_ue_mailbox_order", "user_id,status,group_id,id"},
		{"idx_ue_mailbox_email", "user_id,status,group_id,email_id,id"},
		{"idx_ue_starred_order", "user_id,is_starred,email_id,id"},
	} {
		if e.DriverName() == config.DBTypeMySQL {
			var count int
			if _, err := e.SQL("SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='user_email' AND index_name=?", idx.name).Get(&count); err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			if _, err := e.Exec(fmt.Sprintf("CREATE INDEX %s ON user_email (%s)", idx.name, idx.columns)); err != nil {
				return err
			}
		} else if _, err := e.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON user_email (%s)", idx.name, idx.columns)); err != nil {
			return err
		}
	}
	return nil
}
