package auth

import (
	"os"
	"path/filepath"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/dkimkey"
	log "github.com/sirupsen/logrus"
)

// HasAuth 检查当前用户是否有某个邮件的auth
func HasAuth(ctx *context.Context, email *models.Email) bool {
	if ctx.IsAdmin {
		return true
	}
	var ue []models.UserEmail
	err := db.Instance.Table(&models.UserEmail{}).Where("email_id = ? and user_id = ?", email.Id, ctx.UserID).Find(&ue)
	if err != nil {
		log.Errorf("Error while checking user: %v", err)
		return false
	}

	return len(ue) != 0
}

// EnsureDKIM maintains the legacy setup wizard's DNS record file.
func EnsureDKIM(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		raw, err := dkimkey.Generate()
		if err != nil {
			return "", err
		}
		data = []byte(raw)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	key, err := dkimkey.Parse(string(data))
	if err != nil {
		return "", err
	}
	record, err := dkimkey.Record(key)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "dkim.public"), []byte(record), 0644); err != nil {
		return "", err
	}
	return record, nil
}
