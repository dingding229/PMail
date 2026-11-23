package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- RFC 6238 authenticator interoperability requires HMAC-SHA1, not a bare SHA-1 digest.
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/utils/password"
	"xorm.io/xorm"
)

var (
	ErrFactorState = errors.New("two_factor_state_changed")
	ErrFactorCode  = errors.New("two_factor_invalid_code")
)

const recoveryCount = 10
const setupLifetime = 10 * time.Minute
const factorIssuer = "PMail Pro"
const totpPeriod = 30
const secretAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 32 symbols, five bits each

var factorBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

type FactorSetup struct {
	Secret string `json:"secret"`
	URI    string `json:"otpauth_uri"`
}

func codeAt(secret []byte, step int64) string {
	if step < 0 {
		return ""
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, secret) // #nosec G401 -- HMAC-SHA1 mandated by the selected RFC 6238 profile.
	mac.Write(counter[:])
	bytes := mac.Sum(nil)
	offset := bytes[len(bytes)-1] & 15
	value := binary.BigEndian.Uint32(bytes[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1_000_000)
}

func matchStep(secret []byte, code string, now time.Time, last int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	step := now.Unix() / totpPeriod
	for _, offset := range []int64{0, -1, 1} {
		candidate := step + offset
		if candidate >= 0 && candidate > last && subtle.ConstantTimeCompare([]byte(codeAt(secret, candidate)), []byte(code)) == 1 {
			return candidate, true
		}
	}
	return 0, false
}

func factorCipher() (cipher.AEAD, error) {
	if config.Instance == nil || config.Instance.SecretKey == "" {
		return nil, errors.New("TOTP encryption key is unavailable")
	}
	// Same domain-separated master-key construction as the Telegram secret.
	key := sha256.Sum256([]byte("pmail/totp/v1\x00" + config.Instance.SecretKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func sealFactor(userID int, secret []byte) (string, error) {
	c, err := factorCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, c.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	data := c.Seal(nonce, nonce, secret, []byte(strconv.Itoa(userID)))
	return base64.RawStdEncoding.EncodeToString(data), nil
}

func openFactor(row models.TOTPConfig) ([]byte, error) {
	c, err := factorCipher()
	if err != nil {
		return nil, err
	}
	data, err := base64.RawStdEncoding.DecodeString(row.Ciphertext)
	if err != nil || len(data) < c.NonceSize() {
		return nil, errors.New("invalid encrypted TOTP secret")
	}
	return c.Open(nil, data[:c.NonceSize()], data[c.NonceSize():], []byte(strconv.Itoa(row.UserID)))
}

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = secretAlphabet[b[i]&31]
	}
	return string(b), nil
}

func digest(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}
func recoveryDigest(code string) (string, bool) {
	code = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	return digest("recovery\x00" + code), len(code) == 16 && strings.IndexFunc(code, func(c rune) bool { return !strings.ContainsRune(secretAlphabet, c) }) < 0
}

// Serializes credential mutations against each other and account updates on
// SQLite, PostgreSQL and MySQL. MySQL may report zero affected rows for this
// deliberate no-op, so the authoritative check is the subsequent locked read.
func lockFactorUser(tx *xorm.Session, id int) (models.User, error) {
	var user models.User
	query := "UPDATE " + db.Instance.Quote("user") + " SET factor_version = factor_version WHERE id = ?"
	if _, err := tx.Exec(query, id); err != nil {
		return user, err
	} // #nosec G202 -- fixed statement, dialect-quoted constant table name and bound ID.
	ok, err := tx.ID(id).Get(&user)
	if err == nil && (!ok || user.Disabled != 0) {
		err = ErrCredentials
	}
	return user, err
}

func sameSnapshot(a, b models.User) bool {
	return a.ID == b.ID && a.AuthVersion == b.AuthVersion && a.FactorVersion == b.FactorVersion
}

func changeFactor(ctx context.Context, expected models.User, fn func(*xorm.Session, *models.User) error) (models.User, error) {
	var result models.User
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		u, err := lockFactorUser(tx, expected.ID)
		if err != nil {
			return err
		}
		if !sameSnapshot(expected, u) {
			return ErrCredentials
		}
		if err := fn(tx, &u); err != nil {
			return err
		}
		result = u
		return nil
	})
	return result, err
}

func SetupFactor(ctx context.Context, expected models.User) (FactorSetup, error) {
	primary, err := address.Primary(expected.ID)
	if err != nil {
		return FactorSetup{}, err
	}
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return FactorSetup{}, err
	}
	ciphertext, err := sealFactor(expected.ID, secret)
	if err != nil {
		return FactorSetup{}, err
	}
	_, err = changeFactor(ctx, expected, func(tx *xorm.Session, u *models.User) error {
		if u.TOTPEnabled {
			return ErrFactorState
		}
		if _, err := tx.ID(u.ID).Delete(&models.TOTPConfig{}); err != nil {
			return err
		}
		_, err := tx.Insert(&models.TOTPConfig{UserID: u.ID, Ciphertext: ciphertext, SetupExpires: time.Now().Add(setupLifetime).Unix(), LastStep: -1})
		return err
	})
	if err != nil {
		return FactorSetup{}, err
	}
	key := factorBase32.EncodeToString(secret)
	params := url.Values{"secret": {key}, "issuer": {factorIssuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	uri := url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + factorIssuer + ":" + primary.Address, RawQuery: params.Encode()}
	return FactorSetup{key, uri.String()}, nil
}

