package send

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/textproto"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/utils/smtp"
	log "github.com/sirupsen/logrus"
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

func TestOutboundOpportunisticTLSPolicy(t *testing.T) {
	cert := selfSignedMXCertificate(t)
	const recipientDomain = "recipient.example.net"
	const downgrade = "audit_event=smtp_tls_downgrade"
	const cleartext = "audit_event=smtp_cleartext_fallback"
	for _, tc := range []struct {
		name, mxMode       string
		verified, strict   bool
		want               []string
		wantError          string
		wantDowngrade      bool
		wantCleartextCause string
	}{
		{name: "self-signed MX accepts encrypted mail by default", mxMode: "self-signed", want: []string{"handshake failed", "encrypted DATA"}, wantDowngrade: true},
		{name: "verified mode rejects self-signed MX", mxMode: "self-signed", verified: true, want: []string{"handshake failed"}, wantError: "failed to verify certificate"},
		{name: "strict domain rejects self-signed MX by default", mxMode: "self-signed", strict: true, want: []string{"handshake failed"}, wantError: "failed to verify certificate"},
		{name: "no STARTTLS defaults to cleartext", want: []string{"no STARTTLS", "cleartext DATA"}, wantCleartextCause: "starttls_not_advertised"},
		{name: "verified mode permits MX without STARTTLS", verified: true, want: []string{"no STARTTLS", "cleartext DATA"}, wantCleartextCause: "starttls_not_advertised"},
		{name: "strict domain requires STARTTLS", strict: true, want: []string{"no STARTTLS"}, wantError: "doesn't support STARTTLS"},
		{name: "failed opportunistic negotiation permits cleartext", mxMode: "reject", want: []string{"STARTTLS rejected", "STARTTLS rejected", "cleartext DATA"}, wantDowngrade: true, wantCleartextCause: "opportunistic_starttls_failed"},
		{name: "verified mode rejects advertised but unavailable TLS", mxMode: "reject", verified: true, want: []string{"STARTTLS rejected"}, wantError: "454"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flag, domains := "", ""
			if tc.verified {
				flag = "1"
			}
			if tc.strict {
				domains = recipientDomain
			}
			t.Setenv("PMAIL_SMTP_REQUIRE_VERIFIED_TLS", flag)
			t.Setenv("PMAIL_SMTP_TLS_REQUIRED_DOMAINS", domains)
			t.Setenv("PMAIL_SMTP_OUTBOUND_ALLOW", "")
			policy, err := outboundPolicy()
			if err != nil || policy.RequireVerifiedTLS != tc.verified {
				t.Fatalf("outboundPolicy() = %+v, %v", policy, err)
			}
			logger := log.StandardLogger()
			var audit bytes.Buffer
			oldOutput, oldLevel := logger.Out, logger.GetLevel()
			logger.SetOutput(&audit)
			logger.SetLevel(log.InfoLevel)
			t.Cleanup(func() { logger.SetOutput(oldOutput); logger.SetLevel(oldLevel) })

			policy.Resolver = &pinnedFixture{}
			var attempts []<-chan string
			policy.Dial = func(_ context.Context, _, _ string) (net.Conn, error) {
				server, client := net.Pipe()
				result := make(chan string, 1)
				attempts = append(attempts, result)
				go func() { result <- serveTLSMX(server, tc.mxMode, cert) }()
				return client, nil
			}
			err = policy.deliver(context.Background(), recipientDomain, "sender@example.test", "example.test", []string{"recipient@" + recipientDomain}, []byte("Subject: fixture\r\n\r\nfixture\r\n"))
			if tc.wantError == "" && err != nil || tc.wantError != "" && (!errors.Is(err, smtp.ErrTLSNegotiation) || !strings.Contains(err.Error(), tc.wantError)) {
				t.Errorf("deliver error = %v, want TLS error containing %q (empty means success)", err, tc.wantError)
			}
			var got []string
			for _, result := range attempts {
				got = append(got, <-result)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("SMTP attempts = %v, want %v", got, tc.want)
			}
			logs := audit.String()
			d, c := strings.Index(logs, downgrade), strings.Index(logs, cleartext)
			if (d >= 0) != tc.wantDowngrade || (c >= 0) != (tc.wantCleartextCause != "") ||
				c >= 0 && (!strings.Contains(logs[c:], "reason="+tc.wantCleartextCause) || d > c) {
				t.Errorf("unexpected audit entries: %s", logs)
			}
		})
	}
}

func selfSignedMXCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		DNSNames: []string{"mx.example.net"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func serveTLSMX(conn net.Conn, mode string, cert tls.Certificate) string {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	wire := textproto.NewConn(conn)
	if err := wire.PrintfLine("220 fixture"); err != nil {
		return err.Error()
	}
	var encrypted bool
	for {
		line, err := wire.ReadLine()
		if err != nil {
			if mode == "" {
				return "no STARTTLS"
			}
			return err.Error()
		}
		switch {
		case strings.HasPrefix(line, "EHLO "):
			if mode != "" && !encrypted {
				err = wire.PrintfLine("250-fixture\r\n250 STARTTLS")
			} else {
				err = wire.PrintfLine("250 fixture")
			}
		case line == "STARTTLS" && mode == "reject":
			if err = wire.PrintfLine("454 fixture failed"); err != nil {
				return err.Error()
			}
			return "STARTTLS rejected"
		case line == "STARTTLS" && mode == "self-signed":
			if err = wire.PrintfLine("220 ready"); err != nil {
				return err.Error()
			}
			secure := tls.Server(conn, &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})
			if err = secure.Handshake(); err != nil {
				return "handshake failed"
			}
			wire, encrypted = textproto.NewConn(secure), true
		case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
			err = wire.PrintfLine("250 OK")
		case line == "DATA":
			if err = wire.PrintfLine("354 send message"); err != nil {
				return err.Error()
			}
			body, bodyErr := wire.ReadDotBytes()
			if bodyErr != nil || string(body) != "Subject: fixture\n\nfixture\n" {
				return fmt.Sprintf("unexpected DATA %q: %v", body, bodyErr)
			}
			if err = wire.PrintfLine("250 accepted"); err != nil {
				return err.Error()
			}
			_, _ = wire.ReadLine() // QUIT is best-effort after accepting DATA.
			if encrypted {
				return "encrypted DATA"
			}
			return "cleartext DATA"
		default:
			return "unexpected command: " + line
		}
		if err != nil {
			return err.Error()
		}
	}
}
