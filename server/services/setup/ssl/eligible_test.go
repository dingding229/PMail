package ssl

import (
	"context"
	"net"
	"net/netip"
	"slices"
	"testing"

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

	keep, skipped := requestDomains(context.Background(), cfg)
	want := []string{"mail.a.test", "smtp.a.test", "imap.a.test", "pop.b.test", "imap.b.test"}
	if !slices.Equal(keep, want) {
		t.Fatalf("keep = %v, want %v", keep, want)
	}
	if len(skipped) != 2 || skipped[0].Name != "pop.a.test" || skipped[0].Actual[0] != cdn.String() || skipped[1].Name != "smtp.b.test" {
		t.Fatalf("skipped = %+v", skipped)
	}

	cfg.SSLType = config.SSLTypeAutoDNS
	if keep, skipped = requestDomains(context.Background(), cfg); len(keep) != 7 || skipped != nil {
		t.Fatalf("DNS-01 must keep every name: %v %v", keep, skipped)
	}

	cfg.SSLType = config.SSLTypeAutoHTTP
	serverAddresses = func(context.Context) (netip.Addr, netip.Addr) { return netip.Addr{}, netip.Addr{} }
	if keep, _ = requestDomains(context.Background(), cfg); len(keep) != 7 {
		t.Fatalf("unknown public address must keep every name: %v", keep)
	}
}
