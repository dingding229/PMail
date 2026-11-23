package db

import (
	"testing"

	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

func TestRetiredMailboxStartupMigrationPreservesImportedMail(t *testing.T) {
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	engine.SetMaxOpenConns(1)
	// Simulate an older singleton with DNSBL settings but no catch-all columns.
	for _, q := range []string{
		"CREATE TABLE inbound_settings (id INTEGER NOT NULL PRIMARY KEY, dnsbl_enabled INTEGER NOT NULL, dnsbl_zones TEXT NOT NULL, default_spam_threshold DOUBLE NOT NULL, clamd_enabled INTEGER NOT NULL, clamd_addr TEXT NOT NULL, risk_block_dangerous_attachments INTEGER NOT NULL)",
		"INSERT INTO inbound_settings VALUES (1, 1, '[\"existing.test\"]', 7, 0, '', 1)",
	} {
		if _, err := engine.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := SyncSchema(engine); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"CREATE TABLE external_account (id INTEGER PRIMARY KEY, password_cipher TEXT)",
		"INSERT INTO external_account VALUES (1, 'ciphertext-fixture')",
		"CREATE TABLE external_seen (account_id INTEGER)",
		"INSERT INTO external_seen VALUES (1)",
		"CREATE TABLE external_cursor (account_id INTEGER)",
		"INSERT INTO external_cursor VALUES (1)",
	} {
		if _, err := engine.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	mail := models.Email{Subject: "Already fetched", AuthUnchecked: true}
	if _, err := engine.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	link := models.UserEmail{UserID: 8, EmailID: mail.Id}
	if _, err := engine.Insert(&link); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := initializeSchemaAndData(engine, "test", true); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"external_cursor", "external_seen", "external_account"} {
			if exists, err := engine.IsTableExist(table); err != nil || exists {
				t.Fatalf("%s exists=%t, err=%v", table, exists, err)
			}
		}
		var cfg models.InboundSettings
		if ok, err := engine.ID(1).Get(&cfg); err != nil || !ok || !cfg.DNSBLEnabled || cfg.DefaultSpamThreshold != 7 || cfg.CatchAllEnabled.Valid || cfg.CatchAllRecipientID != 0 {
			t.Fatalf("legacy settings were changed or catch-all defaults were not migrated: found=%t settings=%+v err=%v", ok, cfg, err)
		}
		var got models.Email
		if ok, err := engine.ID(mail.Id).Get(&got); err != nil || !ok || got.Subject != mail.Subject || !got.AuthUnchecked {
			t.Fatalf("already fetched mail changed: found=%t err=%v", ok, err)
		}
		if ok, err := engine.Where("user_id=? AND email_id=?", link.UserID, link.EmailID).Exist(&models.UserEmail{}); err != nil || !ok {
			t.Fatalf("imported mailbox link changed: found=%t err=%v", ok, err)
		}
	}
}
