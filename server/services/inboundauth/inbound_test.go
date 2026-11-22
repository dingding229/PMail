package inboundauth

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"

	"github.com/emersion/go-msgauth/dkim"
)

type testDNS struct {
	txt         map[string][]string
	unavailable map[string]bool
}

func (d testDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if d.unavailable[name] {
		return nil, errors.New("unavailable")
	}
	return d.txt[name], nil
}
func (testDNS) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) { return nil, nil }
func (testDNS) LookupMX(context.Context, string) ([]*net.MX, error)               { return nil, nil }
func (testDNS) LookupAddr(context.Context, string) ([]string, error)              { return nil, nil }

func TestC7InboundEnvelopeHELOAndAlignment(t *testing.T) {
	raw := []byte("From: user@mail.example.test\r\nSender: other@ignored.example.net\r\n\r\nbody")
	dns := testDNS{txt: map[string][]string{
		"envelope.example.net.": {"v=spf1 -all"},
		"bounce.example.test.":  {"v=spf1 ip4:192.0.2.22 -all"},
		"_dmarc.example.test.":  {"v=DMARC1; p=reject; sp=quarantine; aspf=s; adkim=r"},
	}}
	engine := Engine{DNS: dns, Relays: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}, VerifyDKIM: func(context.Context, []byte, Resolver) ([]*dkim.Verification, error) {
		return []*dkim.Verification{{Domain: "sign.example.test"}}, nil
	}}
	peer := netip.MustParseAddr("::ffff:192.0.2.22")
	report := engine.Verify(context.Background(), peer, "x@envelope.example.net", "bounce.example.test", raw)
	if report.SPF.Result != "fail" || report.SPF.Domain != "envelope.example.net" || report.DMARC.Result != "pass" || report.DMARC.Policy != "quarantine" || !report.TrustedRelay {
		t.Fatalf("envelope result: %+v", report)
	}
	report = engine.Verify(context.Background(), peer, "", "bounce.example.test", raw)
	if report.SPF.Result != "pass" || report.SPF.Aligned || report.SPF.Domain != "bounce.example.test" {
		t.Fatalf("HELO result: %+v", report)
	}
	dns.txt["_dmarc.example.test."] = []string{"v=DMARC1; p=reject; aspf=s; adkim=s"}
	report = engine.Verify(context.Background(), peer, "", "bounce.example.test", raw)
	if report.DMARC.Result != "fail" || report.DKIM.Aligned || report.DKIM.Result != "pass" {
		t.Fatalf("strict result: %+v", report)
	}
}

func TestC7SPFQualifiersAndBoundedInclude(t *testing.T) {
	for _, tc := range []struct {
		records  []string
		dnsError bool
		want     string
	}{
		{nil, false, "none"}, {[]string{"v=spf1 ?all"}, false, "neutral"}, {[]string{"v=spf1 ~all"}, false, "softfail"}, {[]string{"v=spf1 -all"}, false, "fail"}, {nil, true, "temperror"}, {[]string{"v=spf1 include:sender.example.test"}, false, "permerror"},
	} {
		dns := testDNS{txt: map[string][]string{"sender.example.test.": tc.records}, unavailable: map[string]bool{"sender.example.test.": tc.dnsError}}
		ip := netip.MustParseAddr("10.0.0.2")
		eval := spfEvaluation{dns: dns, ip: ip, sender: "a@sender.example.test"}
		if got := eval.evaluate(context.Background(), "sender.example.test"); got.Result != tc.want {
			t.Fatalf("records %v: %+v, want %s", tc.records, got, tc.want)
		}
	}
}
