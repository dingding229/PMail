//go:build perfseed

// Package perfseed owns the deterministic, disposable SQLite mail benchmark fixture.
package perfseed

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

const Total, Attachments, Inbox = 50000, 200, 20000

func Seed(b testing.TB) models.User {
	b.Helper()
	e, err := xorm.NewEngine("sqlite", filepath.Join(b.TempDir(), "perf.sqlite"))
	if err != nil {
		b.Fatal(err)
	}
	old := db.Instance
	db.Instance = e
	e.SetMaxOpenConns(1)
	b.Cleanup(func() { db.Instance = old; _ = e.Close() })
	if err = db.SyncSchema(e); err != nil {
		b.Fatal(err)
	}
	reader, other := models.User{Account: "benchmark", Name: "Benchmark", Password: "unused"}, models.User{Account: "other", Name: "Other", Password: "unused"}
	if _, err = e.Insert(&reader, &other); err != nil {
		b.Fatal(err)
	}
	payload, err := json.Marshal([]parsemail.Attachment{{Filename: "fixture.txt", ContentType: "text/plain", Content: bytes.Repeat([]byte("A"), 64<<10)}})
	if err != nil {
		b.Fatal(err)
	}
	for start := 1; start <= Total; start += 250 {
		_, err = e.Transaction(func(tx *xorm.Session) (any, error) {
			for n := start; n < min(start+250, Total+1); n++ {
				owner := other.ID
				if n > Total-Inbox {
					owner = reader.ID
				}
				subject := fmt.Sprintf("Fixture message %05d", n)
				if n%100 == 0 {
					subject += " needle"
				}
				stamp := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Second)
				row := models.Email{Id: n, Subject: subject, FromAddress: "sender@example.test", MsgID: fmt.Sprintf("%d@example.test", n), SendDate: stamp,
					Text: sql.NullString{String: strings.Repeat("Synthetic message content. ", 100), Valid: true}, Html: sql.NullString{String: "<p>" + strings.Repeat("Synthetic HTML. ", 240) + "</p>", Valid: true}, Size: 7000}
				if n > Total-Attachments {
					row.Attachments = string(payload)
					row.Size += len(payload)
				}
				if _, err := tx.Insert(&row); err != nil {
					return nil, err
				}
				if _, err := tx.Insert(&models.UserEmail{UserID: owner, EmailID: n, IsStarred: n%37 == 0}); err != nil {
					return nil, err
				}
			}
			return nil, nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	if _, err = e.Exec("ANALYZE"); err != nil {
		b.Fatal(err)
	}
	return reader
}
