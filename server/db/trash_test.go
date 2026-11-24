package db

import (
	"testing"

	"github.com/Jinnrry/pmail/models"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestTrashSchemaAndLegacyBackfill(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.SetMaxOpenConns(1)
	for _, q := range []string{
		`CREATE TABLE user_email (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL, email_id INTEGER NOT NULL, status INTEGER NOT NULL DEFAULT 0, group_id INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO user_email (user_id,email_id,status,group_id) VALUES (1,10,3,12),(1,11,0,0),(1,12,0,2000000003)`,
	} {
		if _, err = e.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	if err := backfillTrash(e, 12345); err != nil {
		t.Fatal(err)
	}
	if err := SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	if err := backfillTrash(e, 23456); err != nil {
		t.Fatal(err)
	}
	var rows []models.UserEmail
	if err := e.Asc("id").Find(&rows); err != nil || len(rows) != 3 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	for _, i := range []int{0, 2} {
		r := rows[i]
		if r.DeletedAt == nil || *r.DeletedAt != 12345 || r.PrevStatus != nil || r.PrevGroupID != nil || r.Status != 3 || r.GroupId != 0 {
			t.Fatalf("migrated row %+v", r)
		}
	}
	if rows[1].DeletedAt != nil {
		t.Fatalf("live row incorrectly stamped: %+v", rows[1])
	}
	var n int
	if _, err := e.SQL(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='user_email' AND sql LIKE '%deleted_at%'`).Get(&n); err != nil || n != 1 {
		t.Fatalf("deleted_at index=%d err=%v", n, err)
	}
}
