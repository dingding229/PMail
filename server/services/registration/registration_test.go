package registration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/password"
)

func fixture(t *testing.T) context.Context {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{DbType: config.DBTypeSQLite, DbDSN: filepath.Join(root, "test.db"), Domain: "example.test", Domains: []string{"second.test"}}
	engine, err := db.Open(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	oldRoot, oldDB, oldConfig := config.ROOT_PATH, db.Instance, config.Instance
	config.ROOT_PATH, db.Instance, config.Instance = root, engine, cfg
	t.Cleanup(func() {
		pending.Store(false)
		config.ROOT_PATH, db.Instance, config.Instance = oldRoot, oldDB, oldConfig
		_ = engine.Close()
	})
	if err := db.SyncSchema(engine); err != nil {
		t.Fatal(err)
	}
	if err := domain.Initialize(cfg); err != nil {
		t.Fatal(err)
	}
	return context.Background()
}

func pendingKey(t *testing.T, ctx context.Context) string {
	t.Helper()
	if err := Prepare(ctx, "admin", ""); err != nil {
		t.Fatal(err)
	}
	if n, err := db.Instance.Count(&models.User{}); err != nil || n != 0 {
		t.Fatalf("users before registration: %d, %v", n, err)
	}
	if ready, err := Pending(ctx); err != nil || !ready {
		t.Fatalf("not pending: %v", err)
	}
	key, err := config.ReadRegistrationKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.ROOT_PATH, "config", "initial_admin_password")); !os.IsNotExist(err) {
		t.Fatalf("legacy password file unexpectedly created: %v", err)
	}
	return key
}

func TestRegistrationConsumesKeyAndCreatesPrimary(t *testing.T) {
	ctx := fixture(t)
	key := pendingKey(t, ctx)
	if _, err := Register(ctx, "BAD1-BAD1-BAD1-BAD1", "owner", "Owner", "sufficient-password"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("wrong-key result: %v", err)
	}
	if _, err := Register(ctx, key, "owner", "", "short"); !errors.Is(err, ErrPassword) {
		t.Fatalf("short-password result: %v", err)
	}
	u, err := Register(ctx, key, "Owner", "Mailbox Owner", "sufficient-password")
	if err != nil || u.IsAdmin != 1 || u.Account != "owner" || u.Name != "Mailbox Owner" {
		t.Fatalf("registration result: user=%d, err=%v", u.ID, err)
	}
	addresses, err := address.List(u.ID)
	if err != nil || len(addresses) != 1 || !addresses[0].IsPrimary || !addresses[0].CanSend || addresses[0].Address != "owner@example.test" {
		t.Fatalf("primary address: %#v, %v", addresses, err)
	}
	if _, err := config.ReadRegistrationKey(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("registration key was not removed: %v", err)
	}
	if _, err := Register(ctx, key, "other", "", "sufficient-password"); !errors.Is(err, ErrInitialized) {
		t.Fatalf("reused-key result: %v", err)
	}
}

func TestExistingDisabledUserKeepsRegistrationClosed(t *testing.T) {
	ctx := fixture(t)
	legacy := filepath.Join(config.ROOT_PATH, "config", "initial_admin_password")
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Instance.Insert(&models.User{Account: "disabled", Name: "Disabled", Password: "old", Disabled: 1}); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(ctx, "admin", "configured-password"); err != nil {
		t.Fatal(err)
	}
	if ready, err := Pending(ctx); ready || err != nil {
		t.Fatalf("existing user must block registration: %v", err)
	}
	if n, err := db.Instance.Count(&models.User{}); err != nil || n != 1 {
		t.Fatalf("existing database changed: count=%d, err=%v", n, err)
	}
	if raw, err := os.ReadFile(legacy); err != nil || string(raw) != "legacy" {
		t.Fatalf("existing legacy password file changed: %v", err)
	}
}

func TestAdminPasswordTakesPrecedenceOverRegistration(t *testing.T) {
	ctx := fixture(t)
	t.Setenv("PMAIL_ADMIN_ACCOUNT", "environment")
	t.Setenv("PMAIL_ADMIN_PASSWORD", "explicit-password")
	if _, err := config.EnsureRegistrationKey(); err != nil {
		t.Fatal(err)
	}
	account, secret, _ := config.BootstrapCredentials()
	if err := Prepare(ctx, account, secret); err != nil {
		t.Fatal(err)
	}
	var u models.User
	if found, err := db.Instance.Where("account=?", "environment").Get(&u); err != nil || !found || u.IsAdmin != 1 {
		t.Fatalf("explicit administrator not created: %v", err)
	}
	if valid, _ := password.Verify(u.Password, "explicit-password"); !valid {
		t.Fatal("explicit password was not used")
	}
	if ready, err := Pending(ctx); ready || err != nil {
		t.Fatalf("explicit password did not close registration: %v", err)
	}
	if _, err := config.ReadRegistrationKey(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale registration key remains: %v", err)
	}
}

func TestConcurrentRegistrationOnlyOneWins(t *testing.T) {
	ctx := fixture(t)
	key := pendingKey(t, ctx)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, candidate := range []string{"first", "second"} {
		go func() {
			<-start
			_, err := Register(ctx, key, candidate, "", "long-enough-password")
			results <- err
		}()
	}
	close(start)
	var wins int
	for range 2 {
		if err := <-results; err == nil {
			wins++
		} else if !errors.Is(err, ErrInitialized) {
			t.Fatalf("registration loser: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("successes=%d; want one", wins)
	}
	if n, err := db.Instance.Count(&models.User{}); err != nil || n != 1 {
		t.Fatalf("registered %d users: %v", n, err)
	}
	var guard models.RegistrationGuard
	if found, err := db.Instance.ID(1).Get(&guard); err != nil || !found || guard.Generation != 1 {
		t.Fatalf("registration guard: %+v; found=%v, err=%v", guard, found, err)
	}
}
