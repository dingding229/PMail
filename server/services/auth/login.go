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

func AuthenticateMail(login, secret, remote string) (models.User, int, error) {
	return authenticate(login, secret, remote, true, false)
}

func authenticate(login, secret, remote string, mail, canChallenge bool) (models.User, int, error) {
	var user models.User
	if len(login) > 320 {
		return user, 0, ErrCredentials
	}
	ip := ipOf(remote)
	release, ok := lookupLimits.acquire(ip, time.Now())
	if !ok {
		return user, 0, ErrLocked
	}
	defer release()
	login = strings.TrimSpace(login)
	user, found, err := address.OwnerForLogin(login)
	if err != nil {
		return models.User{}, 0, err
	}
	identity := "name:" + strings.ToLower(login)
	if found {
		identity = "id:" + strconv.Itoa(user.ID)
	}
	key := ip + "|" + identity
	if !reserve(key, failureWindow) {
		return models.User{}, 0, ErrLocked
	}
	outcome := 0
	defer func() { complete(key, outcome, failureWindow) }()
	for i := 0; i < 2; i++ {
		if !found || user.Disabled != 0 {
			outcome = -1
			return models.User{}, 0, ErrCredentials
		}
		var valid, upgrade bool
		if !user.TOTPEnabled || canChallenge {
			valid, upgrade = password.Verify(user.Password, secret)
		}
		appID := 0
		if !valid && mail {
			appID, err = authenticateApp(user.ID, secret)
			if err != nil {
				return models.User{}, 0, err
			}
			valid = appID != 0
		}
		if !valid {
			outcome = -1
			return models.User{}, 0, ErrCredentials
		}
		// A verified password is only half a login and cannot reset failures
		// accumulated by the second step or update the last login time.
		if canChallenge && user.TOTPEnabled {
			return user, 0, nil
		}
		now := time.Now().UTC()
		updates := map[string]any{"last_login_at": now}
		if upgrade {
			hash, err := password.Hash(secret)
			if err != nil {
				return models.User{}, 0, err
			}
			updates["password"] = hash
		}
		write := db.Instance.Table(&models.User{}).ID(user.ID).Where("auth_version = ? AND factor_version = ? AND disabled = 0", user.AuthVersion, user.FactorVersion)
		n, err := write.Update(updates)
		if err != nil {
			return models.User{}, 0, err
		}
		if n == 0 {
			var fresh models.User
			found, err = db.Instance.ID(user.ID).Get(&fresh)
			if err != nil {
				return models.User{}, 0, err
			}
			// Some MySQL configurations report zero affected rows for the same timestamp.
			if upgrade || !found || fresh.Disabled != 0 || !sameSnapshot(user, fresh) {
				user = fresh
				continue
			}
			user = fresh
		}
		if upgrade {
			user.Password = updates["password"].(string)
		}
		if appID != 0 {
			if err := touchApp(appID, user.ID, now); err != nil {
				return models.User{}, 0, err
			}
			if err := ValidateMailSession(user.ID, user.AuthVersion, user.FactorVersion, appID, nil); err != nil {
				return models.User{}, 0, err
			}
		}
		user.LastLoginAt = &now
		outcome = 1
		return user, appID, nil
	}
	return models.User{}, 0, ErrCredentials
}

// ReserveRegistration uses a dedicated per-IP bucket in the login limiter.
// The completion callback must be called exactly once with a matching outcome.
func ReserveRegistration(remote string) (func(failed bool), bool) {
	const window = 10 * time.Minute
	release, ok := lookupLimits.acquire(ipOf(remote), time.Now())
	if !ok {
		return nil, false
	}
	key := ipOf(remote) + "|first-registration"
	if !reserve(key, window) {
		release()
		return nil, false
	}
	return func(failed bool) {
		outcome := 0
		if failed {
			outcome = -1
		}
		complete(key, outcome, window)
		release()
	}, true
}

func reserve(key string, window time.Duration) bool {
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
		state = failureState{expires: now.Add(window)}
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

func complete(key string, outcome int, window time.Duration) {
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
		ipState.expires = time.Now().Add(window)
		if state.failures >= failureThreshold {
			state.expires = time.Now().Add(window)
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
