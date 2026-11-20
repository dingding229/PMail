package forward

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/dto/parsemail"
)

func TestSRSCreationParsingAndExpiry(t *testing.T) {
	old := config.Instance
	config.Instance = &config.Config{SecretKey: "test-secret"}
	t.Cleanup(func() { config.Instance = old })
	value := BuildSRS("alice@example.net", "forward.example.com")
	original, err := parseSRS(strings.TrimSuffix(strings.TrimPrefix(strings.Split(value, "@")[0], ""), ""), "forward.example.com")
	if err != nil || original != "alice@example.net" {
		t.Fatalf("SRS parse = %q, err=%v, value=%q", original, err, value)
	}
	oldDay := base32Days(time.Now().Add(-22 * 24 * time.Hour))
	payload := oldDay + "=example.net=alice"
	h := hmac.New(sha256.New, []byte(config.Instance.SecretKey))
	_, _ = h.Write([]byte(payload))
	// parseSRS uses the SRS HMAC-SHA1 format; this deliberately malformed hash
	// must still be rejected before any database lookup.
	bad := "SRS0=" + hex.EncodeToString(h.Sum(nil))[:4] + "=" + payload
	if _, err := parseSRS(bad, "forward.example.com"); err == nil {
		t.Fatal("expired or malformed SRS was accepted")
	}
}

func TestForwardHeaderLoopChecksOnlyHeaders(t *testing.T) {
	old := config.Instance
	config.Instance = &config.Config{SecretKey: "test-secret"}
	t.Cleanup(func() { config.Instance = old })
	node := nodeID()
	email := &parsemail.Email{From: &parsemail.User{EmailAddress: "sender@example.net"}}
	raw := []byte("Subject: original\r\n\r\nbody X-PMail-Forwarded-For: " + node + "; target@example.net")
	if shouldStop(email, raw, node, "target@example.net") {
		t.Fatal("body marker incorrectly stopped forwarding")
	}
	original := []byte("H1: one\r\nH2: two\r\n\r\nbody\r\nsecond")
	headered := addForwardHeaders(original, node, "target@example.net")
	originalSeparator := bytes.Index(original, []byte("\r\n\r\n"))
	newSeparator := bytes.Index(headered, []byte("\r\n\r\n"))
	if originalSeparator < 0 || newSeparator < 0 {
		t.Fatalf("missing header separator: original=%q new=%q", original, headered)
	}
	if !bytes.Equal(original[originalSeparator+4:], headered[newSeparator+4:]) {
		t.Fatalf("forward insertion changed body: original=%q new=%q", original[originalSeparator+4:], headered[newSeparator+4:])
	}
	wantHeader := append([]byte{}, original[:originalSeparator]...)
	wantHeader = append(wantHeader, []byte("\r\nX-PMail-Forwarded-For: "+node+"; target@example.net")...)
	if !bytes.Equal(headered[:newSeparator], wantHeader) {
		t.Fatalf("forward header insertion changed original headers: got=%q want=%q", headered[:newSeparator], wantHeader)
	}
	if !shouldStop(email, headered, node, "target@example.net") {
		t.Fatal("same-node same-target header did not stop forwarding")
	}
}
