// Package releasebundle defines the version-bound, signed PMail bundle format.
package releasebundle

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PublicKeyBase64 contains the public Ed25519 key used to verify PMail releases.
const PublicKeyBase64 = "lMoV5lIfRnex/cjimFqsdWN9f/LtLsk+GPBMKIC8Bik="

const Checksums = "checksums.txt"
const Signature = "checksums.txt.sig"
const MaxManifest = 4096
const MaxDownload = 128 << 20

// Release tags are vMAJOR.MINOR.PATCH without prerelease or build suffixes.
var versionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`)
var ErrSignature = errors.New("invalid PMail release signature or manifest")

func Rank(tag string) int64 {
	m := versionPattern.FindStringSubmatch(tag)
	if len(m) == 0 {
		return -1
	}
	major, _ := strconv.ParseInt(m[1], 10, 64)
	minor, _ := strconv.ParseInt(m[2], 10, 64)
	patch, _ := strconv.ParseInt(m[3], 10, 64)
	return major*1_000_000_000_000 + minor*1_000_000 + patch
}

func Asset(arch string) string {
	switch arch {
	case "amd64", "arm64":
		return "pmail-linux-" + arch + ".tar.gz"
	}
	return ""
}

func PublicKey() ed25519.PublicKey {
	key, err := base64.StdEncoding.Strict().DecodeString(PublicKeyBase64)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil
	}
	return key
}

// Manifest includes the tag in the signed bytes to prevent re-labeling an
// older valid bundle as a newer release. Keep the order and line endings fixed.
func Manifest(tag string, amd64, arm64 [sha256.Size]byte) ([]byte, error) {
	if Rank(tag) < 0 {
		return nil, ErrSignature
	}
	return fmt.Appendf(nil, "# PMail release %s\n%x  %s\n%x  %s\n", tag, amd64, Asset("amd64"), arm64, Asset("arm64")), nil
}

func Verify(key ed25519.PublicKey, tag, arch string, data, signature []byte) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	if len(key) != ed25519.PublicKeySize || len(data) > MaxManifest || Rank(tag) < 0 || Asset(arch) == "" {
		return zero, ErrSignature
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSuffix(string(signature), "\n"))
	if err != nil || !ed25519.Verify(key, data, sig) {
		return zero, ErrSignature
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) != 4 || lines[0] != "# PMail release "+tag || lines[3] != "" {
		return zero, ErrSignature
	}
	var sums [2][sha256.Size]byte
	for i, a := range []string{"amd64", "arm64"} {
		hash, name, _ := strings.Cut(lines[i+1], "  ")
		n, err := hex.Decode(sums[i][:], []byte(hash))
		if err != nil || n != sha256.Size || name != Asset(a) {
			return zero, ErrSignature
		}
	}
	if arch == "amd64" {
		return sums[0], nil
	}
	return sums[1], nil
}
