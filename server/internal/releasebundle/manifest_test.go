package releasebundle

import (
	"crypto/ed25519"
	"testing"
)

func TestEmbeddedReleasePublicKey(t *testing.T) {
	if key := PublicKey(); len(key) != ed25519.PublicKeySize {
		t.Fatalf("embedded Ed25519 release public key decodes to %d bytes, want %d", len(key), ed25519.PublicKeySize)
	}
}
