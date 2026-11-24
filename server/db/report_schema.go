package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

// Create the new child table with a foreign key in its initial DDL. SQLite
// cannot add a foreign key with ALTER TABLE. Let xorm generate dialect types,
// primary keys and auto-increment; Sync2 below installs the indexes.
func syncDMARCRecord(e *xorm.Engine, model *models.DMARCRecord) error {
	name := model.TableName()
	exists, err := e.IsTableExist(name)
	if err != nil {
		return err
	}
	if !exists {
		t, err := e.TableInfo(model)
		if err != nil {
			return err
		}
		ddl, _, err := e.Dialect().CreateTableSQL(context.Background(), e.DB(), t, name)
		if err != nil {
			return err
		}
		end := strings.LastIndexByte(ddl, ')')
		if end < 0 {
			return fmt.Errorf("invalid CREATE TABLE for %s", name)
		}
		ddl = ddl[:end] + ", CONSTRAINT fk_dmarc_record_report FOREIGN KEY (report_id) REFERENCES dmarc_report (id) ON DELETE CASCADE" + ddl[end:]
		if _, err := e.Exec(ddl); err != nil {
			return err
		}
	}
	// MySQL may create a supporting index for the foreign key on its own.
	_, err = e.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, model)
	return err
}
