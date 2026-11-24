package telegram

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/netproxy"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

type legacyTelegramConfig struct {
	ID         int    `xorm:"id pk"`
	Token      string `xorm:"token text notnull"`
	Proxy      string `xorm:"proxy text notnull"`
	Username   string `xorm:"username varchar(64) notnull"`
	Generation string `xorm:"generation varchar(40) notnull"`
	Offset     int64  `xorm:"next_offset bigint notnull default(0)"`
}

func (legacyTelegramConfig) TableName() string { return "telegram_config" }

func TestProxyMigrationEnvOverrideAndAPIPatch(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	oldDB, oldConfig := db.Instance, config.Instance
	db.Instance, config.Instance = e, &config.Config{SecretKey: "migration-test-only"}
	t.Cleanup(func() { db.Instance, config.Instance = oldDB, oldConfig; e.Close() })
	if err := e.Sync2(new(legacyTelegramConfig)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PMAIL_TELEGRAM_PROXY", "")
	legacy, err := seal("socks5://olduser:fixture-old-pass@[2001:db8::5]:1081/", "proxy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&legacyTelegramConfig{ID: 1, Proxy: legacy, Generation: "old-generation", Offset: 3}); err != nil {
		t.Fatal(err)
	}
	if err := e.Sync2(new(models.TelegramConfig)); err != nil {
		t.Fatal(err)
	}
	before, _, src, err := netproxy.TelegramProxy(models.TelegramConfig{Proxy: legacy})
	if err != nil || src != "db" {
		t.Fatal("could not read legacy settings")
	}
	if err := MigrateProxy(t.Context()); err != nil {
		t.Fatal(err)
	}
	row, err := getConfig(t.Context())
	if err != nil || row.Proxy != "" || row.ProxyFields == "" || row.Generation != "old-generation" || row.Offset != 3 {
		t.Fatal("proxy not converted or bot state changed")
	}
	if strings.Contains(row.ProxyFields, "fixture-old-pass") {
		t.Fatal("proxy password stored in plaintext")
	}
	if err := MigrateProxy(t.Context()); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := getConfig(t.Context())
	if row.ProxyFields != unchanged.ProxyFields {
		t.Fatal("non-idempotent migration")
	}
	after, _, src, err := netproxy.TelegramProxy(row)
	if err != nil || src != "db" || before.String() != after.String() {
		t.Fatal("migration changed proxy behavior")
	}

	patch, clear, err := netproxy.Patch(json.RawMessage(`{"type":"socks5","host":"2001:db8::5","username":"olduser","port":1082}`), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AdminPut(t.Context(), nil, patch, clear); err != nil {
		t.Fatal(err)
	}
	row, _ = getConfig(t.Context())
	u, _, _, err := netproxy.TelegramProxy(row)
	if err != nil {
		t.Fatal(err)
	}
	pass, _ := u.User.Password()
	if pass != "fixture-old-pass" || u.Port() != "1082" {
		t.Fatal("omitted password not retained")
	}
	t.Setenv("PMAIL_TELEGRAM_PROXY", "https://envuser:fixture-env-pass@env.example:444")
	out, err := AdminGet(t.Context())
	if err != nil || !out.ProxySet || !out.Proxy.PasswordSet || out.ProxySource != "env" || out.Proxy.Host != "env.example" {
		t.Fatal("env override not applied")
	}
	raw, _ := json.Marshal(out)
	for _, forbidden := range []string{"fixture-old-pass", "fixture-env-pass", "password_ciphertext", `"password":`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("proxy details leaked a password")
		}
	}
	if _, err := AdminPut(t.Context(), nil, nil, true); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PMAIL_TELEGRAM_PROXY", "")
	out, err = AdminGet(t.Context())
	if err != nil || out.ProxySet || out.ProxySource != "none" || out.Proxy != nil {
		t.Fatal("clear_proxy did not remove DB settings")
	}
}
