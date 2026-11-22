package setup

import (
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
)

func TestLegacySetupReadsExistingAdminOnly(t *testing.T) {
	engine, err := db.Open(&config.Config{DbType: config.DBTypeSQLite, DbDSN: ":memory:"}, false)
	if err != nil {
		t.Fatal(err)
	}
	prev := db.Instance
	db.Instance = engine
	t.Cleanup(func() { db.Instance = prev; _ = engine.Close() })
	if err := engine.Sync2(&models.User{}); err != nil {
		t.Fatal(err)
	}
	if account, err := GetAdminAccount(); err != nil || account != "" {
		t.Fatalf("empty database: account=%q, err=%v", account, err)
	}
	if _, err := engine.Insert(&models.User{Account: "root", Name: "Root", Password: "stored", IsAdmin: 1}); err != nil {
		t.Fatal(err)
	}
	if account, err := GetAdminAccount(); err != nil || account != "root" {
		t.Fatalf("existing administrator: account=%q, err=%v", account, err)
	}
}
