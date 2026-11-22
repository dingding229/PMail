package domain

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

func spfTexts(records []string) []string {
	out := []string{}
	for _, value := range records {
		fields := strings.Fields(value)
		if len(fields) > 0 && strings.EqualFold(fields[0], "v=spf1") {
			out = append(out, strings.Join(fields, " "))
		}
	}
	return out
}

type spfResult uint8

const (
	spfNoPass spfResult = iota
	spfPass
	spfUnknown
)

type spfResolver interface {
	LookupTXT(context.Context, string) ([]string, error)
	LookupMX(context.Context, string) ([]*net.MX, error)
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type spfEvaluator struct {
	r         spfResolver
	remaining int
}

func (e *spfEvaluator) budget() bool { e.remaining--; return e.remaining >= 0 }
func missingDNS(err error) bool      { var e *net.DNSError; return errors.As(err, &e) && e.IsNotFound }
func (e *spfEvaluator) follow(ctx context.Context, domain string, addr netip.Addr) spfResult {
	name, err := spfDNSName(domain)
	if err != nil || !e.budget() {
		return spfUnknown
	}
	values, err := e.r.LookupTXT(ctx, fqdn(name))
	if err != nil {
		return spfUnknown
	}
	texts := spfTexts(values)
	if len(texts) != 1 {
		return spfUnknown
	}
	return e.check(ctx, name, texts[0], addr)
}
func (e *spfEvaluator) check(ctx context.Context, domain, value string, addr netip.Addr) spfResult {
	fields := strings.Fields(value)
	if len(fields) == 0 || !strings.EqualFold(fields[0], "v=spf1") {
		return spfUnknown
	}
	redirect := ""
	for _, term := range fields[1:] {
		if ctx.Err() != nil {
			return spfUnknown
		}
		qualifier := byte('+')
		if strings.ContainsRune("+-~?", rune(term[0])) {
			qualifier = term[0]
			term = term[1:]
			if term == "" {
				return spfUnknown
			}
		}
		if key, value, ok := strings.Cut(term, "="); ok {
			if strings.EqualFold(key, "redirect") {
				redirect = value
			}
			continue
		}
		name, arg, hasArg := strings.Cut(term, ":")
		mech, _, _ := strings.Cut(strings.ToLower(name), "/")
		result := spfNoPass
		switch mech {
		case "all":
			result = spfPass
		case "ip4", "ip6":
			if !hasArg {
				return spfUnknown
			}
			family4 := mech == "ip4"
			if strings.Contains(arg, "/") {
				prefix, err := netip.ParsePrefix(arg)
				if err != nil || prefix.Addr().Is4() != family4 {
					return spfUnknown
				}
				if prefix.Contains(addr) {
					result = spfPass
				}
			} else {
				expected, err := netip.ParseAddr(arg)
				if err != nil || expected.Is4() != family4 {
					return spfUnknown
				}
				if addr == expected {
					result = spfPass
				}
			}
		case "include":
			if !hasArg {
				return spfUnknown
			}
			result = e.follow(ctx, arg, addr)
		case "a", "mx":
			value := name[len(mech):]
			if hasArg {
				value = arg
			}
			target, bits, ok := spfTarget(value, domain, addr.Is4())
			if !ok || !e.budget() {
				return spfUnknown
			}
			if mech == "a" {
				result = e.addresses(ctx, target, bits, addr)
			} else {
				mx, err := e.r.LookupMX(ctx, target)
				if err != nil && !missingDNS(err) {
					return spfUnknown
				}
				if len(mx) > 10 {
					return spfUnknown
				}
				for _, item := range mx {
					result = e.addresses(ctx, item.Host, bits, addr)
					if result != spfNoPass {
						break
					}
				}
			}
		default:
			return spfUnknown // unsupported/macro-dependent mechanisms are never guessed
		}
		if result == spfUnknown {
			return result
		}
		if result == spfPass {
			if qualifier == '+' {
				return spfPass
			}
			return spfNoPass
		}
	}
	if redirect != "" {
		return e.follow(ctx, redirect, addr)
	}
	return spfNoPass
}

func spfDNSName(name string) (string, error) {
	// SPF include/redirect commonly point to underscore-prefixed DNS labels.
	name = strings.TrimSuffix(name, ".")
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 {
			return "", ErrInvalid
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return "", ErrInvalid
			}
		}
	}
	if len(name) > 253 {
		return "", ErrInvalid
	}
	return strings.ToLower(name), nil
}

func spfTarget(value, domain string, v4 bool) (string, int, bool) {
	v6Suffix := ""
	if head, tail, ok := strings.Cut(value, "//"); ok {
		value, v6Suffix = head, tail
	}
	target, v4Suffix, _ := strings.Cut(value, "/")
	if target == "" {
		target = domain
	}
	name, err := spfDNSName(target)
	if err != nil {
		return "", 0, false
	}
	suffix, bits := v6Suffix, 128
	if v4 {
		suffix, bits = v4Suffix, 32
	}
	if suffix != "" {
		n, err := strconv.Atoi(suffix)
		if err != nil || n < 0 || n > bits {
			return "", 0, false
		}
		bits = n
	}
	return fqdn(name), bits, true
}
func (e *spfEvaluator) addresses(ctx context.Context, host string, bits int, addr netip.Addr) spfResult {
	family := "ip6"
	if addr.Is4() {
		family = "ip4"
	}
	found, err := e.r.LookupNetIP(ctx, family, host)
	if err != nil && !missingDNS(err) {
		return spfUnknown
	}
	for _, value := range found {
		if netip.PrefixFrom(value.Unmap(), bits).Contains(addr) {
			return spfPass
		}
	}
	return spfNoPass
}
