package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	log "github.com/sirupsen/logrus"
)

var ErrInvalid = errors.New("invalid API token")
var ErrExpired = errors.New("API token expired")

func hash(raw string) string { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }
func Create(userID int, name string, expires *time.Time) (models.APIToken, string, error) {
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
	row := models.APIToken{UserID: userID, Name: name, TokenHash: hash(raw), Prefix: raw[:8], AuthVersion: u.AuthVersion, ExpiresAt: expires, Created: time.Now()}
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
func Authenticate(raw string) (models.User, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return models.User{}, ErrInvalid
	}
	var t models.APIToken
	ok, err := db.Instance.Where("token_hash=?", hash(raw)).Get(&t)
	if err != nil || !ok {
		return models.User{}, ErrInvalid
	}
	if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
		return models.User{}, ErrExpired
	}
	u, err := user(t.UserID)
	if err != nil || u.Disabled != 0 || u.AuthVersion != t.AuthVersion {
		return models.User{}, ErrInvalid
	}
	now := time.Now()
	if _, err := db.Instance.Table(&models.APIToken{}).ID(t.ID).Cols("last_used_at").Update(map[string]any{"last_used_at": now}); err != nil {
		log.Errorf("update API token %d last_used_at failed: %v", t.ID, err)
	}
	return u, nil
}
