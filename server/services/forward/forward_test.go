package forward

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/dto/parsemail"
)

func TestSRSRoundTrip(t *testing.T) {
	old := config.Instance
	config.Instance = &config.Config{SecretKey: "fixture"}
	t.Cleanup(func() { config.Instance = old })
	value, err := BuildSRS("alice@example.net", "forward.example.com")
	if err != nil {
		t.Fatal(err)
	}
	local, _, _ := strings.Cut(value, "@")
	if len(local) > 64 || !verifyBounce(local, "forward.example.com") || verifyBounce(local, "other.example.com") {
		t.Fatal("SRS length or domain binding")
	}
	if verifyBounce(local[:len(local)-1]+"!", "forward.example.com") {
		t.Fatal("tampered tag accepted")
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
