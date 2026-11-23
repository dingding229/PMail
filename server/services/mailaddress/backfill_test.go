package mailaddress

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func fixture(t testing.TB) (owner, a, b int) {
	t.Helper()
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	previous := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = previous; _ = e.Close() })
	if err = db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	u := models.User{Account: "owner", Password: "fixture"}
	d := models.Domain{Name: "partition.test"}
	if _, err := e.Insert(&u, &d); err != nil {
		t.Fatal(err)
	}
	x := models.MailboxAddress{UserID: u.ID, DomainID: d.ID, LocalPart: "main", IsPrimary: true, CanSend: true}
	y := models.MailboxAddress{UserID: u.ID, DomainID: d.ID, LocalPart: "alias", CanSend: true}
	if _, err := e.Insert(&x, &y); err != nil {
		t.Fatal(err)
	}
	return u.ID, x.ID, y.ID
}

func legacy(t testing.TB, owner int, mail models.Email, catchAll bool) models.UserEmail {
	t.Helper()
	if _, err := db.Instance.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	row := models.UserEmail{UserID: owner, EmailID: mail.Id, Status: mail.Status}
	if catchAll {
		row.CatchAllAddresses = sql.NullString{String: `["unknown@partition.test"]`, Valid: true}
	}
	if _, err := db.Instance.Insert(&row); err != nil {
		t.Fatal(err)
	}
	return row
}

func TestAddressBackfillResumesAndKeepsUnmatchedOnlyInAll(t *testing.T) {
	owner, main, alias := fixture(t)
	rows := []models.UserEmail{
		legacy(t, owner, models.Email{To: `[{"Name":"Mailbox","EmailAddress":"MAIN@PARTITION.TEST"}]`, Cc: `Alias <alias@partition.test>`, Bcc: `[{"EmailAddress":"ignored@partition.test"}]`}, false),
		legacy(t, owner, models.Email{Type: consts.EmailTypeSend, FromAddress: "alias@partition.test"}, false),
		legacy(t, owner, models.Email{Status: consts.EmailStatusDrafts, FromAddress: "main@partition.test", To: `alias@partition.test`}, false),
		legacy(t, owner, models.Email{To: `main@partition.test`}, true),
		legacy(t, owner, models.Email{To: `not-an-address`}, false),
	}
	for range BatchSize {
		legacy(t, owner, models.Email{To: `outside@example.test`}, false)
	}
	if n, err := Backfill(t.Context(), db.Instance); err != nil || n != len(rows)+BatchSize {
		t.Fatalf("first run: %d %v", n, err)
	}
	if n, err := Backfill(t.Context(), db.Instance); err != nil || n != 0 {
		t.Fatalf("idempotence: %d %v", n, err)
	}
	ids := []int{}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	views, err := Views(owner, ids)
	if err != nil {
		t.Fatal(err)
	}
	for idx, want := range [][]int{{main, alias}, {alias}, {main}, {}, {}} {
		var got []int
		for _, a := range views[rows[idx].ID] {
			got = append(got, a.ID)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("row %d addresses=%v want %v", idx, got, want)
		}
	}
	defaultCandidates, err := ForMessage(owner, rows[0].EmailID)
	if err != nil || len(defaultCandidates) != 2 || !defaultCandidates[0].IsPrimary {
		t.Fatalf("reply preferences: %+v %v", defaultCandidates, err)
	}
	if err := address.Delete(alias, owner, false); err != nil {
		t.Fatal(err)
	}
	var row models.UserEmail
	if ok, err := db.Instance.ID(rows[1].ID).Get(&row); err != nil || !ok {
		t.Fatalf("deleting alias removed outbound copy: %v", err)
	}
	n, err := db.Instance.Where("address_id=?", alias).Count(&models.UserEmailAddress{})
	if err != nil || n != 0 {
		t.Fatalf("alias matches remain: %d %v", n, err)
	}
	if _, err := db.Instance.Insert(&models.MailboxAddress{UserID: owner, DomainID: 1, LocalPart: "alias"}); err != nil {
		t.Fatal(err)
	}
	late := legacy(t, owner, models.Email{To: `main@partition.test`}, false)
	if n, err := Backfill(t.Context(), db.Instance); err != nil || n != 1 {
		t.Fatalf("old-writer row resumed after checkpoint: %d %v", n, err)
	}
	match, err := Views(owner, []int{rows[1].ID, late.ID})
	if err != nil || len(match[rows[1].ID]) != 0 || len(match[late.ID]) != 1 {
		t.Fatalf("recreated alias must not recover old mail: %v %v", match, err)
	}
}

func BenchmarkBackfill1000LegacyLinks(b *testing.B) {
	for b.Loop() {
		// Run this benchmark with -benchtime=1x: the fixture has one private DB
		// and mock production-shaped indexed rows rather than network latency.
		b.StopTimer()
		owner, _, _ := fixture(b)
		for range 1000 {
			legacy(b, owner, models.Email{To: `main@partition.test`, Cc: `alias@partition.test`}, false)
		}
		b.StartTimer()
		if n, err := Backfill(context.Background(), db.Instance); err != nil || n != 1000 {
			b.Fatalf("%d %v", n, err)
		}
	}
}