func setEnabled(tx *xorm.Session, u *models.User, enabled bool) error {
	_, err := tx.Table(&models.User{}).ID(u.ID).Incr("factor_version").Update(map[string]any{"totp_enabled": enabled})
	if err == nil {
		u.TOTPEnabled = enabled
		u.FactorVersion++
	}
	return err
}

func regenerateCodes(tx *xorm.Session, userID int) ([]string, error) {
	if _, err := tx.Where("user_id = ?", userID).Delete(&models.RecoveryCode{}); err != nil {
		return nil, err
	}
	codes := make([]string, 0, recoveryCount)
	for range recoveryCount {
		secret, err := randomSecret(16)
		if err != nil {
			return nil, err
		}
		code := strings.Join([]string{secret[:4], secret[4:8], secret[8:12], secret[12:]}, "-")
		hash, _ := recoveryDigest(code)
		if _, err := tx.Insert(&models.RecoveryCode{UserID: userID, Digest: hash}); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func EnableFactor(ctx context.Context, expected models.User, code, remote string) (models.User, []string, error) {
	var codes []string
	u, err := withFactorLimit(expected.ID, remote, func() (models.User, error) {
		return changeFactor(ctx, expected, func(tx *xorm.Session, u *models.User) error {
			if u.TOTPEnabled {
				return ErrFactorState
			}
			var row models.TOTPConfig
			ok, err := tx.ID(u.ID).Get(&row)
			if err != nil {
				return err
			}
			if !ok || time.Now().Unix() >= row.SetupExpires {
				return ErrFactorState
			}
			secret, err := openFactor(row)
			if err != nil {
				return err
			}
			step, ok := matchStep(secret, strings.TrimSpace(code), time.Now(), row.LastStep)
			if !ok {
				return ErrFactorCode
			}
			if _, err := tx.ID(u.ID).Cols("last_step", "setup_expires").Update(&models.TOTPConfig{LastStep: step}); err != nil {
				return err
			}
			if err := setEnabled(tx, u, true); err != nil {
				return err
			}
			codes, err = regenerateCodes(tx, u.ID)
			return err
		})
	})
	return u, codes, err
}

func consumeFactor(tx *xorm.Session, u models.User, code string, recovery bool, now time.Time) error {
	if !u.TOTPEnabled {
		return ErrFactorState
	}
	if recovery {
		hash, ok := recoveryDigest(code)
		if !ok {
			return ErrFactorCode
		}
		n, err := tx.Where("user_id = ? AND digest = ?", u.ID, hash).Delete(&models.RecoveryCode{})
		if err == nil && n != 1 {
			err = ErrFactorCode
		}
		return err
	}
	var row models.TOTPConfig
	ok, err := tx.ID(u.ID).Get(&row)
	if err != nil {
		return err
	}
	if !ok {
		return ErrFactorState
	}
	secret, err := openFactor(row)
	if err != nil {
		return err
	}
	step, ok := matchStep(secret, strings.TrimSpace(code), now, row.LastStep)
	if !ok {
		return ErrFactorCode
	}
	n, err := tx.ID(u.ID).Where("last_step < ?", step).Cols("last_step").Update(&models.TOTPConfig{LastStep: step})
	if err == nil && n != 1 {
		err = ErrFactorCode
	}
	return err
}

func clearFactor(tx *xorm.Session, u *models.User) error {
	if _, err := tx.ID(u.ID).Delete(&models.TOTPConfig{}); err != nil {
		return err
	}
	if _, err := tx.Where("user_id = ?", u.ID).Delete(&models.RecoveryCode{}); err != nil {
		return err
	}
	return setEnabled(tx, u, false)
}

func ManageFactor(ctx context.Context, expected models.User, secret, code, remote string, recovery, disable bool) (models.User, []string, error) {
	var codes []string
	u, err := withFactorLimit(expected.ID, remote, func() (models.User, error) {
		return changeFactor(ctx, expected, func(tx *xorm.Session, u *models.User) error {
			if valid, _ := password.Verify(u.Password, secret); !valid {
				return ErrCredentials
			}
			if err := consumeFactor(tx, *u, code, recovery, time.Now()); err != nil {
				return err
			}
			if disable {
				return clearFactor(tx, u)
			}
			var err error
			codes, err = regenerateCodes(tx, u.ID)
			return err
		})
	})
	return u, codes, err
}

func ResetFactor(ctx context.Context, targetID int) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		// Also allow an administrator to reset a disabled target.
		var u models.User
		q := "UPDATE " + db.Instance.Quote("user") + " SET factor_version = factor_version WHERE id = ?"
		if _, err := tx.Exec(q, targetID); err != nil {
			return err
		} // #nosec G202 -- constant table name and bound ID.
		ok, err := tx.ID(targetID).Get(&u)
		if err != nil {
			return err
		}
		if !ok {
			return ErrFactorState
		}
		return clearFactor(tx, &u)
	})
}
