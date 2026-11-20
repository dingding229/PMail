package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/context"
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

// EnsureDKIM generates a private key only if missing and derives its DNS record.
func EnsureDKIM(path string) (string, error) {
	key, err := parsemail.LoadPrivateKey(path)
	if os.IsNotExist(err) {
		private, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", err
		}
		encoded, err := x509.MarshalPKCS8PrivateKey(private)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0600); err != nil {
			return "", err
		}
		key = private
	} else if err != nil {
		return "", err
	}

	var record string
	switch public := key.Public().(type) {
	case *rsa.PublicKey:
		encoded, err := x509.MarshalPKIXPublicKey(public)
		if err != nil {
			return "", err
		}
		record = "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(encoded)
	case ed25519.PublicKey:
		record = "v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(public)
	default:
		return "", fmt.Errorf("unsupported DKIM key type %T", public)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "dkim.public"), []byte(record), 0644); err != nil {
		return "", err
	}
	return record, nil
}
