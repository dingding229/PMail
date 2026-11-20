package db

import (
	"database/sql"
	"fmt"

	"github.com/Jinnrry/pmail/config"
	"xorm.io/xorm"
)

// Sync2 adds columns, but existing char/varchar columns must be widened without losing data.
func widenUserColumns(engine *xorm.Engine) error {
	query := ""
	schema := engine.Dialect().URI().Schema
	if schema == "" {
		schema = "public"
	}
	switch engine.DriverName() {
	case config.DBTypePostgres:
		query = "SELECT character_maximum_length FROM information_schema.columns WHERE table_schema=$1 AND table_name='user' AND column_name=$2"
	case config.DBTypeMySQL:
		query = "SELECT character_maximum_length FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='user' AND column_name=?"
	default:
		// SQLite doesn't impose VARCHAR/CHAR lengths.
		return nil
	}
	for _, c := range []struct {
		name string
		size int
	}{{"password", 100}, {"name", 50}} {
		args := []any{c.name}
		if engine.DriverName() == config.DBTypePostgres {
			args = append([]any{schema}, args...)
		}
		var width sql.NullInt64
		if err := engine.DB().DB.QueryRow(query, args...).Scan(&width); err != nil {
			return fmt.Errorf("inspect user.%s: %w", c.name, err)
		}
		if !width.Valid || width.Int64 >= int64(c.size) {
			continue
		}
		var alter string
		if engine.DriverName() == config.DBTypePostgres {
			alter = fmt.Sprintf("ALTER TABLE %s.%s ALTER COLUMN %s TYPE VARCHAR(%d)", engine.Quote(schema), engine.Quote("user"), engine.Quote(c.name), c.size)
		} else {
			alter = fmt.Sprintf("ALTER TABLE `user` MODIFY COLUMN `%s` VARCHAR(%d) NOT NULL", c.name, c.size)
		}
		if _, err := engine.Exec(alter); err != nil {
			return fmt.Errorf("widen user.%s: %w", c.name, err)
		}
	}
	return nil
}
