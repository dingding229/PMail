package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

var ErrChallenge = errors.New("two_factor_challenge_expired")

const challengeLifetime = 5 * time.Minute
const maxChallenges = 8192

type challenge struct {
	userID                     int
	authVersion, factorVersion int64
	expires                    time.Time
	ip                         string
	busy                       bool
}

var challenges = struct {
	sync.Mutex
	rows map[string]challenge
}{rows: make(map[string]challenge)}

func BeginWebLogin(login, secret, remote string) (models.User, string, error) {
	u, _, err := authenticate(login, secret, remote, false, true)
	if err != nil || !u.TOTPEnabled {
		return u, "", err
	}
	key, err := issueChallenge(u, ipOf(remote), time.Now())
	return u, key, err
}

func issueChallenge(user models.User, ip string, now time.Time) (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(token[:])
	challenges.Lock()
	defer challenges.Unlock()
	for key, c := range challenges.rows {
		if !c.busy && !now.Before(c.expires) {
			delete(challenges.rows, key)
		}
	}
	if len(challenges.rows) >= maxChallenges {
		return "", ErrLocked
	}
	challenges.rows[digest(raw)] = challenge{user.ID, user.AuthVersion, user.FactorVersion, now.Add(challengeLifetime), ip, false}
	return raw, nil
}

func withFactorLimit(userID int, remote string, fn func() (models.User, error)) (models.User, error) {
	ip := ipOf(remote)
	release, ok := lookupLimits.acquire(ip, time.Now())
	if !ok {
		return models.User{}, ErrLocked
	}
	defer release()
	key := ip + "|id:" + strconv.Itoa(userID)
	if !reserve(key, failureWindow) {
		return models.User{}, ErrLocked
	}
	u, err := fn()
	outcome := 0
	if err == nil {
		outcome = 1
	} else if errors.Is(err, ErrCredentials) || errors.Is(err, ErrFactorCode) || errors.Is(err, ErrChallenge) {
		outcome = -1
	}
	complete(key, outcome, failureWindow)
	return u, err
}

func FinishWebLogin(ctx context.Context, key, code, remote string, recovery bool) (models.User, error) {
	return finishWebLogin(ctx, key, code, remote, recovery, time.Now())
}

func finishWebLogin(ctx context.Context, key, code, remote string, recovery bool, now time.Time) (models.User, error) {
	candidate := ""
	if len(key) == base64.RawURLEncoding.EncodedLen(32) {
		candidate = digest(key)
	}
	ip := ipOf(remote)
	challenges.Lock()
	c, ok := challenges.rows[candidate]
	valid := ok && !c.busy && c.ip == ip && now.Before(c.expires)
	if valid {
		c.busy = true
		challenges.rows[candidate] = c
	}
	challenges.Unlock()
	if !valid {
		// Invalid or foreign tokens also consume the existing per-IP budget.
		return withFactorLimit(0, remote, func() (models.User, error) { return models.User{}, ErrChallenge })
	}
	u, err := withFactorLimit(c.userID, remote, func() (models.User, error) {
		expected := models.User{ID: c.userID, AuthVersion: c.authVersion, FactorVersion: c.factorVersion}
		return changeFactor(ctx, expected, func(tx *xorm.Session, u *models.User) error {
			if err := consumeFactor(tx, *u, code, recovery, now); err != nil {
				return err
			}
			_, err := tx.Table(&models.User{}).ID(u.ID).Update(map[string]any{"last_login_at": now.UTC()})
			u.LastLoginAt = &now
			return err
		})
	})
	obsolete := errors.Is(err, ErrCredentials) || errors.Is(err, ErrFactorState)
	challenges.Lock()
	if err == nil || obsolete {
		delete(challenges.rows, candidate)
	} else {
		c.busy = false
		challenges.rows[candidate] = c
	}
	challenges.Unlock()
	if obsolete {
		return models.User{}, ErrChallenge
	}
	return u, err
}
