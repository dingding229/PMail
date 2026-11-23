package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/password"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestTOTPWindowAndReplay(t *testing.T) {
	secret := []byte("12345678901234567890") // RFC 6238 Appendix B, truncated to six digits.
	if got := codeAt(secret, 1); got != "287082" {
		t.Fatalf("RFC vector: %s", got)
	}
	now := time.Unix(1200, 0)
	for _, tc := range []struct {
		step, last int64
		valid      bool
	}{
		{38, -1, false}, {39, -1, true}, {40, -1, true}, {41, -1, true}, {42, -1, false},
		{40, 40, false}, {39, 40, false}, {41, 40, true},
	} {
		got, valid := matchStep(secret, codeAt(secret, tc.step), now, tc.last)
		if valid != tc.valid || (valid && got != tc.step) {
			t.Fatalf("%+v: step %d, valid %t", tc, got, valid)
		}
	}
}

func secondFactorFixture(t *testing.T) (models.User, []byte, []string) {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	engine.SetMaxOpenConns(1)
	oldDB, oldConfig := db.Instance, config.Instance
	db.Instance, config.Instance = engine, &config.Config{Domain: "second-factor.test", SecretKey: "random fixture key, test only"}
	t.Cleanup(func() { db.Instance, config.Instance = oldDB, oldConfig; _ = engine.Close() })
	if err = db.SyncSchema(engine); err != nil {
		t.Fatal(err)
	}
	if err = domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	hash, err := password.Hash("test-login-password")
	if err != nil {
		t.Fatal(err)
	}
	u := models.User{Account: "factoruser", Password: hash}
	if _, err := engine.Insert(&u); err != nil {
		t.Fatal(err)
	}
	primary := models.MailboxAddress{UserID: u.ID, DomainID: domain.Default().ID, LocalPart: "factoruser", IsPrimary: true}
	if _, err := engine.Insert(&primary); err != nil {
		t.Fatal(err)
	}
	setup, err := SetupFactor(context.Background(), u)
	if err != nil || !strings.Contains(setup.URI, "PMail%20Pro:factoruser@second-factor.test") {
		t.Fatalf("setup URI: %v", err)
	}
	secret, err := factorBase32.DecodeString(setup.Secret)
	if err != nil {
		t.Fatal(err)
	}
	var row models.TOTPConfig
	if ok, err := engine.ID(u.ID).Get(&row); err != nil || !ok || row.Ciphertext == "" || strings.Contains(row.Ciphertext, setup.Secret) {
		t.Fatalf("secret not encrypted: found %v/%v", ok, err)
	}
	row.UserID++
	if _, err := openFactor(row); err == nil {
		t.Fatal("ciphertext accepted for another user")
	}
	u, codes, err := EnableFactor(context.Background(), u, codeAt(secret, time.Now().Unix()/30), "192.0.2.211")
	if err != nil || !u.TOTPEnabled || u.FactorVersion != 1 || len(codes) != 10 {
		t.Fatalf("enable: codes=%d, err=%v", len(codes), err)
	}
	return u, secret, codes
}

