package inboundauth

import (
	"context"
	"net/netip"
	"strconv"
	"strings"

	"github.com/Jinnrry/pmail/models"
)

// Bound both SPF's DNS-interactive terms and their aggregate wire work. A
// cycle consumes terms just like any other invalid chain; no recursion can
// run independently of the request deadline.
type spfEvaluation struct {
	dns                   Resolver
	ip                    netip.Addr
	sender, helo          string
	terms, queries, voids int
}

func spfResult(result, reason string) models.MailAuthCheck { return check(result, reason, "") }

func (e *spfEvaluation) query() bool { e.queries++; return e.queries <= 40 && e.voids <= 2 }
func (e *spfEvaluation) term() bool  { e.terms++; return e.terms <= 10 }
func (e *spfEvaluation) dnsResult(err error, empty bool) models.MailAuthCheck {
	if err != nil && !missing(err) {
		return spfResult("temperror", "dns_unavailable")
	}
	if empty || err != nil {
		e.voids++
		if e.voids > 2 {
			return spfResult("permerror", "void_lookup_limit")
		}
	}
	return models.MailAuthCheck{}
}

func spfName(raw string) string {
	raw = strings.ToLower(strings.TrimSuffix(raw, "."))
	if len(raw) > 253 || !strings.Contains(raw, ".") {
		return ""
	}
	for _, label := range strings.Split(raw, ".") {
		if len(label) == 0 || len(label) > 63 {
			return ""
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				return ""
			}
		}
	}
	return raw + "."
}

func (e *spfEvaluation) evaluate(ctx context.Context, domain string) models.MailAuthCheck {
	domain = strings.TrimSuffix(domain, ".")
	if ctx.Err() != nil {
		return spfResult("temperror", "dns_unavailable")
	}
	if !e.query() {
		return spfResult("permerror", "lookup_limit")
	}
	txt, err := e.dns.LookupTXT(ctx, domain+".")
	if result := e.dnsResult(err, len(txt) == 0); result.Result != "" {
		return result
	}
	var fields []string
	for _, v := range txt {
		parts := strings.Fields(v)
		if len(parts) == 0 || !strings.EqualFold(parts[0], "v=spf1") {
			continue
		}
		if fields != nil {
			return spfResult("permerror", "multiple_records")
		}
		fields = parts
	}
	if fields == nil {
		return spfResult("none", "no_record")
	}
	if len(fields) > 128 {
		return spfResult("permerror", "record_too_complex")
	}
	redirect := ""
	seenRedirect := false
	var mechanisms []string
	for _, word := range fields[1:] {
		if key, val, ok := strings.Cut(word, "="); ok && !strings.ContainsAny(key, ":/%") {
			if strings.EqualFold(key, "redirect") {
				if seenRedirect || val == "" {
					return spfResult("permerror", "invalid_redirect")
				}
				seenRedirect, redirect = true, val
			}
			continue // exp and unknown modifiers cannot change matching
		}
		mechanisms = append(mechanisms, word)
	}
	for _, word := range mechanisms {
		q := '+'
		if strings.ContainsRune("+-~?", rune(word[0])) {
			q, word = rune(word[0]), word[1:]
		}
		index := strings.IndexAny(word, ":/")
		mechanism, arg := word, ""
		if index >= 0 {
			mechanism, arg = word[:index], word[index:]
		}
		mechanism = strings.ToLower(mechanism)
		ok, bad := e.match(ctx, domain, mechanism, arg)
		if bad.Result != "" {
			return bad
		}
		if ok {
			value := map[rune]string{'+': "pass", '-': "fail", '~': "softfail", '?': "neutral"}[q]
			return spfResult(value, "matched_"+mechanism)
		}
	}
	if seenRedirect {
		if !e.term() {
			return spfResult("permerror", "lookup_limit")
		}
		name := e.expand(redirect, domain)
		if name == "" {
			return spfResult("permerror", "invalid_redirect")
		}
		result := e.evaluate(ctx, name)
		if result.Result == "none" {
			return spfResult("permerror", "redirect_no_record")
		}
		return result
	}
	return spfResult("neutral", "no_match")
}

