package password

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var ErrEmpty = errors.New("password must not be empty")

const longPrefix = "$pmail-sha256$"

func Hash(value string) (string, error) {
	if value == "" {
		return "", ErrEmpty
	}
	input, prefix := []byte(value), ""
	if len(input) > 72 {
		// Preserve passwords from legacy accounts without truncating bcrypt input.
		sum := sha256.Sum256(input)
		input = []byte(hex.EncodeToString(sum[:]))
		prefix = longPrefix
	}
	b, err := bcrypt.GenerateFromPassword(input, bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return prefix + string(b), nil
}

func Verify(stored, value string) (valid, upgrade bool) {
	if value == "" {
		return false, false
	}
	if len(stored) == 32 {
		if _, err := hex.DecodeString(stored); err != nil {
			return false, false
		}
		valid = subtle.ConstantTimeCompare([]byte(strings.ToLower(stored)), []byte(Encode(value))) == 1
		return valid, valid
	}
	input := []byte(value)
	if hash, ok := strings.CutPrefix(stored, longPrefix); ok {
		sum := sha256.Sum256(input)
		input = []byte(hex.EncodeToString(sum[:]))
		stored = hash
	} else if len(input) > 72 {
		return false, false
	}
	return bcrypt.CompareHashAndPassword([]byte(stored), input) == nil, false
}
