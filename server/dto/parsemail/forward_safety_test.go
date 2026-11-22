package parsemail

import (
	"bytes"
	"net/mail"
	"testing"
)

func TestIsAutomaticMessage(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "auto replied", raw: "Auto-Submitted: auto-replied\r\n\r\nbody", want: true},
		{name: "auto generated", raw: "Auto-Submitted: auto-generated\r\n\r\nbody", want: true},
		{name: "auto no", raw: "Auto-Submitted: no\r\n\r\nbody", want: false},
		{name: "multipart report", raw: "Content-Type: multipart/report; report-type=delivery-status\r\n\r\nbody", want: true},
		{name: "body text is not header", raw: "Subject: ordinary\r\n\r\nAuto-Submitted: auto-replied", want: false},
		{name: "internal dsn marker", raw: "X-PMail-DSN: 1\r\n\r\nbody", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAutomaticMessage([]byte(tc.raw)); got != tc.want {
				t.Fatalf("IsAutomaticMessage()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldSkipForwardSources(t *testing.T) {
	cases := []struct {
		name  string
		email *Email
		raw   string
		want  bool
	}{
		{name: "mailer daemon", email: &Email{From: &User{EmailAddress: "mailer-daemon@example.com"}}, want: true},
		{name: "postmaster", email: &Email{From: &User{EmailAddress: "POSTMASTER@example.com"}}, want: true},
		{name: "empty envelope", email: &Email{From: &User{EmailAddress: "sender@example.com"}, EnvelopeFromKnown: true}, want: true},
		{name: "ordinary mail", email: &Email{From: &User{EmailAddress: "sender@example.com"}}, raw: "Subject: hello\r\n\r\nbody", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldSkipForward(tc.email, []byte(tc.raw)); got != tc.want {
				t.Fatalf("ShouldSkipForward()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestBounceHeaderBuildBytes(t *testing.T) {
	email := &Email{
		From:    &User{EmailAddress: "mailer-daemon@example.com"},
		To:      []*User{{EmailAddress: "user@example.com"}},
		Headers: map[string][]string{"Auto-Submitted": {"auto-replied"}, "X-PMail-DSN": {"1"}},
		Text:    []byte("failure"),
	}
	raw, err := email.BuildBytes(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("generated bounce is not a valid message: %v", err)
	}
	if got := message.Header.Get("Auto-Submitted"); got != "auto-replied" {
		t.Fatalf("Auto-Submitted=%q, want auto-replied", got)
	}
	if got := message.Header.Get("X-PMail-DSN"); got != "1" {
		t.Fatalf("X-PMail-DSN=%q, want 1", got)
	}
}
