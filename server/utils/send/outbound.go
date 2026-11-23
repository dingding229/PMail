package send

import (
	stdctx "context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/textproto"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/utils/smtp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/idna"
)

type MXResolver interface {
	LookupMX(stdctx.Context, string) ([]*net.MX, error)
	LookupNetIP(stdctx.Context, string, string) ([]netip.Addr, error)
}

type Outbound struct {
	Resolver MXResolver
	Dial     func(stdctx.Context, string, string) (net.Conn, error)
	// RequireVerifiedTLS forbids downgrade when an MX advertises STARTTLS.
	RequireVerifiedTLS bool
	// TLSRequired contains DNS names (matches exact domains, not suffixes).
	TLSRequired  map[string]bool
	AllowedHosts map[string]bool
	AllowedCIDRs []netip.Prefix
}

func hostKey(value string) string {
	raw := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if a, err := netip.ParseAddr(raw); err == nil && a.Zone() == "" {
		return a.Unmap().String()
	}
	host, err := idna.Lookup.ToASCII(raw)
	if err != nil || host == "" || len(host) > 253 {
		return ""
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return ""
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return ""
			}
		}
	}
	return host
}

func outboundPolicy() (Outbound, error) {
	d := &net.Dialer{Timeout: 5 * time.Second}
	p := Outbound{RequireVerifiedTLS: config.Flag("PMAIL_SMTP_REQUIRE_VERIFIED_TLS"), Resolver: net.DefaultResolver, Dial: d.DialContext, TLSRequired: map[string]bool{}, AllowedHosts: map[string]bool{}}
	for _, item := range strings.Split(os.Getenv("PMAIL_SMTP_TLS_REQUIRED_DOMAINS"), ",") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		h := hostKey(item)
		if h == "" {
			return p, fmt.Errorf("invalid TLS-required domain %q", item)
		}
		p.TLSRequired[h] = true
	}
	for _, item := range strings.Split(os.Getenv("PMAIL_SMTP_OUTBOUND_ALLOW"), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			prefixes, err := config.CIDRs(item)
			if err != nil {
				return p, err
			}
			p.AllowedCIDRs = append(p.AllowedCIDRs, prefixes...)
			continue
		}
		h := hostKey(item)
		if h == "" {
			return p, fmt.Errorf("invalid SMTP allow host %q", item)
		}
		p.AllowedHosts[h] = true
	}
	return p, nil
}

var outboundReserved = prefixes("0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20")
var ipv6Public = netip.MustParsePrefix("2000::/3")

func prefixes(values ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(values))
	for _, v := range values {
		out = append(out, netip.MustParsePrefix(v))
	}
	return out
}

func (p Outbound) allowed(host string, a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.Zone() != "" {
		return false
	}
	if !a.IsGlobalUnicast() { // loopback may be explicitly allowed, never multicast/unspecified
		if !a.IsLoopback() && !a.IsLinkLocalUnicast() {
			return false
		}
	}
	if p.AllowedHosts[host] {
		return true
	}
	for _, subnet := range p.AllowedCIDRs {
		if subnet.Contains(a) {
			return true
		}
	}
	if !a.IsGlobalUnicast() || a.IsPrivate() || (!a.Is4() && !ipv6Public.Contains(a)) {
		return false
	}
	for _, subnet := range outboundReserved {
		if subnet.Contains(a) {
			return false
		}
	}
	return true
}

func (p Outbound) targets(ctx stdctx.Context, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if p.allowed(host, addr) {
			return []netip.Addr{addr.Unmap()}, nil
		}
		return nil, errBlockedTarget
	}
	addrs, err := p.Resolver.LookupNetIP(ctx, "ip", host+".")
	if err != nil {
		return nil, err
	}
	result := make([]netip.Addr, 0, min(8, len(addrs)))
	seen := map[netip.Addr]bool{}
	for _, a := range addrs {
		a = a.Unmap()
		if !seen[a] && p.allowed(host, a) {
			seen[a] = true
			result = append(result, a)
			if len(result) >= 8 {
				break
			}
		}
	}
	if len(result) == 0 {
		return nil, errBlockedTarget
	}
	return result, nil
}

var errBlockedTarget = errors.New("SMTP target is not allowed by outbound address policy")

func (p Outbound) deliver(ctx stdctx.Context, recipientDomain, from, fromDomain string, to []string, raw []byte) error {
	ctx, cancel := stdctx.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	mx, err := p.Resolver.LookupMX(ctx, recipientDomain+".")
	var ns *net.DNSError
	if err != nil && !(errors.As(err, &ns) && ns.IsNotFound) {
		return err
	}
	// RFC implicit MX for a domain without MX RRs; a null MX declines delivery.
	if len(mx) == 0 {
		mx = []*net.MX{{Host: recipientDomain}}
	}
	mx = append([]*net.MX(nil), mx...)
	sort.SliceStable(mx, func(i, j int) bool {
		if mx[i] == nil {
			return false
		}
		return mx[j] == nil || mx[i].Pref < mx[j].Pref
	})
	var last error
	for _, record := range mx[:min(10, len(mx))] {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if record == nil {
			continue
		}
		if record.Host == "." && len(mx) == 1 {
			return &textproto.Error{Code: 556, Msg: "recipient domain does not accept mail"}
		}
		host := hostKey(record.Host)
		if host == "" {
			continue
		}
		ips, err := p.targets(ctx, host)
		if err != nil {
			last = err
			continue
		}
		for _, ip := range ips {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// IP was resolved and checked exactly once. Each transport uses this
			// numeric dial address; the original MX hostname is the TLS name.
			try := func(port string, mode smtp.TransportMode) error {
				path, cancel := stdctx.WithTimeout(ctx, 20*time.Second)
				defer cancel()
				conn, err := p.Dial(path, "tcp", net.JoinHostPort(ip.String(), port))
				if err != nil {
					return err
				}
				return smtp.SendPinned(path, conn, host, from, fromDomain, to, raw, mode)
			}
			last = try("25", smtp.VerifySTARTTLS)
			if last == nil {
				log.WithContext(ctx).Infof("audit_event=smtp_tls_verified recipient_domain=%q mx=%q ip=%s", recipientDomain, host, ip)
				return nil
			}
			if isPermanentSMTPResponse(last) {
				return last
			}
			if !errors.Is(last, smtp.ErrTLSNegotiation) {
				continue
			}
			if p.TLSRequired[recipientDomain] {
				continue
			}
			reason := "starttls_not_advertised"
			if !errors.Is(last, smtp.ErrSTARTTLSUnsupported) {
				// The global switch enforces authentication when TLS is offered.
				// The domain policy above also requires TLS to be offered at all.
				if p.RequireVerifiedTLS {
					continue
				}
				log.WithContext(ctx).Warnf("audit_event=smtp_tls_downgrade recipient_domain=%q mx=%q ip=%s", recipientDomain, host, ip)
				last = try("25", smtp.OpportunisticSTARTTLS)
				if last == nil || isPermanentSMTPResponse(last) {
					return last
				}
				if !errors.Is(last, smtp.ErrTLSNegotiation) {
					continue
				}
				reason = "opportunistic_starttls_failed"
			}
			log.WithContext(ctx).Warnf("audit_event=smtp_cleartext_fallback recipient_domain=%q mx=%q ip=%s reason=%s", recipientDomain, host, ip, reason)
			last = try("25", smtp.CleartextFallback)
			if last == nil || isPermanentSMTPResponse(last) {
				return last
			}
		}
	}
	if last == nil {
		last = errBlockedTarget
	}
	return last
}
