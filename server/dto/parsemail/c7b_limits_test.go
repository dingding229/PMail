package parsemail

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/utils/mailbudget"
)

func TestC7bMIMEPartCountAndSize(t *testing.T) {
	const header = "From: fixture@example.test\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n"
	const part = "--x\r\nContent-Type: text/plain\r\n\r\nhello\r\n"
	message := header + strings.Repeat(part, mailbudget.Parts) + "--x--\r\n"
	if _, err := ParseLimited(nil, strings.NewReader(message), len(message)); !errors.Is(err, mailbudget.ErrLimit) {
		t.Fatalf("MIME part limit: %v", err)
	}
	if _, err := ParseLimited(nil, strings.NewReader(""), mailbudget.Message+1); !errors.Is(err, mailbudget.ErrLimit) {
		t.Fatalf("raw mail limit: %v", err)
	}
	large := "Content-Type: application/octet-stream\r\n\r\n" + strings.Repeat("x", mailbudget.Part+1)
	if _, err := ParseLimited(nil, strings.NewReader(large), len(large)); !errors.Is(err, mailbudget.ErrLimit) {
		t.Fatalf("MIME part bytes: %v", err)
	}
}
