package db

import (
	"testing"

	"github.com/Jinnrry/pmail/models"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestSecondFactorSchemaUpgradeIsAdditiveAndIdempotent(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	defer e.Close()
	if err := SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	// Reproduce the production pre-C21 layout using the same established columns.
	for _, statement := range []string{
		`ALTER TABLE "user" DROP COLUMN factor_version`, `ALTER TABLE "user" DROP COLUMN totp_enabled`,
		`DROP TABLE totp_config`, `DROP TABLE recovery_code`, `DROP TABLE app_password`,
		`INSERT INTO "user" (account, name, password) VALUES ('existing', 'Existing', 'prior-hash')`,
	} {
		if _, err := e.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	var existing models.User
	if ok, err := e.Where("account = ?", "existing").Get(&existing); err != nil || !ok || existing.TOTPEnabled || existing.FactorVersion != 0 || existing.Password != "prior-hash" {
		t.Fatalf("existing credential modified: %v", err)
	}
	if _, err := e.Insert(&models.RecoveryCode{UserID: existing.ID, Digest: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	if n, err := e.Count(&models.RecoveryCode{}); err != nil || n != 1 {
		t.Fatalf("repeated schema sync changed rows: %d/%v", n, err)
	}
}
