package auth

import (
	"os"
	"path/filepath"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/dkimkey"
	"github.com/Jinnrry/pmail/utils/securefile"
	log "github.com/sirupsen/logrus"
)

// AdminMailOverride is reserved for deliberate, audited operator access.
func AdminMailOverride(ctx *context.Context, email *models.Email) bool {
	if !ctx.IsAdmin || os.Getenv("PMAIL_ADMIN_MAIL_ACCESS") != "1" || email.Status == 4 {
		return false
	}
	log.WithContext(ctx).Warnf("audit_event=admin_cross_mail_access actor_id=%d email_id=%d", ctx.UserID, email.Id)
	return true
}

func HasAuth(ctx *context.Context, email *models.Email) bool {
	linked, err := db.Instance.Where("email_id=? AND user_id=?", email.Id, ctx.UserID).Exist(&models.UserEmail{})
	if err != nil {
		log.Errorf("mail access check failed: %v", err)
		return false
	}
	return linked || AdminMailOverride(ctx, email)
}

// EnsureDKIM maintains the legacy setup wizard's DNS record file.
func EnsureDKIM(path string) (string, error) {
	data, err := securefile.Read(path)
	if os.IsNotExist(err) {
		raw, err := dkimkey.Generate()
		if err != nil {
			return "", err
		}
		data = []byte(raw)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		if err := securefile.Write(path, data, true); os.IsExist(err) {
			return EnsureDKIM(path)
		} else if err != nil {
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
