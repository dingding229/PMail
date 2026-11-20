package external

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"golang.org/x/crypto/hkdf"
)

const passwordVersion = "v1"

func deriveKey() ([]byte, error) {
	if config.Instance.SecretKey == "" {
		return nil, errors.New("PMAIL_SECRET_KEY is not configured")
	}
	reader := hkdf.New(sha256.New, []byte(config.Instance.SecretKey), []byte("pmail-external-password"), []byte("aes-256-gcm-v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func EncryptPassword(password string) (string, error) {
	key, err := deriveKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(password), nil)
	payload := append(nonce, sealed...)
	return fmt.Sprintf("%s:%s", passwordVersion, base64.RawStdEncoding.EncodeToString(payload)), nil
}

func DecryptPassword(value string) (string, error) {
	version, encoded, ok := strings.Cut(value, ":")
	if !ok || version != passwordVersion {
		return "", errors.New("unsupported external password version")
	}
	payload, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	key, err := deriveKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("invalid external password payload")
	}
	plain, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], nil)
	return string(plain), err
}