func (e *spfEvaluation) match(ctx context.Context, domain, mechanism, arg string) (bool, models.MailAuthCheck) {
	bad := spfResult("permerror", "invalid_mechanism")
	limit := spfResult("permerror", "lookup_limit")
	var no models.MailAuthCheck
	switch mechanism {
	case "all":
		if arg != "" {
			return false, bad
		}
		return true, no
	case "ip4", "ip6":
		raw, ok := strings.CutPrefix(arg, ":")
		if !ok {
			return false, bad
		}
		p, err := netip.ParsePrefix(raw)
		if !strings.Contains(raw, "/") {
			a, parseErr := netip.ParseAddr(raw)
			err = parseErr
			if parseErr == nil {
				p = netip.PrefixFrom(a, a.BitLen())
			}
		}
		if err != nil || p.Addr().Is4In6() || (mechanism == "ip4") != p.Addr().Is4() {
			return false, bad
		}
		return p.Contains(e.ip), no
	case "include", "exists", "ptr", "a", "mx":
		if !e.term() {
			return false, limit
		}
	default:
		return false, bad
	}
	target, bits, ok := e.target(domain, mechanism, arg)
	if !ok {
		return false, bad
	}
	switch mechanism {
	case "include":
		i := e.evaluate(ctx, target)
		switch i.Result {
		case "pass":
			return true, no
		case "temperror", "permerror":
			return false, i
		case "none":
			return false, spfResult("permerror", "include_no_record")
		}
		return false, no
	case "exists":
		if !e.query() {
			return false, limit
		}
		addrs, err := e.dns.LookupNetIP(ctx, "ip4", target)
		return len(addrs) > 0, e.dnsResult(err, len(addrs) == 0)
	case "ptr":
		if !e.query() {
			return false, limit
		}
		names, err := e.dns.LookupAddr(ctx, e.ip.String())
		if res := e.dnsResult(err, len(names) == 0); res.Result != "" {
			return false, res
		}
		for _, name := range names[:min(10, len(names))] {
			name = spfName(name)
			if name == "" || (name != target && !strings.HasSuffix(name, "."+target)) {
				continue
			}
			if yes, result := e.matchAddress(ctx, name, e.ip.BitLen()); yes || result.Result != "" {
				return yes, result
			}
		}
		return false, no
	case "a":
		return e.matchAddress(ctx, target, bits)
	case "mx":
		if !e.query() {
			return false, limit
		}
		mx, err := e.dns.LookupMX(ctx, target)
		if res := e.dnsResult(err, len(mx) == 0); res.Result != "" {
			return false, res
		}
		if len(mx) > 10 {
			return false, limit
		}
		for _, m := range mx {
			if m == nil {
				continue
			}
			if name := spfName(m.Host); name != "" {
				if yes, res := e.matchAddress(ctx, name, bits); yes || res.Result != "" {
					return yes, res
				}
			}
		}
	}
	return false, no
}

func (e *spfEvaluation) target(domain, mechanism, arg string) (string, int, bool) {
	raw4, raw6 := "", ""
	if mechanism == "a" || mechanism == "mx" {
		// Find CIDR separators only outside %{...} macro expressions.
		i, inside := -1, false
		for pos, c := range arg {
			switch c {
			case '{':
				inside = true
			case '}':
				inside = false
			case '/':
				if !inside && i < 0 {
					i = pos
				}
			}
		}
		if i >= 0 {
			suffix := arg[i:]
			arg = arg[:i]
			if v4, v6, ok := strings.Cut(suffix, "//"); ok {
				raw4 = strings.TrimPrefix(v4, "/")
				raw6 = v6
				if raw6 == "" {
					return "", 0, false
				}
			} else {
				raw4 = suffix[1:]
				if raw4 == "" {
					return "", 0, false
				}
			}
		}
	}
	target := spfName(domain)
	if arg != "" {
		raw, ok := strings.CutPrefix(arg, ":")
		if !ok {
			return "", 0, false
		}
		target = e.expand(raw, domain)
	} else if mechanism == "exists" || mechanism == "include" {
		return "", 0, false
	}
	bits := e.ip.BitLen()
	for i, raw := range []string{raw4, raw6} {
		if raw == "" {
			continue
		}
		v, err := strconv.Atoi(raw)
		maxBits := 32
		if i == 1 {
			maxBits = 128
		}
		if err != nil || v < 0 || v > maxBits {
			return "", 0, false
		}
		if bits == maxBits {
			bits = v
		}
	}
	return target, bits, target != ""
}

