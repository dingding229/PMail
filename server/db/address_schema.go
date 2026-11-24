package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

// Add the columns with active/0 defaults for existing rows. Install the new
// unique index BEFORE dropping the old one: MySQL DDL commits independently,
// and an interrupted upgrade must never leave active addresses unprotected.
func syncAddressSchema(engine *xorm.Engine) error {
	bean := &models.MailboxAddress{}
	if _, err := engine.SyncWithOptions(xorm.SyncOptions{IgnoreConstrains: true, IgnoreDropIndices: true}, bean); err != nil {
		return err
	}
	table, err := engine.TableInfo(bean)
	if err != nil {
		return err
	}
	claim := table.Indexes["address_claim"]
	if claim == nil {
		return errors.New("missing mailbox address reservation index definition")
	}
	dialect := engine.Dialect()
	short, full := engine.TableName(bean), engine.TableName(bean, true)
	indexes, err := dialect.GetIndexes(engine.DB(), context.Background(), short)
	if err != nil {
		return err
	}
	present := false
	for _, idx := range indexes {
		if claim.Equal(idx) {
			present = true
			break
		}
	}
	if !present {
		if _, err := engine.Exec(dialect.CreateIndexSQL(full, claim)); err != nil {
			return fmt.Errorf("add reservation index: %w", err)
		}
	}
	legacy := schemas.NewIndex("address", schemas.UniqueType)
	legacy.AddColumn("local_part", "domain_id")
	for _, idx := range indexes {
		if legacy.Equal(idx) {
			if _, err := engine.Exec(dialect.DropIndexSQL(full, idx)); err != nil {
				return fmt.Errorf("drop legacy mailbox address index: %w", err)
			}
		}
	}
	return nil
}
