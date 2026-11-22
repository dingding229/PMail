package token

import (
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestC7TokenScopeAndExpiryPolicy(t *testing.T) {
	if Scope(models.APIToken{}) != Legacy || !Allows(Legacy, "POST", "/api/token/create") {
		t.Fatal("legacy permissions were not preserved")
	}
	if !Allows(Read, "GET", "/api/attachments/preview/1.0") || !Allows(Read, "POST", "/api/email/list") || Allows(Read, "POST", "/api/email/send") || Allows(Read, "POST", "/api/user/delete") {
		t.Fatal("read scope mismatch")
	}
	if !Allows(Send, "POST", "/api/email/send") || Allows(Send, "POST", "/api/domain/create") || Allows(Admin, "POST", "/api/token/create") {
		t.Fatal("send/admin scope mismatch")
	}
	now := time.Now()
	date, err := expiry(now, nil, false)
	if err != nil || date.Sub(now) != 90*24*time.Hour {
		t.Fatal("default expiry mismatch")
	}
	if date, err := expiry(now, nil, true); err != nil || date != nil {
		t.Fatal("explicit no-expiry rejected")
	}
	if _, err := expiry(now, &now, true); err == nil {
		t.Fatal("conflicting expiry accepted")
	}
}

func TestTokenLastUsedOnlyWritesOncePerMinute(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	defer e.Close()
	old := db.Instance
	db.Instance = e
	defer func() { db.Instance = old }()
	if err = testschema.Sync(e); err != nil {
		t.Fatal(err)
	}
	u := models.User{Account: "reader", Name: "Reader", Password: "fixture"}
	if _, err = e.Insert(&u); err != nil {
		t.Fatal(err)
	}
	raw := "fixture-token"
	row := models.APIToken{UserID: u.ID, TokenHash: hash(raw), Prefix: "fixture", Name: "fixture", Scope: Read}
	if _, err = e.Insert(&row); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Exec("CREATE TABLE token_touches(n INT)"); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Exec("CREATE TRIGGER track_token_touch AFTER UPDATE OF last_used_at ON api_token BEGIN INSERT INTO token_touches VALUES (1); END"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, _, err = Authenticate(raw); err != nil {
			t.Fatal(err)
		}
	}
	verify := func(want int) {
		t.Helper()
		var n int
		if _, err := e.SQL("SELECT count(*) FROM token_touches").Get(&n); err != nil || n != want {
			t.Fatalf("touches=%d want=%d %v", n, want, err)
		}
	}
	verify(1)
	if _, err = e.Table(&models.APIToken{}).ID(row.ID).Update(map[string]any{"last_used_at": time.Now().Add(-time.Minute * 2).UTC().Format(time.DateTime), "last_used_epoch": time.Now().Add(-2 * time.Minute).Unix()}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Authenticate(raw); err != nil {
		t.Fatal(err)
	}
	verify(3)
}