func (e *spfEvaluation) matchAddress(ctx context.Context, target string, bits int) (bool, models.MailAuthCheck) {
	if !e.query() {
		return false, spfResult("permerror", "lookup_limit")
	}
	family := "ip6"
	if e.ip.Is4() {
		family = "ip4"
	}
	ips, err := e.dns.LookupNetIP(ctx, family, target)
	if res := e.dnsResult(err, len(ips) == 0); res.Result != "" {
		return false, res
	}
	for _, ip := range ips {
		if netip.PrefixFrom(ip.Unmap(), bits).Contains(e.ip) {
			return true, models.MailAuthCheck{}
		}
	}
	return false, models.MailAuthCheck{}
}

func (e *spfEvaluation) expand(raw, domain string) string {
	var dst strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] != '%' {
			dst.WriteByte(raw[i])
			continue
		}
		i++
		if i == len(raw) {
			return ""
		}
		switch raw[i] {
		case '%':
			dst.WriteByte('%')
		case '_', '-':
			return "" // spaces are not valid in a DNS domain-spec
		case '{':
			end := strings.IndexByte(raw[i+1:], '}')
			if end < 1 {
				return ""
			}
			parts, ok := e.macro(raw[i+1:i+1+end], domain)
			if !ok {
				return ""
			}
			dst.WriteString(parts)
			i += end + 1
		default:
			return ""
		}
		if dst.Len() > 1024 {
			return ""
		}
	}
	return spfName(dst.String())
}

func (e *spfEvaluation) macro(expr, domain string) (string, bool) {
	local, _, _ := strings.Cut(e.sender, "@")
	senderDomain := domainOf(e.sender)
	var value string
	switch expr[0] {
	case 's':
		value = e.sender
	case 'l':
		value = local
	case 'o':
		value = senderDomain
	case 'd':
		value = domain
	case 'h':
		value = e.helo
	case 'v':
		value = "in-addr"
		if e.ip.Is6() {
			value = "ip6"
		}
	case 'i':
		value = e.ip.String()
		if e.ip.Is6() {
			var b strings.Builder
			const hex = "0123456789abcdef"
			for _, x := range e.ip.As16() {
				b.WriteByte(hex[x>>4])
				b.WriteByte('.')
				b.WriteByte(hex[x&15])
				b.WriteByte('.')
			}
			value = strings.TrimSuffix(b.String(), ".")
		}
	case 'p':
		return "", false // Do not guess a validated domain without a bounded SPF implementation for this macro.
	default:
		return "", false
	}
	modifiers := expr[1:]
	end := 0
	for end < len(modifiers) && modifiers[end] >= '0' && modifiers[end] <= '9' {
		end++
	}
	count := 0
	if end > 0 {
		var err error
		count, err = strconv.Atoi(modifiers[:end])
		if err != nil || count <= 0 {
			return "", false
		}
	}
	modifiers = modifiers[end:]
	reverse := strings.HasPrefix(modifiers, "r")
	modifiers = strings.TrimPrefix(modifiers, "r")
	if modifiers == "" {
		modifiers = "."
	}
	for _, c := range modifiers {
		if !strings.ContainsRune(".-+,/_=", c) {
			return "", false
		}
	}
	items := strings.FieldsFunc(value, func(c rune) bool { return strings.ContainsRune(modifiers, c) })
	if reverse {
		for l, r := 0, len(items)-1; l < r; l, r = l+1, r-1 {
			items[l], items[r] = items[r], items[l]
		}
	}
	if count > 0 && count < len(items) {
		items = items[len(items)-count:]
	}
	return strings.Join(items, "."), true
}
