package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestC7WebRootRejectsSensitiveOverlap(t *testing.T) {
	old := ROOT_PATH
	ROOT_PATH = t.TempDir()
	t.Cleanup(func() { ROOT_PATH = old })
	t.Setenv("PMAIL_DB_PASSWORD_FILE", "")
	private := filepath.Join(ROOT_PATH, "config")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PMAIL_WEB_ROOT", ROOT_PATH)
	if ValidateWebRoot(&Config{}) == nil {
		t.Fatal("root containing config was accepted")
	}
	link := filepath.Join(ROOT_PATH, "site-link")
	if err := os.Symlink(private, link); err == nil {
		t.Setenv("PMAIL_WEB_ROOT", filepath.Join(link, "future-public"))
		if ValidateWebRoot(&Config{}) == nil {
			t.Fatal("symlink into config was accepted")
		}
	}
	t.Setenv("PMAIL_WEB_ROOT", filepath.Join(ROOT_PATH, "dist"))
	if err := ValidateWebRoot(&Config{}); err != nil {
		t.Fatal(err)
	}
	if ValidateWebRoot(&Config{DbType: DBTypeSQLite, DbDSN: "file:dist%2fpmail.db?cache=shared"}) == nil {
		t.Fatal("SQLite data in web root was accepted")
	}
}

func TestC7LogFormatterEscapesLines(t *testing.T) {
	e := log.NewEntry(log.New())
	e.Message = "first\r\nsecond\x00"
	b, err := (&logFormatter{}).Format(e)
	if err != nil || strings.Count(string(b), "\n") != 1 || !strings.Contains(string(b), `first\r\nsecond\0`) {
		t.Fatalf("formatter output: %q (%v)", b, err)
	}
}
