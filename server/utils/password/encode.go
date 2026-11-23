package password

import (
	"crypto/md5" // #nosec G501 -- read-only compatibility for verifying historical PMail hashes; no new credentials use MD5.
	"encoding/hex"
)

// legacyHash reproduces the original double-MD5 format for Verify only.
func legacyHash(password string) string {
	return md5Hex(md5Hex(password+"pmail") + "pmail2023")
}
func md5Hex(value string) string {
	digest := md5.Sum([]byte(value)) // #nosec G401 -- legacy password comparison only; successful auth upgrades the row to bcrypt before returning.
	return hex.EncodeToString(digest[:])
}
