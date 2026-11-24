package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jinnrry/pmail/models"
	log "github.com/sirupsen/logrus"
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
		var count int64
		duplicate, err := engine.SQL("SELECT COUNT(*) FROM " + dialect.Quoter().Quote(full) + " GROUP BY local_part,domain_id,reservation_key HAVING COUNT(*)>1 LIMIT 1").Get(&count)
		if err != nil {
			return fmt.Errorf("inspect mailbox reservations: %w", err)
		}
		if !duplicate {
			if _, err := engine.Exec(dialect.CreateIndexSQL(full, claim)); err != nil {
				if !IsUniqueViolation(err) {
					return fmt.Errorf("add reservation index: %w", err)
				}
				// A concurrent write may race the preflight.
				duplicate = true
			}
		}
		if duplicate {
			// Keep all rows and leave any legacy protection in place.
			log.Warn("mailbox_address has duplicate reservations; skipping replacement unique index until data is repaired")
			return syncAddressReviewIndex(engine, bean)
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
	return syncAddressReviewIndex(engine, bean)
}

func syncAddressReviewIndex(engine *xorm.Engine, bean *models.MailboxAddress) error {
	if err := ensureMailIndex(engine, bean, "idx_mailbox_address_review", "status,requested_at,id"); err != nil {
		return err
	}
	return dropRedundantModelIndex(engine, bean, "status")
}
