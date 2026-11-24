package db

import (
	"context"
	"fmt"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

func ensureMailIndex(e *xorm.Engine, bean any, name, columns string) error {
	table := e.Dialect().Quoter().Quote(e.TableName(bean, true))
	conditional := " IF NOT EXISTS"
	if e.DriverName() == config.DBTypeMySQL {
		// MySQL lacks CREATE INDEX IF NOT EXISTS. Read the catalog first.
		var count int
		if _, err := e.SQL("SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?", e.TableName(bean), name).Get(&count); err != nil || count > 0 {
			return err
		}
		conditional = ""
	}
	// Names, tables and column lists come only from startup schema definitions.
	_, err := e.Exec(fmt.Sprintf("CREATE INDEX%s %s ON %s (%s)", conditional, e.Quote(name), table, columns))
	return err
}

// Drop only a known model-generated, nonunique index of the expected shape.
// Other indexes installed by an operator are not ours to remove.
func dropRedundantModelIndex(e *xorm.Engine, bean any, column string) error {
	dialect := e.Dialect()
	indexes, err := dialect.GetIndexes(e.DB(), context.Background(), e.TableName(bean))
	if err != nil {
		return err
	}
	if idx := indexes[column]; idx != nil && idx.IsRegular && idx.Type == schemas.IndexType && len(idx.Cols) == 1 && idx.Cols[0] == column {
		_, err = e.Exec(dialect.DropIndexSQL(e.TableName(bean, true), idx))
	}
	return err
}

func syncMailIndexes(e *xorm.Engine) error {
	mail := &models.UserEmail{}
	for _, idx := range []struct{ name, columns string }{
		{"idx_ue_mailbox_order", "user_id,status,group_id,id"},
		{"idx_ue_mailbox_email", "user_id,status,group_id,email_id,id"},
		{"idx_ue_starred_order", "user_id,is_starred,email_id,id"},
		{"idx_ue_unread", "user_id,status,is_read,group_id"},
		{"idx_ue_trash_expiry", "status,user_id,deleted_at,id"},
	} {
		if err := ensureMailIndex(e, mail, idx.name, idx.columns); err != nil {
			return err
		}
	}
	// The existing (user_id,email_id) index already supports owner-only reads
	// and is the right index for the long-poll max/range queries.
	return dropRedundantModelIndex(e, mail, "user_id")
}
