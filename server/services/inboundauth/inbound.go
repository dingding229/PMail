// Package inboundauth evaluates transport authentication without trusting
// authentication headers supplied by the sender.
package inboundauth

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/mail"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/models"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-msgauth/dmarc"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

type Resolver interface {
	LookupTXT(context.Context, string) ([]string, error)
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
	LookupMX(context.Context, string) ([]*net.MX, error)
	LookupAddr(context.Context, string) ([]string, error)
}

type Engine struct {
	DNS        Resolver
	VerifyDKIM func(context.Context, []byte, Resolver) ([]*dkim.Verification, error)
	Relays     []netip.Prefix
}

func Verify(ctx context.Context, peer netip.Addr, envelope, hello string, raw []byte) models.MailAuthResults {
	relays, _ := config.CIDRs(os.Getenv("PMAIL_TRUSTED_RELAYS"))
	return (Engine{DNS: net.DefaultResolver, Relays: relays}).Verify(ctx, peer, envelope, hello, raw)
}

func check(result, reason, domain string) models.MailAuthCheck {
	return models.MailAuthCheck{Result: result, Reason: reason, Domain: domain}
}

func normalize(raw string) string {
	raw = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	a, err := idna.Lookup.ToASCII(raw)
	if err != nil || a == "" || len(a) > 253 || !strings.Contains(a, ".") {
		return ""
	}
	if ip, _ := netip.ParseAddr(a); ip.IsValid() {
		return ""
	}
	return a
}

func domainOf(address string) string {
	if i := strings.LastIndexByte(address, '@'); i > 0 {
		return normalize(address[i+1:])
	}
	return ""
}

func org(domain string) string {
	if x, err := publicsuffix.EffectiveTLDPlusOne(domain); err == nil {
		return x
	}
	return domain
}

func aligned(candidate, from string, strict bool) bool {
	candidate = normalize(candidate)
	if candidate == "" || from == "" {
		return false
	}
	return candidate == from || (!strict && org(candidate) == org(from))
}

func verifyDKIM(ctx context.Context, raw []byte, dns Resolver) ([]*dkim.Verification, error) {
	return dkim.VerifyWithOptions(bytes.NewReader(raw), &dkim.VerifyOptions{MaxVerifications: 8, LookupTXT: func(name string) ([]string, error) { return dns.LookupTXT(ctx, strings.TrimSuffix(name, ".")+".") }})
}

func (e Engine) Verify(ctx context.Context, peer netip.Addr, envelope, hello string, raw []byte) models.MailAuthResults {
	if e.DNS == nil {
		e.DNS = net.DefaultResolver
	}
	if e.VerifyDKIM == nil {
		e.VerifyDKIM = verifyDKIM
	}
	var out models.MailAuthResults
	peer = peer.Unmap()
	for _, p := range e.Relays {
		if p.Contains(peer) {
			out.TrustedRelay = true
			break
		}
	}
	from := ""
	if m, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil && len(m.Header["From"]) == 1 {
		if people, err := m.Header.AddressList("From"); err == nil && len(people) == 1 {
			from = domainOf(people[0].Address)
		}
	}
	envelope = strings.TrimSpace(envelope)
	spfDomain := domainOf(envelope)
	identity := "mail_from"
	if envelope == "" || envelope == "<>" {
		spfDomain = normalize(hello)
		envelope = "postmaster@" + spfDomain
		identity = "helo"
	}
	if !peer.IsValid() || spfDomain == "" {
		out.SPF = check("none", "no_valid_"+identity, spfDomain)
	} else {
		ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		out.SPF = (&spfEvaluation{dns: e.DNS, ip: peer, sender: envelope, helo: hello}).evaluate(ctx, spfDomain)
		cancel()
		out.SPF.Domain = spfDomain
		out.SPF.Reason = identity + "_" + out.SPF.Reason
	}
	dmarcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	record, policyDomain, dmarcCheck := e.dmarc(dmarcCtx, from)
	cancel()
	strictSPF := record != nil && record.SPFAlignment == dmarc.AlignmentStrict
	strictDKIM := record != nil && record.DKIMAlignment == dmarc.AlignmentStrict
	out.SPF.Aligned = out.SPF.Result == "pass" && aligned(spfDomain, from, strictSPF)

	dkCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	verifications, err := e.VerifyDKIM(dkCtx, raw, e.DNS)
	cancel()
	out.DKIM = check("none", "no_signature", "")
	if err != nil && len(verifications) == 0 {
		out.DKIM = check("permerror", "signature_evaluation_error", "")
	}
	for _, v := range verifications {
		if v == nil {
			continue
		}
		d := normalize(v.Domain)
		if v.Err == nil {
			if aligned(d, from, strictDKIM) {
				out.DKIM = check("pass", "aligned_signature", d)
				out.DKIM.Aligned = true
				break
			}
			out.DKIM = check("pass", "unaligned_signature", d)
			continue
		}
		if out.DKIM.Result == "pass" {
			continue
		}
		if dkim.IsTempFail(v.Err) || errors.Is(v.Err, context.DeadlineExceeded) {
			out.DKIM = check("temperror", "signature_dns_unavailable", d)
		} else if out.DKIM.Result != "temperror" {
			if dkim.IsPermFail(v.Err) {
				out.DKIM = check("permerror", "invalid_signature_parameters", d)
			} else {
				out.DKIM = check("fail", "signature_invalid", d)
			}
		}
	}
	out.DMARC = dmarcCheck
	if record != nil {
		policy := record.Policy
		if policyDomain != from && record.SubdomainPolicy != "" {
			policy = record.SubdomainPolicy
		}
		out.DMARC = check("fail", "no_aligned_pass", policyDomain)
		switch {
		case out.SPF.Aligned || out.DKIM.Aligned:
			out.DMARC = check("pass", "aligned_spf_or_dkim", policyDomain)
		case out.SPF.Result == "temperror" || out.DKIM.Result == "temperror":
			out.DMARC = check("temperror", "upstream_authentication_unavailable", policyDomain)
		}
		out.DMARC.Policy = string(policy)
		out.DMARC.Aligned = out.DMARC.Result == "pass"
	}
	return out
}

func missing(err error) bool {
	var dns *net.DNSError
	return errors.As(err, &dns) && dns.IsNotFound
}

func (e Engine) dmarc(ctx context.Context, from string) (*dmarc.Record, string, models.MailAuthCheck) {
	if from == "" {
		return nil, "", check("permerror", "invalid_from_identity", "")
	}
	names := []string{from}
	if root := org(from); root != from {
		names = append(names, root)
	}
	for _, name := range names {
		texts, err := e.DNS.LookupTXT(ctx, "_dmarc."+name+".")
		if err != nil && !missing(err) {
			return nil, name, check("temperror", "policy_dns_unavailable", name)
		}
		var found []string
		for _, txt := range texts {
			prefix, _, _ := strings.Cut(strings.TrimSpace(txt), ";")
			if strings.EqualFold(strings.TrimSpace(prefix), "v=DMARC1") {
				found = append(found, txt)
			}
		}
		if len(found) == 0 {
			continue
		}
		if len(found) != 1 {
			return nil, name, check("permerror", "multiple_policies", name)
		}
		r, err := dmarc.Parse(found[0])
		if err != nil {
			return nil, name, check("permerror", "invalid_policy", name)
		}
		return r, name, models.MailAuthCheck{}
	}
	return nil, from, check("none", "no_policy", from)
}
