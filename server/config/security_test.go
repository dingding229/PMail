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

func TestVerifiedStagedWebRootKeepsSensitivePathChecks(t *testing.T) {
	old, previous := ROOT_PATH, stagedWebRoot
	t.Cleanup(func() { ROOT_PATH, stagedWebRoot = old, previous })
	ROOT_PATH = t.TempDir()
	stagedWebRoot = ""
	site := filepath.Join(ROOT_PATH, "config", "update", "staged", "v0.0.4", "web")
	if err := os.MkdirAll(site, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PMAIL_WEB_ROOT", site)
	if err := ValidateWebRoot(&Config{}); err == nil {
		t.Fatal("an env override cannot expose the staged config subtree")
	}
	if err := UseStagedWebRoot("v0.0.4"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWebRoot(&Config{}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWebRoot(&Config{DbType: DBTypeSQLite, DbDSN: filepath.Join(site, "db.sqlite")}); err == nil {
		t.Fatal("staged served files must still not overlap secrets or databases")
	}
}

func TestValidateWebRootRecreatesMissingDataDirectories(t *testing.T) {
	old := ROOT_PATH
	root := t.TempDir()
	ROOT_PATH = root
	t.Cleanup(func() { ROOT_PATH = old })
	if err := os.MkdirAll(filepath.Join(root, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	// Root-level aliases left dangling, as after a restore removed the target.
	for _, name := range []string{"backups", "exports"} {
		if err := os.Symlink(filepath.Join("config", name), filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"backups", "exports"} {
		_ = os.Mkdir(filepath.Join(root, "config", name), 0700)
		_ = os.Remove(filepath.Join(root, "config", name))
	}
	t.Setenv("PMAIL_WEB_ROOT", t.TempDir())
	_ = ValidateWebRoot(&Config{})
	for _, name := range []string{"backups", "exports"} {
		if _, err := filepath.EvalSymlinks(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s alias still dangling: %v", name, err)
		}
	}
}
