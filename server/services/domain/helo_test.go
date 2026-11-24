package domain

import (
	"context"
	"net/netip"
	"testing"

	"github.com/Jinnrry/pmail/models"
)

func TestHeloHost(t *testing.T) {
	if got := HeloHost("Example.COM"); got != "smtp."+fallbackOrDefault("example.com") {
		t.Fatalf("default: %q", got)
	}
	t.Setenv("PMAIL_SMTP_HELO", "mx1.example.net.")
	if got := HeloHost("example.com"); got != "mx1.example.net" {
		t.Fatalf("override: %q", got)
	}
	for _, bad := range []string{"no-dot", "bad host.example", "-x.example", "a..b", "inject\r\nRSET.example"} {
		t.Setenv("PMAIL_SMTP_HELO", bad)
		if got := HeloHost("example.com"); got == bad || got == "inject" {
			t.Fatalf("invalid override %q used: %q", bad, got)
		}
	}
}

func fallbackOrDefault(fallback string) string {
	if d := DefaultName(); d != "" {
		return d
	}
	return fallback
}

// One IP has one PTR: a second domain's guide expects the shared EHLO host,
// not smtp.<second domain>.
func TestGuidePTRUsesSharedHeloHost(t *testing.T) {
	t.Setenv("PMAIL_SMTP_HELO", "smtp.example.com")
	ip := netip.MustParseAddr("203.0.113.10")
	guide := buildGuide(models.Domain{Name: "second.example", DkimSelector: "default"}, guideKey{web: "mail.second.example"}, ip, netip.Addr{})
	ptr := getRecord(t, guide, "ptr")
	if ptr.Value != "smtp.example.com." {
		t.Fatalf("PTR value %q", ptr.Value)
	}
	// guideDNS answers SMTP.Example.COM. for every address.
	checkGuideRecord(context.Background(), guideDNS{}, &ptr, guide.Domain, guide.MailHost, ip, netip.Addr{}, false)
	if ptr.Matches == nil || !*ptr.Matches || ptr.Status != "ok" {
		t.Fatalf("shared PTR not accepted: %+v", ptr)
	}
	t.Setenv("PMAIL_SMTP_HELO", "smtp.other.example")
	ptr = getRecord(t, buildGuide(models.Domain{Name: "second.example", DkimSelector: "default"}, guideKey{web: "mail.second.example"}, ip, netip.Addr{}), "ptr")
	checkGuideRecord(context.Background(), guideDNS{}, &ptr, "second.example", "smtp.second.example", ip, netip.Addr{}, false)
	if ptr.Matches == nil || *ptr.Matches || ptr.Status != "mismatch" {
		t.Fatalf("mismatched PTR accepted: %+v", ptr)
	}
}
