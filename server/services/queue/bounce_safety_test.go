package queue

import (
	"testing"
	"time"

	"github.com/Jinnrry/pmail/models"
)

func TestShouldSuppressBounce(t *testing.T) {
	cases := []struct {
		name string
		row  models.OutboundQueue
		want bool
	}{
		{name: "empty envelope", row: models.OutboundQueue{MailFrom: ""}, want: true},
		{name: "mailer daemon", row: models.OutboundQueue{MailFrom: "mailer-daemon@example.com"}, want: true},
		{name: "postmaster", row: models.OutboundQueue{MailFrom: "postmaster@example.com"}, want: true},
		{name: "srs mailer daemon", row: models.OutboundQueue{MailFrom: "SRS0=ABCD=AA=example.com=mailer-daemon@example.com"}, want: true},
		{name: "auto submitted", row: models.OutboundQueue{MailFrom: "sender@example.com", RawData: []byte("Auto-Submitted: auto-replied\r\n\r\nbody")}, want: true},
		{name: "multipart report", row: models.OutboundQueue{MailFrom: "sender@example.com", RawData: []byte("Content-Type: multipart/report\r\n\r\nbody")}, want: true},
		{name: "ordinary", row: models.OutboundQueue{MailFrom: "sender@example.com", RawData: []byte("Subject: hello\r\n\r\nbody")}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldSuppressBounce(tc.row); got != tc.want {
				t.Fatalf("ShouldSuppressBounce()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestBounceNoticeRecentWindow(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.Local)
	cases := []struct {
		name    string
		created time.Time
		want    bool
	}{
		{name: "inside window", created: now.Add(-23 * time.Hour), want: true},
		{name: "at boundary", created: now.Add(-24 * time.Hour), want: false},
		{name: "outside window", created: now.Add(-24*time.Hour - time.Second), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bounceNoticeRecent(tc.created, now); got != tc.want {
				t.Fatalf("bounceNoticeRecent()=%v, want %v", got, tc.want)
			}
		})
	}
}
