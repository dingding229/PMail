package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/password"
)

func TestBootstrapAdminGeneratesPasswordOnlyOnce(t *testing.T) {
	root := t.TempDir()
	oldRoot, oldDB := config.ROOT_PATH, db.Instance
	config.ROOT_PATH = root
	t.Cleanup(func() { config.ROOT_PATH, db.Instance = oldRoot, oldDB })
	engine, err := db.Open(&config.Config{DbType: config.DBTypeSQLite, DbDSN: ":memory:"}, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	if err := engine.Sync2(&models.User{}); err != nil {
		t.Fatal(err)
	}
	db.Instance = engine
	if err := ensureBootstrapAdmin("admin", ""); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "config", "initial_admin_password")
	raw, err := os.ReadFile(file)
	if err != nil || len(strings.TrimSpace(string(raw))) != 48 {
		t.Fatalf("expected a generated password file: %v", err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("administrator password file must be mode 0600: %v", err)
	}
	var initial models.User
	if found, err := engine.Where("account = ?", "admin").Get(&initial); err != nil || !found {
		t.Fatalf("administrator not created: %v", err)
	}
	if valid, _ := password.Verify(initial.Password, strings.TrimSpace(string(raw))); !valid || initial.IsAdmin != 1 {
		t.Fatal("stored admin credentials do not match the generated password")
	}
	// An administrator can remove the plaintext file after changing the password.
	// Bootstrapping the existing account must not recreate the file or update its hash.
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := ensureBootstrapAdmin("admin", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("password file unexpectedly recreated: %v", err)
	}
	var after models.User
	if found, err := engine.ID(initial.ID).Get(&after); err != nil || !found || after.Password != initial.Password {
		t.Fatalf("existing administrator was changed: %v", err)
	}
}
