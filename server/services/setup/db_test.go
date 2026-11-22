package setup

import (
	"path/filepath"
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

func TestC7InvalidCandidateDoesNotReplaceSetupConfig(t *testing.T) {
	old := config.ROOT_PATH
	config.ROOT_PATH = t.TempDir()
	t.Cleanup(func() { config.ROOT_PATH = old })
	t.Setenv("PMAIL_DB_DSN", "")
	t.Setenv("PMAIL_DB_TYPE", "")
	t.Setenv("PMAIL_DB_PASSWORD_FILE", "")
	t.Setenv("PMAIL_WEB_ROOT", filepath.Join(config.ROOT_PATH, "web"))
	baseline := &config.Config{DbType: config.DBTypePostgres, DbDSN: "postgres://user:fixture@db.invalid:5432/example"}
	if err := config.WriteConfig(baseline); err != nil {
		t.Fatal(err)
	}
	if err := SetDatabaseSettings(nil, config.DBTypePostgres, "invalid DSN"); err == nil {
		t.Fatal("invalid candidate was accepted")
	}
	got, err := config.ReadConfig()
	if err != nil || got.DbDSN != baseline.DbDSN {
		t.Fatalf("old configuration was overwritten: %v", err)
	}
	if _, dsn, err := GetDatabaseSettings(nil); err != nil || dsn != "" {
		t.Fatalf("database settings exposed DSN: %v", err)
	}
}
