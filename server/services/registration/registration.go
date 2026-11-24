package registration

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/maintenance"
	"github.com/Jinnrry/pmail/services/user"
	"github.com/Jinnrry/pmail/utils/password"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

var (
	ErrInitialized = errors.New("system already initialized")
	ErrInvalidKey  = errors.New("invalid registration key")
	ErrAccount     = errors.New("invalid registration account")
	ErrDisplayName = errors.New("invalid registration display name")
	ErrPassword    = errors.New("invalid registration password strength")
	pending        atomic.Bool
	noticeMu       sync.Mutex
)

// Prepare runs after the domain and database are available, before serving HTTP.
// Existing accounts (including disabled accounts) always take precedence.
func Prepare(ctx context.Context, account, adminPassword string) error {
	pending.Store(false)
	hasUser, err := db.Instance.Context(ctx).Exist(&models.User{})
	if err != nil {
		return err
	}
	if hasUser {
		consumeKey()
		return nil
	}
	if err := ensureGuard(ctx); err != nil {
		return err
	}
	if adminPassword != "" {
		if _, err := createAdmin(ctx, account, "admin", adminPassword, true); err != nil && !errors.Is(err, ErrInitialized) {
			if exists, checkErr := db.Instance.Context(ctx).Exist(&models.User{}); checkErr != nil || !exists {
				return err
			}
		}
		consumeKey()
		return nil
	}
	_, keyErr := config.EnsureRegistrationKey()
	hasUser, err = db.Instance.Context(ctx).Exist(&models.User{})
	if err != nil {
		return err
	}
	if hasUser {
		consumeKey()
		return nil
	}
	if keyErr != nil {
		return keyErr
	}
	pending.Store(true)
	return nil
}

// Once initialized, supported account management cannot remove the last
// account. While pending, recheck the database so another process can finish.
func Pending(ctx context.Context) (bool, error) {
	if !pending.Load() {
		return false, nil
	}
	hasUser, err := db.Instance.Context(ctx).Exist(&models.User{})
	if err != nil {
		return false, err
	}
	if hasUser {
		pending.Store(false)
	}
	return !hasUser && pending.Load(), nil
}

func Register(ctx context.Context, suppliedKey, account, displayName, secret string) (models.User, error) {
	ready, err := Pending(ctx)
	if err != nil {
		return models.User{}, err
	}
	if !ready {
		return models.User{}, ErrInitialized
	}
	key, err := config.ReadRegistrationKey()
	if err != nil {
		// A concurrent winner may have committed and consumed the key.
		if ready, checkErr := Pending(ctx); checkErr == nil && !ready {
			return models.User{}, ErrInitialized
		}
		return models.User{}, err
	}
	want, got := sha256.Sum256([]byte(key)), sha256.Sum256([]byte(suppliedKey))
	if subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
		return models.User{}, ErrInvalidKey
	}
	if !utf8.ValidString(secret) || utf8.RuneCountInString(secret) < 8 || strings.TrimSpace(secret) == "" {
		return models.User{}, ErrPassword
	}
	created, err := createAdmin(ctx, account, displayName, secret, false)
	if err != nil {
		if ready, checkErr := Pending(ctx); checkErr == nil && !ready {
			return models.User{}, ErrInitialized
		}
		return models.User{}, err
	}
	noticeMu.Lock()
	pending.Store(false)
	consumeKey()
	noticeMu.Unlock()
	log.Infof("audit_event=first_admin_registered user_id=%d account=%q", created.ID, created.Account)
	return created, nil
}

// Prepare the single guard row even if two processes reach startup together.
func ensureGuard(ctx context.Context) error {
	exists, err := db.Instance.Context(ctx).ID(1).Exist(&models.RegistrationGuard{})
	if err != nil || exists {
		return err
	}
	if _, err := db.Instance.Context(ctx).Insert(&models.RegistrationGuard{ID: 1}); err != nil {
		if !db.IsUniqueViolation(err) {
			return err
		}
		if exists, checkErr := db.Instance.Context(ctx).ID(1).Exist(&models.RegistrationGuard{}); checkErr != nil || !exists {
			return err
		}
	}
	return nil
}

