package config

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func cleanEffectiveConfigEnv(t *testing.T) {
	t.Helper()
	oldRoot := ROOT_PATH
	ROOT_PATH = t.TempDir()
	t.Cleanup(func() { ROOT_PATH = oldRoot })
	for _, key := range []string{
		"PMAIL_DOMAIN", "PMAIL_DOMAINS", "PMAIL_WEB_DOMAIN", "PMAIL_WEB_MODE", "PMAIL_SSL_TYPE",
		"PMAIL_HTTPS_ENABLED", "PMAIL_HTTP_PORT", "PMAIL_HTTPS_PORT", "PMAIL_SPAM_FILTER_LEVEL",
		"PMAIL_CATCH_ALL", "PMAIL_ALLOW_USER_ALIAS", "PMAIL_DB_TYPE", "PMAIL_DB_DSN",
		"PMAIL_DB_PASSWORD_FILE", "PMAIL_DB_HOST", "PMAIL_DB_PORT", "PMAIL_DB_USER", "PMAIL_DB_NAME",
	} {
		t.Setenv(key, "")
	}
}

func TestWebModeDefaultsAndOverrides(t *testing.T) {
	cases := []struct {
		name, mode, webDomain, ssl, https string
		wantDomain, wantSSL               string
		wantHTTPS                         int
	}{
		{"default", "", "", "", "", "mail.example.com", "0", 2},
		{"proxy", "proxy", "", "", "", "mail.example.com", "0", 2},
		{"public", "public", "", "", "", "mail.example.com", "0", 0},
		{"explicit overrides", "public", "web.example.com", "1", "2", "web.example.com", "1", 2},
		{"proxy HTTPS override", "proxy", "", "2", "0", "mail.example.com", "2", 0},
		{"legacy TLS disabled", "", "", "3", "", "mail.example.com", "3", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanEffectiveConfigEnv(t)
			t.Setenv("PMAIL_DOMAIN", " example.com ")
			t.Setenv("PMAIL_WEB_MODE", tc.mode)
			t.Setenv("PMAIL_WEB_DOMAIN", tc.webDomain)
			t.Setenv("PMAIL_SSL_TYPE", tc.ssl)
			t.Setenv("PMAIL_HTTPS_ENABLED", tc.https)
			cfg, err := ReadEffectiveConfig()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.WebDomain != tc.wantDomain || cfg.SSLType != tc.wantSSL || cfg.HttpsEnabled != tc.wantHTTPS {
				t.Fatalf("got domain=%q SSL=%q HTTPS=%d", cfg.WebDomain, cfg.SSLType, cfg.HttpsEnabled)
			}
		})
	}
	t.Run("invalid mode", func(t *testing.T) {
		cleanEffectiveConfigEnv(t)
		t.Setenv("PMAIL_WEB_MODE", "invalid")
		if _, err := ReadEffectiveConfig(); err == nil {
			t.Fatal("invalid web mode accepted")
		}
	})
}

func TestPostgresPasswordFileAndExplicitDSN(t *testing.T) {
	useCurrentDBSecretOwner(t)
	cleanEffectiveConfigEnv(t)
	const raw = "has space:@/?#%+&$ "
	file := filepath.Join(t.TempDir(), "db_password")
	if err := InitDatabasePassword(file, raw); err != nil {
		t.Fatal(err)
	}
	if err := InitDatabasePassword(file, ""); err != nil {
		t.Fatal(err)
	}
	if err := InitDatabasePassword(file, "other password"); err == nil {
		t.Fatal("refusing to overwrite a provisioned password must return an error")
	}
	t.Setenv("PMAIL_DB_PASSWORD_FILE", file)
	t.Setenv("PMAIL_DB_USER", "mail:user")
	t.Setenv("PMAIL_DB_NAME", "mail/db")
	cfg, err := ReadEffectiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := url.Parse(cfg.DbDSN)
	if err != nil {
		t.Fatal(err)
	}
	password, ok := dsn.User.Password()
	if !ok || password != raw || dsn.User.Username() != "mail:user" || dsn.Host != "postgres:5432" ||
		dsn.EscapedPath() != "/mail%2Fdb" || !strings.Contains(cfg.DbDSN, "mail%3Auser:has%20space%3A%40%2F%3F%23%25") {
		t.Fatal("DSN did not encode and round-trip credentials and database name")
	}
	t.Setenv("PMAIL_DB_DSN", "postgres://other:5432/pmail")
	t.Setenv("PMAIL_DB_PASSWORD_FILE", filepath.Join(t.TempDir(), "does-not-exist"))
	cfg, err = ReadEffectiveConfig()
	if err != nil || cfg.DbDSN != "postgres://other:5432/pmail" {
		t.Fatalf("explicit DSN must not read the password file: %v", err)
	}
}

func TestAutomaticDatabasePasswordPersists(t *testing.T) {
	useCurrentDBSecretOwner(t)
	file := filepath.Join(t.TempDir(), "db_password")
	if err := InitDatabasePassword(file, ""); err != nil {
		t.Fatal(err)
	}
	first, err := readSecret(file)
	if err != nil || len(first) != 48 {
		t.Fatalf("expected a 48-character generated password: %v", err)
	}
	if err := InitDatabasePassword(file, ""); err != nil {
		t.Fatal(err)
	}
	second, err := readSecret(file)
	if err != nil || second != first {
		t.Fatalf("password changed after initialization: %v", err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("password file must be mode 0600: %v", err)
	}
}

func useCurrentDBSecretOwner(t *testing.T) {
	t.Helper()
	uid, gid := os.Geteuid(), os.Getegid()
	if uid < 0 || gid < 0 {
		t.Skip("database secret ownership requires numeric Unix user and group IDs")
	}
	t.Setenv("PMAIL_DB_SECRET_UID", strconv.Itoa(uid))
	t.Setenv("PMAIL_DB_SECRET_GID", strconv.Itoa(gid))
}

func TestInitDatabasePasswordRepairsExistingPermissions(t *testing.T) {
	useCurrentDBSecretOwner(t)
	dir := filepath.Join(t.TempDir(), "secrets")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "db_password")
	const original = "existing-password\n"
	if err := os.WriteFile(file, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if err := InitDatabasePassword(file, ""); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("existing file was not reset to mode 0600: %v", err)
	}
	info, err = os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0711 {
		t.Fatalf("existing directory must permit traversal without granting listing: %v", err)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != original {
		t.Fatalf("permission repair must not change the existing password: %v", err)
	}
}
