package migration

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/internal/backupcatalog"
	"github.com/go-sql-driver/mysql"
)

func TestMigrateKeepsCredentialsAddressLinksAndIMAPUIDs(t *testing.T) {
	open := func() *sql.DB {
		t.Helper()
		c, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	from, to := open(), open()
	fixtures := []struct {
		name, schema string
		row          func(int) (int, int)
		sequence     bool
	}{
		{"totp_config", "user_id INT PRIMARY KEY, last_step INT", func(n int) (int, int) { return n, n + 5 }, false},
		{"recovery_code", "user_id INT, digest INT, PRIMARY KEY(user_id,digest)", func(n int) (int, int) { return 1, n }, false},
		{"app_password", "id INT PRIMARY KEY, user_id INT", func(n int) (int, int) { return n, 1 }, true},
		{"user_email_address", "user_email_id INT, address_id INT, PRIMARY KEY(user_email_id,address_id)", func(n int) (int, int) { return 1, n }, false},
		{"user_email_address_progress", "id INT PRIMARY KEY, through_id INT", func(n int) (int, int) { return n, n + 1 }, false},
		{"imap_mailbox", "id INT PRIMARY KEY, next_uid INT", func(n int) (int, int) { return n, n + 2 }, true},
		{"imap_mailbox_uid", "box_id INT, link_id INT, PRIMARY KEY(box_id,link_id)", func(n int) (int, int) { return 1, n }, false},
	}
	for _, f := range fixtures {
		for _, c := range []*sql.DB{from, to} {
			if _, err := c.Exec("CREATE TABLE " + f.name + " (" + f.schema + ")"); err != nil {
				t.Fatal(err)
			}
		}
		for i := 1; i <= batchSize+3; i++ {
			a, b := f.row(i)
			if _, err := from.Exec("INSERT INTO "+f.name+" VALUES (?,?)", a, b); err != nil {
				t.Fatal(err)
			}
		}
	}
	source, err := from.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer source.Rollback()
	target, err := to.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer target.Rollback()
	for _, f := range fixtures {
		if !slices.Contains(optionalTables, f.name) || backupcatalog.HasIDSequence(f.name) != f.sequence {
			t.Fatalf("bad allowlist/sequence for %s", f.name)
		}
		// The copy SQL's $n bind parameters and quoted identifiers work on
		// SQLite too; production destination and sequence SQL are PostgreSQL.
		if err := copyTable(source, target, config.DBTypeSQLite, f.name, f.name); err != nil {
			t.Fatalf("copy %s: %v", f.name, err)
		}
		var count int
		if err := target.QueryRow("SELECT COUNT(*) FROM " + f.name).Scan(&count); err != nil || count != batchSize+3 {
			t.Fatalf("%s copied %d rows: %v", f.name, count, err)
		}
	}
	if err := copyTable(source, target, config.DBTypeSQLite, "telegram_binding", "telegram_binding"); !errors.Is(err, errSourceTableAbsent) {
		t.Fatalf("missing legacy source should be optional: %v", err)
	}
	if err := copyTable(source, target, config.DBTypeSQLite, "app_password", "missing_target"); err == nil || errors.Is(err, errSourceTableAbsent) {
		t.Fatalf("missing target must not be skipped: %v", err)
	}
	if !isMissingSourceTable(fmt.Errorf("wrapped: %w", &mysql.MySQLError{Number: 1146})) || isMissingSourceTable(&mysql.MySQLError{Number: 1142}) {
		t.Fatal("MySQL missing table classification confused permissions and absence")
	}
}
