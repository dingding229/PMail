package ssl

import (
	"context"
	"crypto/x509"
	"errors"
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
	r, err := domain.Resolver()
	if err != nil {
		return nil, err
	}
	return r.LookupNetIP(ctx, "ip", host)
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

// requestDomains picks the names to request. With HTTP-01 every name in one
// order must pass validation, so a single mail host that is missing or
// proxied by a CDN used to block the whole certificate and leave SMTP, IMAP
// and POP3 without TLS. Mail hosts whose A/AAAA records do not include this
// server's public address are skipped (and retried on the next check).
// The web domain is always kept: it is commonly behind a reverse proxy or
// CDN that still forwards the challenge path. DNS-01 keeps every name.
func requestDomains(ctx context.Context, cfg *config.Config) ([]string, []skippedName) {
	all := certificateDomains(cfg)
	if cfg.SSLType != config.SSLTypeAutoHTTP {
		return all, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	v4, v6 := serverAddresses(ctx)
	if !v4.IsValid() && !v6.IsValid() {
		return all, nil // unknown public address: keep the previous behaviour
	}
	keep := make([]string, 0, len(all))
	var skipped []skippedName
	for _, name := range all {
		if name == cfg.WebDomain {
			keep = append(keep, name)
			continue
		}
		found, err := hostLookup(ctx, name)
		actual := make([]string, 0, len(found))
		match := false
		for _, addr := range found {
			addr = addr.Unmap()
			actual = append(actual, addr.String())
			match = match || addr == v4 || addr == v6
		}
		if match {
			keep = append(keep, name)
			continue
		}
		var dnsErr *net.DNSError
		if err != nil && len(actual) == 0 && (!errors.As(err, &dnsErr) || !dnsErr.IsNotFound) {
			// Transient lookup failure: do not drop the name on a resolver hiccup.
			keep = append(keep, name)
			continue
		}
		skipped = append(skipped, skippedName{Name: name, Actual: actual})
	}
	return keep, skipped
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

// coversRequested reports whether cert covers every name that can currently be requested.
func coversRequested(ctx context.Context, cfg *config.Config, cert *x509.Certificate) bool {
	names, _ := requestDomains(ctx, cfg)
	return !slices.ContainsFunc(names, func(name string) bool { return cert.VerifyHostname(name) != nil })
}
