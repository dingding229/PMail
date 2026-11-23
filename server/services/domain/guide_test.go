package domain

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/models"
)

type guideDNS struct {
	txt       map[string][]string
	addresses map[string][]netip.Addr
	mx        []*net.MX
}

func (g guideDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	return g.txt[name], nil
}
func (g guideDNS) LookupMX(context.Context, string) ([]*net.MX, error) {
	if g.mx != nil {
		return g.mx, nil
	}
	return []*net.MX{{Host: "SMTP.EXAMPLE.COM", Pref: 10}}, nil
}
func (g guideDNS) LookupNetIP(_ context.Context, _, name string) ([]netip.Addr, error) {
	return g.addresses[name], nil
}
func (g guideDNS) LookupAddr(context.Context, string) ([]string, error) {
	return []string{"SMTP.Example.COM."}, nil
}
func (g guideDNS) LookupCNAME(context.Context, string) (string, error) {
	return "mail.example.com.", nil
}
func (g guideDNS) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, nil
}

func getRecord(t *testing.T, g DNSGuide, key string) GuideRecord {
	t.Helper()
	for _, r := range g.Records {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("record %q missing", key)
	return GuideRecord{}
}
func TestDNSRecordGenerationAndZone(t *testing.T) {
	row := models.Domain{Name: "example.com", DkimSelector: "default"}
	key := guideKey{web: "mail.example.com", dkim: "v=DKIM1; k=rsa; p=" + strings.Repeat("A", 390)}
	v4, v6 := netip.MustParseAddr("203.0.113.10"), netip.MustParseAddr("2001:db8::10")
	guide := localizeGuide(buildGuide(row, key, v4, v6), "zh-CN")
	mx := getRecord(t, guide, "mx")
	if mx.Host != "@" || mx.FQDN != "example.com." || mx.Value != "smtp.example.com." || *mx.Priority != 10 {
		t.Fatalf("MX=%+v", mx)
	}
	if r := getRecord(t, guide, "mail_aaaa"); !r.Required || r.Host != "smtp" {
		t.Fatalf("AAAA=%+v", r)
	}
	if r := getRecord(t, guide, "dkim"); len(r.ValueChunks) != 2 || len(r.ValueChunks[0]) != 255 || strings.Join(r.ValueChunks, "") != key.dkim {
		t.Fatalf("invalid TXT chunks")
	}
	if a := normalizeDKIM(" V = dkim1; P = ABC \tdef; K=RSA "); a != normalizeDKIM("v=DKIM1;p=ABCdef") {
		t.Fatal("DKIM normalization disagrees")
	}
	if normalizeDKIM("p=AAA") == normalizeDKIM("p=aaa") {
		t.Fatal("DKIM base64 case must stay significant")
	}
	zone := renderZone(guide, "zh-CN")
	for _, want := range []string{"$ORIGIN example.com.", "@ 3600 IN MX 10 smtp.example.com.", `@ 3600 IN TXT "v=spf1 mx a ~all"`, `; _submission._tcp 3600 IN SRV 10 0 587 smtp.example.com.`, `" "AAAA`} {
		if !strings.Contains(zone, want) {
			t.Errorf("zone missing %q: %s", want, zone)
		}
	}
	if strings.Contains(zone, " IN PTR ") {
		t.Fatal("reverse DNS was emitted in a forward zone")
	}
	key.web = "smtp.example.com"
	for _, r := range buildGuide(row, key, v4, netip.Addr{}).Records {
		if r.Key == "web_a" || (r.Key == "mail_aaaa" && r.Required) {
			t.Fatalf("unexpected duplicate or required AAAA: %+v", r)
		}
	}
}

func TestSPFAndProxyRecordChecks(t *testing.T) {
	const domain = "example.com"
	ip := netip.MustParseAddr("203.0.113.10")
	fake := guideDNS{txt: map[string][]string{"_spf.example.com.": {"v=spf1 ip4:203.0.113.0/24 -all"}}, addresses: map[string][]netip.Addr{"smtp.example.com.": {ip}, "mail.example.com.": {netip.MustParseAddr("203.0.113.11")}}}
	for _, tc := range []struct {
		record string
		want   spfResult
	}{{"v=spf1 include:_spf.example.com -all", spfPass}, {"v=spf1 mx a ~all", spfPass}, {"v=spf1 -ip4:203.0.113.10 +all", spfNoPass}, {"v=spf1 redirect=_spf.example.com", spfPass}, {"v=spf1 ip4:203.0.113.11 ~all", spfNoPass}} {
		e := spfEvaluator{r: fake, remaining: 10}
		if got := e.check(context.Background(), domain, tc.record, ip); got != tc.want {
			t.Errorf("SPF %q=%v want %v", tc.record, got, tc.want)
		}
	}
	fake.txt["example.com."] = []string{"v=spf1 mx ~all", "V=SPF1 ip4:203.0.113.10 -all", "unrelated=TXT"}
	guide := buildGuide(models.Domain{Name: domain, DkimSelector: "default"}, guideKey{web: "mail.example.com", proxy: true}, ip, netip.Addr{})
	spf := getRecord(t, guide, "spf")
	checkGuideRecord(context.Background(), fake, &spf, domain, guide.MailHost, ip, netip.Addr{}, true)
	if spf.Status != "mismatch" || spf.hint != "dns_guide_spf_merge" || len(spf.Actual) != 2 || spf.SuggestedValue != "v=spf1 mx ip4:203.0.113.10 a ~all" {
		t.Fatalf("SPF merge: %+v", spf)
	}
	web := getRecord(t, guide, "web_a")
	checkGuideRecord(context.Background(), fake, &web, domain, guide.MailHost, ip, netip.Addr{}, true)
	if web.Status != "ok" {
		t.Fatalf("proxy A must check existence, not IP: %+v", web)
	}
	ptr := getRecord(t, guide, "ptr")
	checkGuideRecord(context.Background(), fake, &ptr, domain, guide.MailHost, ip, netip.Addr{}, true)
	if ptr.Status != "advice" || ptr.Matches == nil || !*ptr.Matches {
		t.Fatalf("PTR: %+v", ptr)
	}
}

func TestSuggestedSPFPreservesMechanismsAndPrimaryPolicy(t *testing.T) {
	for _, tc := range []struct {
		records []string
		want    string
	}{
		{[]string{"v=spf1 include:_spf.provider.test ip4:192.0.2.1 -all"}, "v=spf1 include:_spf.provider.test ip4:192.0.2.1 mx a -all"},
		{[]string{"v=spf1 +mx ~all"}, "v=spf1 +mx a ~all"},
		{[]string{"V=SPF1 a ?ALL"}, "V=SPF1 a mx ?ALL"},
		{[]string{"v=spf1 include:old.test ~all exp=explain.test", "v=spf1 include:other.test ip6:2001:db8::/32 include:old.test -all"}, "v=spf1 include:old.test include:other.test ip6:2001:db8::/32 mx a ~all exp=explain.test"},
		{[]string{"v=spf1 redirect=_spf.provider.test"}, "v=spf1 redirect=_spf.provider.test mx a"},
	} {
		if got := suggestSPF(tc.records); got != tc.want {
			t.Errorf("records %q: %q != %q", tc.records, got, tc.want)
		}
	}
	ip := netip.MustParseAddr("203.0.113.10")
	guide := buildGuide(models.Domain{Name: "example.com", DkimSelector: "default"}, guideKey{web: "mail.example.com"}, ip, netip.Addr{})
	for _, tc := range []struct{ record, want string }{{"v=spf1 ip4:203.0.113.10 -all", ""}, {"v=spf1 ip4:203.0.113.11 -all", "v=spf1 ip4:203.0.113.11 mx a -all"}} {
		fake := guideDNS{txt: map[string][]string{"example.com.": {tc.record}}}
		r := getRecord(t, guide, "spf")
		checkGuideRecord(context.Background(), fake, &r, guide.Domain, guide.MailHost, ip, netip.Addr{}, false)
		if r.SuggestedValue != tc.want {
			t.Errorf("SPF %q suggestion: %q != %q", tc.record, r.SuggestedValue, tc.want)
		}
	}
}

func TestMXAliasAndForeignHostsAndProxiedMailHost(t *testing.T) {
	const domain = "example.com"
	ip := netip.MustParseAddr("203.0.113.10")
	cdn := netip.MustParseAddr("104.21.91.140")
	fake := guideDNS{addresses: map[string][]netip.Addr{
		"_dc-mx.abc.example.com.":   {ip},
		"route1.mx.cloudflare.net.": {netip.MustParseAddr("162.159.205.11")},
		"smtp.example.com.":         {cdn},
	}, mx: []*net.MX{{Host: "_dc-mx.abc.example.com.", Pref: 1}, {Host: "route1.mx.cloudflare.net.", Pref: 13}}}
	guide := buildGuide(models.Domain{Name: domain, DkimSelector: "default"}, guideKey{web: "mail.example.com", proxy: true}, ip, netip.Addr{})

	mx := getRecord(t, guide, "mx")
	checkGuideRecord(context.Background(), fake, &mx, domain, guide.MailHost, ip, netip.Addr{}, true)
	if mx.Status != "ok" || mx.hint != "dns_guide_mx_foreign" {
		t.Fatalf("MX alias pointing here must be ok and foreign MX flagged: %+v", mx)
	}

	fake.mx = []*net.MX{{Host: "_dc-mx.abc.example.com.", Pref: 1}}
	mx = getRecord(t, guide, "mx")
	checkGuideRecord(context.Background(), fake, &mx, domain, guide.MailHost, ip, netip.Addr{}, true)
	if mx.Status != "ok" || mx.hint != "" {
		t.Fatalf("single MX alias pointing here: %+v", mx)
	}

	a := getRecord(t, guide, "mail_a")
	checkGuideRecord(context.Background(), fake, &a, domain, guide.MailHost, ip, netip.Addr{}, true)
	if a.Status != "mismatch" || a.hint != "dns_guide_mail_proxied" {
		t.Fatalf("proxied mail host: %+v", a)
	}
}
