package db

import (
	"fmt"

	"xorm.io/xorm"
)

// Drop only the retired fetcher's credentials and synchronization state. Its
// imported messages, mailbox links and attachments use the ordinary mail tables.
// Run at startup even if the version marker was already written. Each DDL is
// independently repeatable, including on MySQL, where DDL can auto-commit.
func dropExternalMailboxTables(engine *xorm.Engine) error {
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS external_cursor",
		"DROP TABLE IF EXISTS external_seen",
		"DROP TABLE IF EXISTS external_account",
	} {
		if _, err := engine.Exec(stmt); err != nil {
			return fmt.Errorf("remove retired external mailbox configuration: %w", err)
		}
	}
	return nil
}
