package auth

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/utils/password"
)

var (
	ErrCredentials = errors.New("invalid credentials")
	ErrLocked      = errors.New("too many authentication attempts; try again later")
)

const (
	failureThreshold   = 5
	failureWindow      = 15 * time.Minute
	ipFailureThreshold = 30
	maxFailureEntries  = 100000
)

type failureState struct {
	failures, pending int
	expires           time.Time
}

var attempts = struct {
	sync.Mutex
	entries map[string]failureState
	ips     map[string]failureState
}{entries: make(map[string]failureState), ips: make(map[string]failureState)}

func ipOf(remote string) string {
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	if ip := net.ParseIP(remote); ip != nil {
		return ip.String()
	}
	return "unknown"
}

// Authenticate is the only password login path. Resolve aliases before limiting
// so changing the spelling or alias of an account doesn't reset its attempt count.
func Authenticate(login, secret, remote string) (models.User, error) {
	var user models.User
	if len(login) > 320 {
		return user, ErrCredentials
	}
	login = strings.TrimSpace(login)
	user, found, err := address.OwnerForLogin(login)
	if err != nil {
		return models.User{}, err
	}
	identity := "name:" + strings.ToLower(login)
	if found {
		identity = "id:" + strconv.Itoa(user.ID)
	}
	key := ipOf(remote) + "|" + identity
	if !reserve(key) {
		return models.User{}, ErrLocked
	}
	outcome := 0 // Internal errors don't count as failed credential attempts.
	defer func() { complete(key, outcome) }()
	for i := 0; i < 2; i++ {
		if !found || user.Disabled != 0 {
			outcome = -1
			return models.User{}, ErrCredentials
		}
		valid, upgrade := password.Verify(user.Password, secret)
		if !valid {
			outcome = -1
			return models.User{}, ErrCredentials
		}
		now := time.Now().UTC()
		updates := map[string]any{"last_login_at": now}
		write := db.Instance.Table(&models.User{}).ID(user.ID).Where("auth_version = ? AND disabled = 0", user.AuthVersion)
		if upgrade {
			hash, err := password.Hash(secret)
			if err != nil {
				return models.User{}, err
			}
			updates["password"] = hash
		}
		n, err := write.Update(updates)
		if err != nil {
			return models.User{}, err
		}
		if n == 0 {
			// MySQL may report no change for two last_login_at updates in the same
			// second. Check the current version before rejecting or retrying.
			var fresh models.User
			found, err = db.Instance.ID(user.ID).Get(&fresh)
			if err != nil {
				return models.User{}, err
			}
			if !upgrade && found && fresh.Disabled == 0 && fresh.AuthVersion == user.AuthVersion {
				outcome = 1
				return fresh, nil
			}
			user = fresh
			continue
		}
		if upgrade {
			user.Password = updates["password"].(string)
		}
		user.LastLoginAt = &now
		outcome = 1
		return user, nil
	}
	return models.User{}, ErrCredentials
}

func reserve(key string) bool {
	attempts.Lock()
	defer attempts.Unlock()
	now := time.Now()
	ip, _, _ := strings.Cut(key, "|")
	cleanupFailures(now)
	ipState := attempts.ips[ip]
	if ipState.failures >= ipFailureThreshold && now.Before(ipState.expires) {
		return false
	}
	state := attempts.entries[key]
	if state.pending == 0 && !now.Before(state.expires) {
		state = failureState{expires: now.Add(failureWindow)}
	}
	if state.failures+state.pending >= failureThreshold {
		return false
	}
	if _, exists := attempts.entries[key]; !exists && len(attempts.entries) >= maxFailureEntries {
		evictOldest()
	}
	state.pending++
	attempts.entries[key] = state
	ipState.pending++
	attempts.ips[ip] = ipState
	return true
}

func cleanupFailures(now time.Time) {
	for key, state := range attempts.entries {
		if state.pending == 0 && !now.Before(state.expires) {
			delete(attempts.entries, key)
		}
	}
	for ip, state := range attempts.ips {
		if state.pending == 0 && !now.Before(state.expires) {
			delete(attempts.ips, ip)
		}
	}
}
func evictOldest() {
	var candidate string
	var when time.Time
	for key, state := range attempts.entries {
		if state.pending > 0 {
			continue
		}
		if candidate == "" || state.expires.Before(when) {
			candidate, when = key, state.expires
		}
	}
	if candidate != "" {
		delete(attempts.entries, candidate)
	}
}

func complete(key string, outcome int) {
	attempts.Lock()
	defer attempts.Unlock()
	state := attempts.entries[key]
	state.pending--
	ip, _, _ := strings.Cut(key, "|")
	ipState := attempts.ips[ip]
	ipState.pending--
	if outcome > 0 {
		state.failures = 0
	} else if outcome < 0 {
		state.failures++
		ipState.failures++
		ipState.expires = time.Now().Add(failureWindow)
		if state.failures >= failureThreshold {
			state.expires = time.Now().Add(failureWindow)
		}
	}
	if state.pending == 0 && state.failures == 0 {
		delete(attempts.entries, key)
	} else {
		attempts.entries[key] = state
	}
	if ipState.pending == 0 && ipState.failures == 0 {
		delete(attempts.ips, ip)
	} else {
		attempts.ips[ip] = ipState
	}
}

func RunCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			attempts.Lock()
			cleanupFailures(now)
			for key, state := range attempts.entries {
				if state.pending == 0 && !now.Before(state.expires) {
					delete(attempts.entries, key)
				}
			}
			attempts.Unlock()
		}
	}
}

// ValidateSession re-checks long-lived mail protocol connections after password changes or disabling.
func ValidateSession(userID int, version int64) error {
	if userID <= 0 {
		return ErrCredentials
	}
	ok, err := db.Instance.ID(userID).Where("disabled = 0 AND auth_version = ?", version).Exist(&models.User{})
	if err != nil {
		return err
	}
	if !ok {
		return ErrCredentials
	}
	return nil
}
