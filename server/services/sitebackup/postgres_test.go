//go:build integration

package sitebackup

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
)

func TestSQLiteArchiveIntoPostgres(t *testing.T) {
	dsn := os.Getenv("PMAIL_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PMAIL_TEST_PG_DSN is not set")
	}
	parent, err := db.Open(&config.Config{DbType: "postgres", DbDSN: dsn}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	schema := fmt.Sprintf("pmail_c33_%x", time.Now().UnixNano())
	if _, err = parent.Exec("CREATE SCHEMA " + parent.Quote(schema)); err != nil {
		t.Fatal(err)
	}
	defer parent.Exec("DROP SCHEMA " + parent.Quote(schema) + " CASCADE")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	e, err := db.Open(&config.Config{DbType: "postgres", DbDSN: u.String()}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.SetSchema(schema)
	if err := db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	s, d := sqliteStore(t), Store{e, t.TempDir(), "1.0.0", ""}
	withFixture(t, s)
	if err = d.Restore(t.Context(), bytes.NewReader(archive(t, s, false)), testPass, nil, nil); err != nil {
		t.Fatal(err)
	}
	var x models.User
	if ok, err := e.ID(5).Get(&x); err != nil || !ok || !x.TOTPEnabled {
		t.Fatalf("portable account: %v", err)
	}
	var b models.OutboundPayload
	if ok, err := e.ID(17).Get(&b); err != nil || !ok || !bytes.Equal(b.Data, []byte{0, 0x80, 0xff}) {
		t.Fatalf("portable binary: %v", err)
	}
	row := models.Email{Subject: "after restore"}
	if _, err := e.Insert(&row); err != nil || row.Id != 8 {
		t.Fatalf("sequence: %d/%v", row.Id, err)
	}
}
