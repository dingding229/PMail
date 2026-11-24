// Package seal stores versioned application secrets using the config volume key.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"github.com/Jinnrry/pmail/config"
)

var ErrSecret = errors.New("cannot access protected settings")

func aead(namespace string) (cipher.AEAD, error) {
	if config.Instance == nil || config.Instance.SecretKey == "" {
		return nil, ErrSecret
	}
	key := sha256.Sum256([]byte(namespace + "\x00" + config.Instance.SecretKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, ErrSecret
	}
	return cipher.NewGCM(block)
}

func Encode(namespace, field, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	a, err := aead(namespace)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", ErrSecret
	}
	return base64.RawStdEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(plain), []byte(field))), nil
}

func Decode(namespace, field, encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	a, err := aead(namespace)
	if err != nil {
		return "", err
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(raw) < a.NonceSize() {
		return "", ErrSecret
	}
	p, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], []byte(field))
	if err != nil {
		return "", ErrSecret
	}
	return string(p), nil
}
