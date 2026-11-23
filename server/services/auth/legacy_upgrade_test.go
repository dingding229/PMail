package auth

import (
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/password"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestAuthenticateMigratesHistoricalPasswordBeforeReturning(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	defer e.Close()
	old := db.Instance
	db.Instance = e
	defer func() { db.Instance = old }()
	if err = db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	const secret = "legacy-migration-fixture"
	const fixture = "7db29f4a4f29ed2575e30ae8da139b16"
	u := models.User{Account: "legacy-fixture", Name: "Legacy", Password: fixture}
	if _, err = e.Insert(&u); err != nil {
		t.Fatal(err)
	}
	if _, err = Authenticate(u.Account, "invalid secret", "192.0.2.51"); err == nil {
		t.Fatal("wrong password accepted")
	}
	var row models.User
	if _, err = e.ID(u.ID).Get(&row); err != nil || row.Password != fixture {
		t.Fatalf("failure changed credential: %v", err)
	}
	authenticated, err := Authenticate(u.Account, secret, "192.0.2.51")
	if err != nil {
		t.Fatal(err)
	}
	row = models.User{}
	if ok, err := e.ID(u.ID).Get(&row); err != nil || !ok || !strings.HasPrefix(row.Password, "$2") || row.Password != authenticated.Password {
		t.Fatalf("bcrypt upgrade was not persisted: %v", err)
	}
	if valid, legacy := password.Verify(row.Password, secret); !valid || legacy {
		t.Fatal("upgraded password is not verifiable as bcrypt")
	}
}
