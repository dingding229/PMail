package ssl

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/ip"
	log "github.com/sirupsen/logrus"
)

// hostLookup resolves a host name to addresses; replaced in tests.
var hostLookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
	r, err := acmeResolver()
	if err != nil {
		return nil, err
	}
	return r.LookupNetIP(ctx, "ip", strings.TrimSuffix(host, ".")+".")
}

func acmeResolver() (*net.Resolver, error) {
	r, err := domain.Resolver()
	if err != nil {
		return nil, err
	}
	// A temporary error on either A or AAAA must not be mistaken for a
	// complete result, even if the other address family resolves correctly.
	return &net.Resolver{PreferGo: true, StrictErrors: true, Dial: r.Dial}, nil
}

// serverAddresses returns this server's public addresses; replaced in tests.
var serverAddresses = func(ctx context.Context) (netip.Addr, netip.Addr) {
	return ip.PublicAddresses(ctx, os.Getenv("PMAIL_PUBLIC_IPV4"), os.Getenv("PMAIL_PUBLIC_IPV6"))
}

// skippedName is a mail host left out of an HTTP-01 request because its DNS
// does not point at this server (for example a CDN-proxied record).
type skippedName struct {
	Name   string
	Actual []string
}

// requestDomains distinguishes confirmed HTTP-01 targets from DNS/address
// discovery failures. Neither a stale AAAA nor one good address in a mixed
// set is enough: a validator can reach any published address.
func requestDomains(ctx context.Context, cfg *config.Config) (keep []string, skipped []skippedName, uncertain []string) {
	all := certificateDomains(cfg)
	if cfg.SSLType != config.SSLTypeAutoHTTP {
		return all, nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	v4, v6 := serverAddresses(ctx)
	for _, name := range all {
		if cfg.HttpsEnabled != 2 && name == cfg.WebDomain {
			keep = append(keep, name) // A proxy/CDN may relay this challenge.
			continue
		}
		found, err := hostLookup(ctx, name)
		var dnsErr *net.DNSError
		if err != nil && (len(found) > 0 || !errors.As(err, &dnsErr) || !dnsErr.IsNotFound) {
			uncertain = append(uncertain, name)
			continue
		}
		actual := make([]string, 0, len(found))
		mismatch, unknown := len(found) == 0, false
		for _, addr := range found {
			addr = addr.Unmap()
			actual = append(actual, addr.String())
			expected := v6
			if addr.Is4() {
				expected = v4
			}
			if !expected.IsValid() {
				unknown = true
			} else if addr != expected {
				mismatch = true
			}
		}
		switch {
		case mismatch:
			skipped = append(skipped, skippedName{Name: name, Actual: actual})
		case unknown:
			uncertain = append(uncertain, name)
		default:
			keep = append(keep, name)
		}
	}
	return keep, skipped, uncertain
}

func logSkipped(skipped []skippedName) {
	if len(skipped) == 0 {
		return
	}
	parts := make([]string, 0, len(skipped))
	for _, item := range skipped {
		target := "no A/AAAA record"
		if len(item.Actual) > 0 {
			target = strings.Join(item.Actual, " ")
		}
		parts = append(parts, item.Name+" -> "+target)
	}
	log.Warnf("TLS certificate: skipping mail hosts whose DNS does not point to this server (set them to DNS-only / grey cloud if a CDN proxies them; they are retried automatically): %s", strings.Join(parts, "; "))
}

// renewalDomains returns one snapshot for both the renewal decision and the
// ACME order. A healthy certificate may expand, but never trade one configured
// SAN for another, which could otherwise cause repeated reissuance on reload.
func renewalDomains(ctx context.Context, cfg *config.Config, cert *x509.Certificate) ([]string, error) {
	expiring := cert == nil || time.Until(cert.NotAfter) < 30*24*time.Hour
	covers := func(name string) bool { return cert != nil && cert.VerifyHostname(name) == nil }
	all := certificateDomains(cfg)
	if !expiring && !slices.ContainsFunc(all, func(name string) bool { return !covers(name) }) {
		return nil, nil // No missing SANs and not close to expiry: no DNS work.
	}
	names, skipped, uncertain := requestDomains(ctx, cfg)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !expiring {
		if !slices.ContainsFunc(names, func(name string) bool { return !covers(name) }) {
			return nil, nil
		}
		if slices.ContainsFunc(all, func(name string) bool { return covers(name) && !slices.Contains(names, name) }) {
			return nil, nil
		}
	} else if slices.ContainsFunc(uncertain, covers) {
		return nil, fmt.Errorf("could not verify DNS/public addresses for TLS names %s; deferring renewal", strings.Join(uncertain, ", "))
	}
	logSkipped(skipped)
	if len(names) == 0 {
		return nil, fmt.Errorf("no certificate name confirmed for HTTP-01 yet; check public addresses and DNS-only mail records (unconfirmed: %s)", strings.Join(uncertain, ", "))
	}
	return names, nil
}
