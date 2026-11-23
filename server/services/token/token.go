package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	log "github.com/sirupsen/logrus"
)

var ErrInvalid = errors.New("invalid API token")
var ErrExpired = errors.New("API token expired")

func hash(raw string) string { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }

const (
	Read   = "read"
	Send   = "send"
	Admin  = "admin"
	Legacy = "legacy"
)

var ErrScope = errors.New("invalid token scope")
var ErrAdminScope = errors.New("administrator scope requires an administrator")
var ErrExpiry = errors.New("invalid token expiry")

func Scope(row models.APIToken) string {
	if row.Scope == "" {
		return Legacy
	}
	return row.Scope
}

func expiry(now time.Time, expires *time.Time, never bool) (*time.Time, error) {
	if never {
		if expires != nil {
			return nil, ErrExpiry
		}
		return nil, nil
	}
	if expires == nil {
		next := now.Add(90 * 24 * time.Hour)
		return &next, nil
	}
	if !expires.After(now) {
		return nil, ErrExpiry
	}
	return expires, nil
}

func Allows(scope, method, path string) bool {
	if scope == Legacy {
		return true
	}
	if path == "/api/login" || path == "/api/login/2fa" || path == "/api/logout" || path == "/api/token/create" {
		return false
	}
	if scope == Admin {
		return true
	}
	if scope != Read && scope != Send {
		return false
	}
	if method == http.MethodGet || method == http.MethodHead {
		if scope == Send && strings.HasPrefix(path, "/api/compose/inline-image/") {
			return true
		}
		if strings.HasPrefix(path, "/api/attachments/") || strings.HasPrefix(path, "/attachments/") {
			return true
		}
	}
	switch path {
	case "/api/bootstrap", "/api/email/list", "/api/email/detail", "/api/group", "/api/group/list", "/api/user/info", "/api/address/list":
		return method == http.MethodGet || method == http.MethodHead || method == http.MethodPost
	case "/api/compose/inline-image", "/api/email/send", "/api/email/forward", "/api/email/draft/save", "/api/email/draft/delete":
		return scope == Send && method == http.MethodPost
	}
	return false
}

func Create(userID int, name, scope string, expires *time.Time, never bool) (models.APIToken, string, error) {
	if scope == "" {
		scope = Read
	}
	if scope != Read && scope != Send && scope != Admin {
		return models.APIToken{}, "", ErrScope
	}
	now := time.Now().UTC()
	expires, err := expiry(now, expires, never)
	if err != nil {
		return models.APIToken{}, "", err
	}
	if strings.TrimSpace(name) == "" {
		name = "API token"
	}
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return models.APIToken{}, "", err
	}
	raw := "pm_" + base64.RawURLEncoding.EncodeToString(rawBytes)
	u, err := user(userID)
	if err != nil {
		return models.APIToken{}, "", err
	}
	if scope == Admin && u.IsAdmin != 1 {
		return models.APIToken{}, "", ErrAdminScope
	}
	row := models.APIToken{Scope: scope, UserID: userID, Name: name, TokenHash: hash(raw), Prefix: raw[:8], AuthVersion: u.AuthVersion, ExpiresAt: expires, Created: now}
	if _, err := db.Instance.Insert(&row); err != nil {
		return row, "", err
	}
	row.TokenHash = ""
	return row, raw, nil
}
func user(id int) (models.User, error) {
	var u models.User
	ok, err := db.Instance.ID(id).Get(&u)
	if err != nil {
		return u, err
	}
	if !ok {
		return u, ErrInvalid
	}
	return u, nil
}
func List(userID int) ([]models.APIToken, error) {
	var rows []models.APIToken
	err := db.Instance.Where("user_id=?", userID).Desc("id").Find(&rows)
	for i := range rows {
		rows[i].TokenHash = ""
	}
	return rows, err
}
func Delete(id, userID int, admin bool) error {
	q := db.Instance.ID(id)
	if !admin {
		q.Where("user_id=?", userID)
	}
	_, err := q.Delete(&models.APIToken{})
	return err
}
func Authenticate(raw string) (models.User, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return models.User{}, "", ErrInvalid
	}
	var t models.APIToken
	ok, err := db.Instance.Where("token_hash=?", hash(raw)).Get(&t)
	if err != nil || !ok {
		return models.User{}, "", ErrInvalid
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return models.User{}, "", ErrExpired
	}
	u, err := user(t.UserID)
	if err != nil || u.Disabled != 0 || u.AuthVersion != t.AuthVersion {
		return models.User{}, "", ErrInvalid
	}
	scope := Scope(t)
	if scope == Admin && u.IsAdmin != 1 {
		return models.User{}, "", ErrInvalid
	}
	if scope != Admin && scope != Legacy && scope != Read && scope != Send {
		return models.User{}, "", ErrInvalid
	}
	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.Add(-time.Minute)
	// The integer CAS is portable even when legacy last_used_at rows have
	// mixed timestamp encodings. Keep the human-facing timestamp for the API.
	if (t.LastUsedEpoch > 0 && t.LastUsedEpoch <= cutoff.Unix()) || (t.LastUsedEpoch == 0 && (t.LastUsedAt == nil || !t.LastUsedAt.After(cutoff))) {
		next := models.APIToken{LastUsedAt: &now, LastUsedEpoch: now.Unix()}
		if _, err := db.Instance.ID(t.ID).Where("last_used_epoch<=?", cutoff.Unix()).Cols("last_used_at", "last_used_epoch").Update(&next); err != nil {
			log.Errorf("update API token %d last_used_at failed: %v", t.ID, err)
		}
	}

	return u, scope, nil
}
