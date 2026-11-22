package send

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type pinnedFixture struct{ lookup int }

func (*pinnedFixture) LookupMX(context.Context, string) ([]*net.MX, error) {
	return []*net.MX{{Host: "MX.EXAMPLE.NET.", Pref: 10}}, nil
}
func (d *pinnedFixture) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	if host != "mx.example.net." {
		return nil, fmt.Errorf("unexpected lookup %q", host)
	}
	d.lookup++
	return []netip.Addr{netip.MustParseAddr("::ffff:127.0.0.1"), netip.MustParseAddr("198.18.0.7"), netip.MustParseAddr("8.8.8.8")}, nil
}

func TestC7OutboundUsesPinnedAddressAndDoesNotDowngradeStrictTLS(t *testing.T) {
	dns := &pinnedFixture{}
	var attempts []string
	policy := Outbound{Resolver: dns, TLSRequired: map[string]bool{"recipient.example.net": true}, Dial: func(_ context.Context, _ string, address string) (net.Conn, error) {
		attempts = append(attempts, address)
		srv, client := net.Pipe()
		go func() {
			defer srv.Close()
			_ = srv.SetDeadline(time.Now().Add(3 * time.Second))
			if _, err := fmt.Fprint(srv, "220 fixture\r\n"); err != nil {
				return
			}
			r := bufio.NewReader(srv)
			if _, err := r.ReadString('\n'); err != nil {
				return
			}
			_, _ = fmt.Fprint(srv, "250 fixture\r\n")
		}()
		return client, nil
	}}
	err := policy.deliver(context.Background(), "recipient.example.net", "sender@example.test", "example.test", []string{"recipient@recipient.example.net"}, []byte("fixture"))
	if err == nil || !strings.Contains(err.Error(), "TLS") || dns.lookup != 1 || len(attempts) != 1 || attempts[0] != "8.8.8.8:25" {
		t.Fatalf("strict/pinned transport: %v, dns=%d, dials=%v", err, dns.lookup, attempts)
	}
	for _, s := range []string{"::ffff:127.0.0.1", "::ffff:10.3.2.1", "fd00::1", "2001:db8::1"} {
		if policy.allowed("mx.example.net", netip.MustParseAddr(s)) {
			t.Fatalf("reserved address accepted: %s", s)
		}
	}
	policy.AllowedHosts = map[string]bool{"mx.example.net": true}
	if !policy.allowed("mx.example.net", netip.MustParseAddr("::ffff:10.3.2.1")) || policy.allowed("other.example.net", netip.MustParseAddr("::ffff:10.3.2.1")) {
		t.Fatal("allow host is not exact")
	}
}
