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
	r            spfResolver
	remaining    int
	explicitOnly bool
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
			if e.explicitOnly {
				return spfNoPass
			}
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
					if item == nil || item.Host == "." {
						continue
					}
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
	// MX targets can be returned with arbitrary case and without a root dot.
	found, err := e.r.LookupNetIP(ctx, family, fqdn(host))
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

// suggestSPF keeps the base record and its terminal policy. Additional SPF
// records contribute their distinct mechanisms, not their competing policies.
func suggestSPF(records []string) string {
	if len(records) == 0 {
		return ""
	}
	base := strings.Fields(records[0])
	if len(base) == 0 {
		return ""
	}
	canonical := func(term string) string { return strings.ToLower(strings.TrimPrefix(term, "+")) }
	all := len(base)
	for i, term := range base[1:] {
		if spfAll(term) {
			all = i + 1
			break
		}
	}
	seen := map[string]bool{}
	for _, term := range base[1:all] {
		seen[canonical(term)] = true
	}
	merge := []string{}
	for _, record := range records[1:] {
		parts := strings.Fields(record)
		if len(parts) == 0 {
			continue
		}
		for _, term := range parts[1:] {
			key := canonical(term)
			if spfAll(term) {
				break
			}
			if strings.Contains(term, "=") || seen[key] {
				continue
			}
			merge = append(merge, term)
			seen[key] = true
		}
	}
	// A qualified positive mx/a is equivalent to its bare mechanism. Do not
	// remove or override existing explicit negative mechanisms.
	for _, term := range []string{"mx", "a"} {
		if !seen[term] {
			merge = append(merge, term)
		}
	}
	return strings.Join(append(append(base[:all:all], merge...), base[all:]...), " ")
}

func spfAll(term string) bool {
	if len(term) > 0 && strings.ContainsRune("+-~?", rune(term[0])) {
		term = term[1:]
	}
	return strings.EqualFold(term, "all")
}

// ReportIPAuthorized approximates ownership using SPF mechanisms that
// explicitly identify a source. An open +all policy is never evidence of ownership.
func ReportIPAuthorized(ctx context.Context, r spfResolver, name string, addr netip.Addr) bool {
	e := spfEvaluator{r: r, remaining: 10, explicitOnly: true}
	return e.follow(ctx, name, addr.Unmap()) == spfPass
}
