package external

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/models"
	"golang.org/x/crypto/hkdf"
)

const passwordVersion = "v2"
const legacyPasswordVersion = "v1"

type endpoint struct {
	ID                          int
	Host                        string
	Port                        int
	Username, Protocol, TLSMode string
	InsecureSkipVerify          bool
}

func binding(a models.ExternalAccount) endpoint {
	return endpoint{a.ID, strings.ToLower(strings.TrimSuffix(strings.TrimSpace(a.Host), ".")), a.Port, a.Username, a.Protocol, a.TLSMode, a.InsecureSkipVerify}
}

func (e endpoint) additionalData() []byte {
	b, _ := json.Marshal(e)
	return b
}

func passwordCipher(version string) (cipher.AEAD, error) {
	if config.Instance.SecretKey == "" {
		return nil, errors.New("PMAIL_SECRET_KEY is not configured")
	}
	key := make([]byte, 32)
	reader := hkdf.New(sha256.New, []byte(config.Instance.SecretKey), []byte("pmail-external-password"), []byte("aes-256-gcm-"+version))
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func sealPassword(version, password string, aad []byte) (string, error) {
	gcm, err := passwordCipher(version)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := gcm.Seal(nonce, nonce, []byte(password), aad)
	return fmt.Sprintf("%s:%s", version, base64.RawStdEncoding.EncodeToString(payload)), nil
}

func encryptAccount(a models.ExternalAccount, password string) (string, error) {
	if a.ID <= 0 {
		return "", errors.New("external account ID is required before encrypting")
	}
	return sealPassword(passwordVersion, password, binding(a).additionalData())
}

func decryptAccount(a models.ExternalAccount) (password string, legacy bool, err error) {
	version, encoding, ok := strings.Cut(a.PasswordEnc, ":")
	if !ok || (version != passwordVersion && version != legacyPasswordVersion) {
		return "", false, errors.New("unsupported external password version")
	}
	payload, err := base64.RawStdEncoding.DecodeString(encoding)
	if err != nil {
		return "", false, err
	}
	gcm, err := passwordCipher(version)
	if err != nil {
		return "", false, err
	}
	if len(payload) < gcm.NonceSize() {
		return "", false, errors.New("invalid external password payload")
	}
	legacy = version == legacyPasswordVersion
	var aad []byte
	if !legacy {
		aad = binding(a).additionalData()
	}
	plain, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], aad)
	return string(plain), legacy, err
}
