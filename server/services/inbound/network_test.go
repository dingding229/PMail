package inbound

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/dto/parsemail"
)

func TestDNSBLCacheResolverRefusalAndTrustedRelay(t *testing.T) {
	n := networkState{cached: map[string]netResult{}, warnings: map[string]netResult{}}
	raw := []byte("Received: from peer.example ([203.0.113.9]) by relay.example; Now\r\nReceived: from other ([203.0.113.5]) by peer.example; Then\r\nFrom: sender@example.test\r\n\r\nFixture")
	t.Setenv("PMAIL_TRUSTED_RELAYS", "192.0.2.3/32")
	relay := netip.MustParseAddr("192.0.2.3")
	ip := netip.MustParseAddr("203.0.113.9")
	if got := SourceIP(relay, raw); got != ip {
		t.Fatalf("trusted previous hop: %v", got)
	}
	if got := SourceIP(relay, []byte("From: sender@example.test\r\n\r\nFixture")); got.IsValid() {
		t.Fatal("missing trusted hop fell back to relay IP")
	}
	if got := SourceIP(netip.MustParseAddr("192.0.2.8"), raw); got.String() != "192.0.2.8" {
		t.Fatal("untrusted header overrode peer")
	}
	calls := 0
	fake := func(ctx context.Context, network, key string) ([]netip.Addr, error) {
		calls++
		if network != "ip4" || key != "9.113.0.203.dnsbl.example." {
			t.Fatalf("lookup: %q %q", network, key)
		}
		if bounds, ok := ctx.Deadline(); !ok || time.Until(bounds) > 2*time.Second {
			t.Fatal("query deadline missing")
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.2"), netip.MustParseAddr("127.255.255.254")}, nil
	}
	for i := 0; i < 2; i++ {
		r := n.dns(context.Background(), ip, "dnsbl.example", fake)
		if r.listed || r.code != "dnsbl_resolver_refused" {
			t.Fatalf("error code counted as listing: %+v", r)
		}
	}
	if calls != 1 {
		t.Fatalf("cache missed: %d", calls)
	}
	if h := n.warnings["dnsbl.example"]; h.code != "dnsbl_resolver_refused" {
		t.Fatalf("missing warning %+v", h)
	}
	if r := n.dns(context.Background(), ip, "other.example", func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.2")}, nil
	}); !r.listed || r.code != "" {
		t.Fatalf("missed listing: %+v", r)
	}
	if r := n.dns(context.Background(), ip, "unavailable.example", func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, &net.DNSError{IsNotFound: true}
	}); r.listed || r.code != "" {
		t.Fatalf("NXDOMAIN: %+v", r)
	}
	if key := dnsKey(netip.MustParseAddr("2001:db8::a"), "dnsbl.example"); !strings.HasPrefix(key, "a.0.0.0.") || !strings.HasSuffix(key, "8.b.d.0.1.0.0.2.dnsbl.example.") {
		t.Fatalf("IPv6 DNSBL: %s", key)
	}
}

func TestClamdINSTREAMAndFailureDegrades(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	data := strings.Repeat("harmless fixture ", 4000)
	result := make(chan error, 1)
	go func() {
		c, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		r := bufio.NewReader(c)
		command, err := r.ReadString(0)
		if err != nil || command != "zINSTREAM\x00" {
			result <- errors.New("wrong clamd command")
			return
		}
		var body strings.Builder
		for {
			var chunk uint32
			if err = binary.Read(r, binary.BigEndian, &chunk); err != nil {
				result <- err
				return
			}
			if chunk == 0 {
				break
			}
			if chunk > 32<<10 {
				result <- errors.New("oversized clamd frame")
				return
			}
			if _, err = io.CopyN(&body, r, int64(chunk)); err != nil {
				result <- err
				return
			}
		}
		if body.String() != data {
			result <- errors.New("scan bytes changed")
			return
		}
		_, err = io.WriteString(c, "stream: Test.Fixture FOUND\x00")
		result <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	code, err := scanClamd(ctx, listener.Addr().String(), []byte(data))
	if err != nil || code != "clamav_Test.Fixture" {
		t.Fatalf("clamd %q %v", code, err)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	a := analyzeShared(ctx, &parsemail.Email{Subject: "OTP: 234567", Text: []byte("fixture")}, Envelope{}, nil, Settings{ClamdEnabled: true, ClamdAddr: addr})
	if a == nil || a.VerificationCode == nil || a.Risk.Level != "none" {
		t.Fatalf("scanner failure stopped analysis: %+v", a)
	}
	warnings := Warnings(Settings{ClamdEnabled: true})
	if len(warnings) != 1 || warnings[0].Code != "clamd_unavailable" {
		t.Fatalf("warnings: %+v", warnings)
	}
}