func TestChallengeExpiryReplayAndLimit(t *testing.T) {
	u, secret, codes := secondFactorFixture(t)
	ctx := context.Background()
	const source = "192.0.2.212"
	now := time.Now()

	expired, err := issueChallenge(u, source, now.Add(-5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finishWebLogin(ctx, expired, codes[0], source, true, now); !errors.Is(err, ErrChallenge) {
		t.Fatalf("expired: %v", err)
	}
	firstUser, challenge, err := BeginWebLogin(u.Account, "test-login-password", source)
	if err != nil || challenge == "" || firstUser.LastLoginAt != nil {
		t.Fatalf("password-only step: %v", err)
	}
	if _, err := finishWebLogin(ctx, challenge, codes[0], "192.0.2.213", true, now); !errors.Is(err, ErrChallenge) {
		t.Fatalf("foreign IP: %v", err)
	}
	next := now.Add(30 * time.Second)
	code := codeAt(secret, next.Unix()/30)
	if _, err := finishWebLogin(ctx, challenge, code, source, false, next); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := finishWebLogin(ctx, challenge, codes[0], source, true, next); !errors.Is(err, ErrChallenge) {
		t.Fatalf("challenge replay: %v", err)
	}
	_, challenge, err = BeginWebLogin(u.Account, "test-login-password", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finishWebLogin(ctx, challenge, code, source, false, next); !errors.Is(err, ErrFactorCode) {
		t.Fatalf("TOTP replay: %v", err)
	}
	if _, err := finishWebLogin(ctx, challenge, codes[0], source, true, next); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	_, challenge, err = BeginWebLogin(u.Account, "test-login-password", source)
	if err != nil {
		t.Fatal(err)
	}
	for range failureThreshold {
		if _, err := finishWebLogin(ctx, challenge, codes[0], source, true, next); !errors.Is(err, ErrFactorCode) {
			t.Fatalf("recovery must be spent: %v", err)
		}
	}
	if _, err := finishWebLogin(ctx, challenge, codes[1], source, true, next); !errors.Is(err, ErrLocked) {
		t.Fatalf("second-factor limit: %v", err)
	}
	if _, _, err := BeginWebLogin(u.Account, "test-login-password", source); !errors.Is(err, ErrLocked) {
		t.Fatalf("password login shares limit: %v", err)
	}
}

func TestRecoveryConsumptionIsAtomicAcrossChallenges(t *testing.T) {
	u, _, codes := secondFactorFixture(t)
	now := time.Now()
	const source = "192.0.2.214"
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		key, err := issueChallenge(u, source, now)
		if err != nil {
			t.Fatal(err)
		}
		wg.Go(func() {
			_, err := finishWebLogin(context.Background(), key, codes[0], source, true, now)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrFactorCode) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("recovery was accepted %d times", successes)
	}
}

func TestManageFactorAndAppPasswordLifecycle(t *testing.T) {
	u, _, codes := secondFactorFixture(t)
	ctx := context.Background()
	const source = "192.0.2.215"
	const old = "test-login-password"
	created, err := CreateAppPassword(ctx, u, "desktop")
	if err != nil || len(created.Password) != 16 || created.Hash == created.Password {
		t.Fatalf("app creation: %v", err)
	}
	if _, appID, err := AuthenticateMail("factoruser@second-factor.test", created.Password, source); err != nil || appID != created.ID {
		t.Fatalf("mail authentication: %v", err)
	}
	rows, err := AppPasswords(ctx, u.ID)
	if err != nil || len(rows) != 1 || rows[0].LastUsedAt == nil {
		t.Fatalf("last use not tracked: %v", err)
	}
	if _, _, err := BeginWebLogin(u.Account, created.Password, source); !errors.Is(err, ErrCredentials) {
		t.Fatalf("app password works on web: %v", err)
	}

	if _, _, err := ManageFactor(ctx, u, "wrong", codes[0], source, true, false); !errors.Is(err, ErrCredentials) {
		t.Fatalf("wrong current password: %v", err)
	}
	_, newCodes, err := ManageFactor(ctx, u, old, codes[0], source, true, false)
	if err != nil || len(newCodes) != 10 {
		t.Fatalf("regenerate: %v", err)
	}
	if _, _, err := ManageFactor(ctx, u, old, codes[1], source, true, true); !errors.Is(err, ErrFactorCode) {
		t.Fatalf("old recovery code survived regeneration: %v", err)
	}
	u, _, err = ManageFactor(ctx, u, old, newCodes[0], source, true, true)
	if err != nil || u.TOTPEnabled || u.FactorVersion != 2 {
		t.Fatalf("disable: %v", err)
	}
	if n, err := db.Instance.Count(&models.TOTPConfig{}); err != nil || n != 0 {
		t.Fatalf("old TOTP secret survived: %d/%v", n, err)
	}
	if n, err := db.Instance.Count(&models.RecoveryCode{}); err != nil || n != 0 {
		t.Fatalf("old recovery codes survived: %d/%v", n, err)
	}

	newHash, err := password.Hash("updated-login-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Instance.Table(&models.User{}).ID(u.ID).Incr("auth_version").Update(map[string]any{"password": newHash}); err != nil {
		t.Fatal(err)
	}
	u.AuthVersion++
	if _, _, err := AuthenticateMail(u.Account, created.Password, source); err != nil {
		t.Fatalf("password change revoked app password: %v", err)
	}
	if err := DeleteAppPassword(ctx, u, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMailSession(u.ID, u.AuthVersion, u.FactorVersion, created.ID, nil); !errors.Is(err, ErrCredentials) {
		t.Fatalf("deleted app session: %v", err)
	}
	created, err = CreateAppPassword(ctx, u, "disabled account")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Instance.ID(u.ID).Cols("disabled").Update(&models.User{Disabled: 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := AuthenticateMail(u.Account, created.Password, source); !errors.Is(err, ErrCredentials) {
		t.Fatalf("disabled user app password: %v", err)
	}
	if err := ValidateMailSession(u.ID, u.AuthVersion, u.FactorVersion, created.ID, nil); !errors.Is(err, ErrCredentials) {
		t.Fatalf("disabled user live session: %v", err)
	}
}
