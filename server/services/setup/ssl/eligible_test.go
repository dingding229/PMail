package ssl

import (
	"context"
	"crypto/x509"
	"net"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
)

func TestRequestDomainsSkipsHostsNotPointingHere(t *testing.T) {
	server := netip.MustParseAddr("185.99.135.205")
	cdn := netip.MustParseAddr("104.21.91.140")
	oldLookup, oldAddr := hostLookup, serverAddresses
	t.Cleanup(func() { hostLookup, serverAddresses = oldLookup, oldAddr })
	serverAddresses = func(context.Context) (netip.Addr, netip.Addr) { return server, netip.Addr{} }
	records := map[string][]netip.Addr{
		"smtp.a.test": {server},
		"pop.a.test":  {cdn},
		"imap.a.test": {netip.MustParseAddr("::ffff:185.99.135.205")},
	}
	hostLookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		switch host {
		case "smtp.b.test":
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		case "pop.b.test", "imap.b.test":
			return nil, &net.DNSError{Err: "i/o timeout", Name: host, IsTimeout: true}
		}
		return records[host], nil
	}
	cfg := &config.Config{SSLType: config.SSLTypeAutoHTTP, HttpsEnabled: 1, WebDomain: "mail.a.test", Domains: []string{"a.test", "b.test"}}

	keep, skipped, uncertain := requestDomains(context.Background(), cfg)
	want := []string{"mail.a.test", "smtp.a.test", "imap.a.test"}
	if !slices.Equal(keep, want) {
		t.Fatalf("keep = %v, want %v", keep, want)
	}
	if !slices.Equal(uncertain, []string{"pop.b.test", "imap.b.test"}) {
		t.Fatalf("uncertain = %v", uncertain)
	}
	if len(skipped) != 2 || skipped[0].Name != "pop.a.test" || skipped[0].Actual[0] != cdn.String() || skipped[1].Name != "smtp.b.test" {
		t.Fatalf("skipped = %+v", skipped)
	}

	cfg.SSLType = config.SSLTypeAutoDNS
	if keep, skipped, uncertain = requestDomains(context.Background(), cfg); len(keep) != 7 || skipped != nil || uncertain != nil {
		t.Fatalf("DNS-01 must keep every name: %v %v", keep, skipped)
	}

	cfg.SSLType = config.SSLTypeAutoHTTP
	serverAddresses = func(context.Context) (netip.Addr, netip.Addr) { return netip.Addr{}, netip.Addr{} }
	if keep, _, uncertain = requestDomains(context.Background(), cfg); !slices.Equal(keep, []string{"mail.a.test"}) || len(uncertain) != 5 {
		t.Fatalf("unknown addresses must not nominate mail names: %v; uncertain=%v", keep, uncertain)
	}
}

func TestRenewalUsesConfirmedSnapshotWithoutSANChurn(t *testing.T) {
	oldLookup, oldAddr := hostLookup, serverAddresses
	t.Cleanup(func() { hostLookup, serverAddresses = oldLookup, oldAddr })
	v4 := netip.MustParseAddr("192.0.2.1")
	v6 := netip.MustParseAddr("2001:db8::1")
	wrong := netip.MustParseAddr("2001:db8::2")
	calls := 0
	serverAddresses = func(context.Context) (netip.Addr, netip.Addr) { calls++; return v4, v6 }
	records := map[string][]netip.Addr{
		"smtp.a.test": {v4, wrong}, // A matches but AAAA is stale.
		"imap.a.test": {v6},        // IPv6-only is valid.
	}
	hostLookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "pop.a.test" {
			return []netip.Addr{v4}, &net.DNSError{IsTimeout: true}
		}
		return records[host], nil
	}
	ctx := context.Background()
	cfg := &config.Config{SSLType: config.SSLTypeAutoHTTP, HttpsEnabled: 2, WebDomain: "smtp.a.test", Domains: []string{"a.test"}}
	cert := &x509.Certificate{DNSNames: []string{"smtp.a.test", "pop.a.test", "imap.a.test"}, NotAfter: time.Now().Add(90 * 24 * time.Hour)}
	if names, err := renewalDomains(ctx, cfg, cert); len(names) != 0 || err != nil || calls != 0 {
		t.Fatalf("fully covered healthy cert must not query DNS: %v, %v (%d calls)", names, err, calls)
	}
	cert.DNSNames = []string{"smtp.a.test"}
	if names, err := renewalDomains(ctx, cfg, cert); len(names) != 0 || err != nil {
		t.Fatalf("healthy certificate must not exchange SMTP for IMAP: %v, %v", names, err)
	}
	cert.DNSNames = []string{"imap.a.test"}
	if names, err := renewalDomains(ctx, cfg, cert); len(names) != 0 || err != nil {
		t.Fatalf("mixed addresses and partial DNS responses must not trigger expansion: %v, %v", names, err)
	}
	records["smtp.a.test"] = []netip.Addr{v4, v6}
	if names, err := renewalDomains(ctx, cfg, cert); !slices.Equal(names, []string{"smtp.a.test", "imap.a.test"}) || err != nil {
		t.Fatalf("confirmed expansion: %v, %v", names, err)
	}
	cert.NotAfter = time.Now().Add(time.Hour)
	cert.DNSNames = append(cert.DNSNames, "pop.a.test")
	if names, err := renewalDomains(ctx, cfg, cert); len(names) != 0 || err == nil {
		t.Fatalf("temporary DNS failure must not remove an existing name at renewal: %v, %v", names, err)
	}
}

func TestACMEResolverRequiresCompleteAddressResults(t *testing.T) {
	for _, nameserver := range []string{"", "127.0.0.1:53"} {
		t.Setenv("PMAIL_DNS_RESOLVER", nameserver)
		r, err := acmeResolver()
		if err != nil {
			t.Fatal(err)
		}
		if !r.PreferGo || !r.StrictErrors || (r.Dial != nil) != (nameserver != "") {
			t.Fatal("ACME needs strict A/AAAA resolution with the configured DNS server")
		}
	}
}
