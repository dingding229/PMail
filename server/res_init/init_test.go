package res_init

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
)

func TestEnsureMailDomainUsesExistingDatabase(t *testing.T) {
	cfg := domainSetupTestConfig(t)
	if _, err := db.Instance.Insert(&models.Domain{Name: "secondary.test"}, &models.Domain{Name: "primary.test", IsDefault: true}); err != nil {
		t.Fatal(err)
	}
	for range 2 { // A listener reload reconstructs effective config from disk/env.
		config.Instance = &cfg
		cfg.Domain, cfg.WebDomain, cfg.Domains = "", "", []string{"extra.test"}
		if err := ensureMailDomain(func() error { t.Fatal("existing domain must not enter setup"); return nil }); err != nil {
			t.Fatal(err)
		}
		if cfg.Domain != "primary.test" || cfg.WebDomain != "mail.primary.test" || !slices.Equal(cfg.Domains, []string{"primary.test", "extra.test"}) {
			t.Fatalf("wrong initial domain settings: %q %q %v", cfg.Domain, cfg.WebDomain, cfg.Domains)
		}
	}
}

func TestEnsureMailDomainDoesNotReopenSetupAfterSaving(t *testing.T) {
	cfg := domainSetupTestConfig(t)
	config.Instance = &cfg
	t.Setenv("PMAIL_DOMAINS", "extra.test")
	if err := ensureMailDomain(func() error { return saveMailDomain("primary.test") }); err != nil {
		t.Fatal(err)
	}
	got := config.Instance
	if got.Domain != "primary.test" || got.WebDomain != "mail.primary.test" || got.HttpsEnabled != 2 || !slices.Equal(got.Domains, []string{"primary.test", "extra.test"}) {
		t.Fatalf("saved settings did not survive reload: %q %q %d %v", got.Domain, got.WebDomain, got.HttpsEnabled, got.Domains)
	}
	if err := ensureMailDomain(func() error { t.Fatal("persisted domain must not reenter setup"); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureMailDomainRejectsLegacyUsersWithoutADomain(t *testing.T) {
	cfg := domainSetupTestConfig(t)
	config.Instance = &cfg
	if _, err := db.Instance.Insert(&models.User{Account: "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := ensureMailDomain(func() error { t.Fatal("existing users must not enter setup"); return nil }); err == nil {
		t.Fatal("missing original domain must be reported")
	}
}

func domainSetupTestConfig(t *testing.T) config.Config {
	t.Helper()
	oldRoot, oldCfg, oldDB, oldInit := config.ROOT_PATH, config.Instance, db.Instance, config.IsInit
	root := t.TempDir()
	t.Setenv("PMAIL_DB_DSN", filepath.Join(root, "test.db"))
	t.Setenv("PMAIL_DB_TYPE", config.DBTypeSQLite)
	for _, name := range []string{"PMAIL_DOMAIN", "PMAIL_WEB_DOMAIN", "PMAIL_WEB_MODE", "PMAIL_HTTPS_ENABLED", "PMAIL_SSL_TYPE"} {
		t.Setenv(name, "")
	}
	t.Setenv("PMAIL_WEB_ROOT", filepath.Join(root, "web"))
	config.ROOT_PATH = root + string(os.PathSeparator)
	cfg, err := config.ReadEffectiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	engine, err := db.Open(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		engine.Close()
		config.ROOT_PATH, config.Instance, db.Instance, config.IsInit = oldRoot, oldCfg, oldDB, oldInit
	})
	db.Instance = engine
	if err := engine.Sync(new(models.Domain), new(models.User)); err != nil {
		t.Fatal(err)
	}
	return *cfg
}
