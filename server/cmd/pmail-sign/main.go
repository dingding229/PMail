// pmail-sign signs existing release bundles without writing private key material.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Jinnrry/pmail/internal/releasebundle"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: pmail-sign vX.Y.Z <archive-directory>")
		os.Exit(2)
	}
	if err := sign(os.Args[1], os.Args[2], os.Getenv("PMAIL_RELEASE_SIGNING_KEY"), releasebundle.PublicKey()); err != nil {
		fmt.Fprintln(os.Stderr, "sign release:", err)
		os.Exit(1)
	}
}

func sign(tag, dir, encoded string, public ed25519.PublicKey) error {
	seed, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(seed) != ed25519.SeedSize {
		return errors.New("PMAIL_RELEASE_SIGNING_KEY must be a base64-encoded 32-byte Ed25519 seed")
	}
	key := ed25519.NewKeyFromSeed(seed)
	clear(seed)
	defer clear(key)
	if !key.Public().(ed25519.PublicKey).Equal(public) {
		return errors.New("signing key does not match the configured release public key")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	var sums [2][sha256.Size]byte
	for i, arch := range []string{"amd64", "arm64"} {
		f, err := root.Open(releasebundle.Asset(arch))
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(f, releasebundle.MaxDownload+1))
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
		if n == 0 || n > releasebundle.MaxDownload {
			return errors.New("invalid release archive size")
		}
		copy(sums[i][:], h.Sum(nil))
	}
	body, err := releasebundle.Manifest(tag, sums[0], sums[1])
	if err != nil {
		return err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, body)) + "\n"
	for name, data := range map[string][]byte{releasebundle.Checksums: body, releasebundle.Signature: []byte(sig)} {
		if err := root.WriteFile(name, data, 0600); err != nil {
			return err
		}
	}
	return nil
}