func createAdmin(ctx context.Context, account, displayName, secret string, legacy bool) (models.User, error) {
	local, err := address.NormalizeLocal(account)
	if err != nil || utf8.RuneCountInString(local) > 20 {
		return models.User{}, ErrAccount
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = local
	}
	if !user.ValidName(displayName) {
		return models.User{}, ErrDisplayName
	}
	hash, err := password.Hash(secret)
	if err != nil {
		return models.User{}, err
	}
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	var created models.User
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		// Lock (and actually update) the same row before reading users. The
		// competing transaction will either see the committed user or abort
		// on a snapshot conflict; the claimant name is immaterial.
		result, err := tx.Exec("UPDATE registration_guard SET generation = generation + 1 WHERE id = 1")
		if err != nil {
			return err
		}
		if updated, err := result.RowsAffected(); err != nil || updated != 1 {
			return errors.New("registration guard row unavailable")
		}
		hasUser, err := tx.Exist(&models.User{})
		if err != nil {
			return err
		}
		if hasUser {
			return ErrInitialized
		}
		created = models.User{Account: local, Name: displayName, Password: hash, IsAdmin: 1}
		if !legacy {
			now := time.Now()
			created.LastLoginAt = &now
		}
		if _, err := tx.Insert(&created); err != nil {
			return err
		}
		if legacy {
			err = address.Seed(tx, created)
		} else {
			err = address.SeedPrimary(tx, created)
		}
		return err
	})
	return created, err
}

func consumeKey() {
	if err := config.RemoveRegistrationKey(); err != nil {
		// The committed user already invalidates the key, even if the config
		// volume failed to delete it. Startup retries the cleanup.
		log.Warnf("remove consumed registration key: %v", err)
	}
}

func logNotice(ctx context.Context, webDomain string) bool {
	noticeMu.Lock()
	defer noticeMu.Unlock()
	ready, err := Pending(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Warnf("check first registration state: %v", err)
		}
		return true
	}
	if !ready {
		return false
	}
	key, err := config.ReadRegistrationKey()
	if err != nil {
		log.Warnf("read first registration key for reminder: %v", err)
		return true
	}
	target := (&url.URL{Scheme: "https", Host: webDomain, Path: "/"}).String()
	// Stdout also keeps this visible at restrictive PMail log levels.
	writeNotice(os.Stdout, key, target, os.Getenv("PMAIL_PRINT_REGISTRATION_KEY") == "1")
	return true
}

// StartNotices prints synchronously at each startup, then repeats until the
// first user exists or the server is stopped.
func StartNotices(ctx context.Context, webDomain string) {
	if !logNotice(ctx, webDomain) {
		return
	}
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				keep := true
				maintenance.Work(ctx, func(ctx context.Context) { keep = logNotice(ctx, webDomain) })
				if !keep {
					return
				}
			}
		}
	}()
}

func writeNotice(w io.Writer, key, target string, printKey bool) {
	fmt.Fprintf(w, "PMail 首次注册 / First registration: %s; 注册密钥文件 / Registration key file: %q\n", target, config.RegistrationKeyPath())
	if printKey {
		fmt.Fprintf(w, "注册密钥 / Registration key: %s\n", key)
	}
}

// VerifyKey checks a supplied first-run registration key in constant time
// without consuming it (used before the mail domain is configured).
func VerifyKey(supplied string) error {
	key, err := config.ReadRegistrationKey()
	if err != nil {
		return err
	}
	want, got := sha256.Sum256([]byte(key)), sha256.Sum256([]byte(strings.TrimSpace(supplied)))
	if subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
		return ErrInvalidKey
	}
	return nil
}
