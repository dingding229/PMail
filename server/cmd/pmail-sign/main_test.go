package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jinnrry/pmail/internal/releasebundle"
)

func TestSignWithPairedEphemeralKey(t *testing.T) {
	dir := t.TempDir()
	for _, arch := range []string{"amd64", "arm64"} {
		if err := os.WriteFile(filepath.Join(dir, releasebundle.Asset(arch)), []byte(arch), 0600); err != nil {
			t.Fatal(err)
		}
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	seed := base64.StdEncoding.EncodeToString(private.Seed())
	if err := sign("v0.0.4", dir, seed, nil); err == nil {
		t.Fatal("missing embedded public key must fail release")
	}
	if err := sign("v0.0.4", dir, "not a seed", public); err == nil {
		t.Fatal("invalid secret accepted")
	}
	mismatched := append(ed25519.PublicKey(nil), public...)
	mismatched[0] ^= 1
	if err := sign("v0.0.4", dir, seed, mismatched); err == nil {
		t.Fatal("mismatched signing key accepted")
	}
	for _, name := range []string{releasebundle.Checksums, releasebundle.Signature} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("rejected signing attempt must not create %s: %v", name, err)
		}
	}
	if err := sign("v0.0.4", dir, seed, public); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, releasebundle.Checksums))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := os.ReadFile(filepath.Join(dir, releasebundle.Signature))
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		hash, err := releasebundle.Verify(public, "v0.0.4", arch, manifest, sig)
		if err != nil || hash != sha256.Sum256([]byte(arch)) {
			t.Fatalf("%s failed to verify: %v", arch, err)
		}
	}
}
