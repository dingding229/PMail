package domain

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/dkimkey"
	"github.com/Jinnrry/pmail/utils/ip"
	"golang.org/x/net/publicsuffix"
)

type GuideRecord struct {
	Key            string   `json:"key"`
	Purpose        string   `json:"purpose"`
	Type           string   `json:"type"`
	Host           string   `json:"host"`
	FQDN           string   `json:"fqdn"`
	Zone           string   `json:"zone"`
	Value          string   `json:"value"`
	ValueChunks    []string `json:"value_chunks,omitempty"`
	SuggestedValue string   `json:"suggested_value,omitempty"`
	Priority       *uint16  `json:"priority,omitempty"`
	TTL            int      `json:"ttl"`
	Required       bool     `json:"required"`
	Status         string   `json:"status"`
	Actual         []string `json:"actual"`
	Matches        *bool    `json:"matches,omitempty"`
	Description    string   `json:"description"`
	FixHint        string   `json:"fix_hint,omitempty"`
	hint           string
}

type DNSGuide struct {
	Domain        string        `json:"domain"`
	MailHost      string        `json:"mail_host"`
	WebDomain     string        `json:"web_domain"`
	ServerIPv4    string        `json:"server_ipv4,omitempty"`
	ServerIPv6    string        `json:"server_ipv6,omitempty"`
	AllRequiredOK bool          `json:"all_required_ok"`
	CheckedAt     time.Time     `json:"checked_at"`
	Records       []GuideRecord `json:"records"`
	proxy         bool
}

type guideKey struct {
	id                                            int
	domain, selector, web, dkim, resolver, v4, v6 string
	proxy                                         bool
}

var dnsCache = struct {
	sync.Mutex
	entries map[guideKey]DNSGuide
}{entries: make(map[guideKey]DNSGuide)}

func guideInput(id int) (models.Domain, guideKey, error) {
	row, ok := GetByID(id)
	if !ok {
		return row, guideKey{}, ErrNotFound
	}
	_, signer, ok := Signer(row.Name)
	if !ok {
		return row, guideKey{}, ErrNotFound
	}
	dkim, err := dkimkey.Record(signer)
	if err != nil {
		return row, guideKey{}, err
	}
	web, _ := Normalize(config.Instance.WebDomain)
	if web == "" {
		web = "mail." + row.Name
	}
	mode := os.Getenv("PMAIL_WEB_MODE")
	proxy := mode == "proxy" || (mode == "" && config.Instance.HttpsEnabled == 2)
	return row, guideKey{id, row.Name, row.DkimSelector, web, dkim, strings.TrimSpace(os.Getenv("PMAIL_DNS_RESOLVER")), os.Getenv("PMAIL_PUBLIC_IPV4"), os.Getenv("PMAIL_PUBLIC_IPV6"), proxy}, nil
}

func resolverFor(endpoint string) (*net.Resolver, error) {
	if endpoint == "" {
		return net.DefaultResolver, nil
	}
	if _, err := netip.ParseAddrPort(endpoint); err != nil {
		return nil, fmt.Errorf("invalid PMAIL_DNS_RESOLVER: %w", err)
	}
	dialer := net.Dialer{}
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, endpoint)
	}}, nil
}

func Guide(ctx context.Context, id int, refresh bool, lang string) (DNSGuide, error) {
	row, key, err := guideInput(id)
	if err != nil {
		return DNSGuide{}, err
	}
	if !refresh {
		dnsCache.Lock()
		item, found := dnsCache.entries[key]
		dnsCache.Unlock()
		if found && time.Since(item.CheckedAt) < time.Minute {
			return localizeGuide(item, lang), nil
		}
	}
	resolver, err := resolverFor(key.resolver)
	if err != nil {
		return DNSGuide{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	v4, v6 := ip.PublicAddresses(ctx, key.v4, key.v6)
	result := buildGuide(row, key, v4, v6)
	var wg sync.WaitGroup
	for index := range result.Records {
		wg.Go(func() {
			checkGuideRecord(ctx, resolver, &result.Records[index], result.Domain, result.MailHost, v4, v6, result.proxy)
		})
	}
	wg.Wait()
	result.CheckedAt = time.Now().UTC()
	result.AllRequiredOK = true
	for _, record := range result.Records {
		if record.Required && record.Status != "ok" {
			result.AllRequiredOK = false
		}
	}
	dnsCache.Lock()
	if len(dnsCache.entries) >= 256 {
		for k, entry := range dnsCache.entries {
			if time.Since(entry.CheckedAt) >= time.Minute {
				delete(dnsCache.entries, k)
			}
		}
		if len(dnsCache.entries) >= 256 {
			clear(dnsCache.entries)
		}
	}
	dnsCache.entries[key] = result
	dnsCache.Unlock()
	return localizeGuide(result, lang), nil
}

func inZone(name, zone string) bool { return name == zone || strings.HasSuffix(name, "."+zone) }
func relativeHost(name, zone string) string {
	if name == zone {
		return "@"
	}
	return strings.TrimSuffix(name, "."+zone)
}
func hostZone(name, zone string) string {
	if inZone(name, zone) {
		return zone
	}
	other := ""
	for _, domain := range List() {
		if inZone(name, domain.Name) && len(domain.Name) > len(other) {
			other = domain.Name
		}
	}
	if other != "" {
		return other
	}
	if other, err := publicsuffix.EffectiveTLDPlusOne(name); err == nil {
		return other
	}
	return name
}
func fqdn(name string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(name), ".")) + "."
}

func buildGuide(row models.Domain, key guideKey, v4, v6 netip.Addr) DNSGuide {
	out := DNSGuide{Domain: row.Name, MailHost: "smtp." + row.Name, WebDomain: key.web, proxy: key.proxy}
	if v4.IsValid() {
		out.ServerIPv4 = v4.String()
	}
	if v6.IsValid() {
		out.ServerIPv6 = v6.String()
	}
	add := func(id, purpose, typ, name, value string, required bool) *GuideRecord {
		zone := hostZone(name, row.Name)
		out.Records = append(out.Records, GuideRecord{Key: id, Purpose: purpose, Type: typ, Host: relativeHost(name, zone), FQDN: fqdn(name), Zone: zone, Value: value, TTL: 3600, Required: required, Status: "advice", Actual: []string{}})
		return &out.Records[len(out.Records)-1]
	}
	d := row.Name
	m := out.MailHost
	priority := uint16(10)
	add("mx", "receiving", "MX", d, fqdn(m), true).Priority = &priority
	add("mail_a", "mail_host", "A", m, out.ServerIPv4, true)
	add("mail_aaaa", "mail_host", "AAAA", m, out.ServerIPv6, v6.IsValid())
	if key.web != m {
		value := out.ServerIPv4
		if key.proxy {
			value = ""
		}
		add("web_a", "web", "A", key.web, value, true)
	}
	add("spf", "authentication", "TXT", d, "v=spf1 mx a ~all", true)
	dkim := add("dkim", "authentication", "TXT", row.DkimSelector+"._domainkey."+d, key.dkim, true)
	if len(key.dkim) > 255 {
		dkim.ValueChunks = txtChunks(key.dkim)
	}
	add("dmarc", "authentication", "TXT", "_dmarc."+d, "v=DMARC1; p=quarantine; rua=mailto:postmaster@"+d, true)
	ptr := add("ptr", "reverse_dns", "PTR", m, fqdn(m), false)
	ptr.Host = "@"
	if v4.IsValid() {
		ptr.FQDN = reverseName(v4)
	} else if v6.IsValid() {
		ptr.FQDN = reverseName(v6)
	}
	if v4.IsValid() || v6.IsValid() {
		ptr.Zone = strings.TrimSuffix(ptr.FQDN, ".")
	}
	for _, name := range []string{"imap", "pop"} {
		add(name+"_alias", "mail_host", "CNAME", name+"."+d, fqdn(m), false)
	}
	for _, name := range []string{"autoconfig", "autodiscover"} {
		add(name, "client_configuration", "CNAME", name+"."+d, fqdn(key.web), false)
	}
	for _, item := range []struct {
		key, name string
		port      int
	}{{"imaps", "_imaps", 993}, {"submission", "_submission", 587}, {"pop3s", "_pop3s", 995}} {
		r := add(item.key, "client_configuration", "SRV", item.name+"._tcp."+d, fmt.Sprintf("0 %d %s", item.port, fqdn(m)), false)
		prio := uint16(10)
		r.Priority = &prio
	}
	return out
}

func txtChunks(value string) []string {
	parts := []string{}
	for len(value) > 255 {
		at := 255
		for value[at]&0xc0 == 0x80 {
			at--
		}
		parts = append(parts, value[:at])
		value = value[at:]
	}
	return append(parts, value)
}

func reverseName(ip netip.Addr) string {
	if ip.Is4() {
		p := ip.As4()
		return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa.", p[3], p[2], p[1], p[0])
	}
	var b strings.Builder
	a := ip.As16()
	const hex = "0123456789abcdef"
	for i := len(a) - 1; i >= 0; i-- {
		b.WriteByte(hex[a[i]&15])
		b.WriteByte('.')
		b.WriteByte(hex[a[i]>>4])
		b.WriteByte('.')
	}
	b.WriteString("ip6.arpa.")
	return b.String()
}

type guideResolver interface {
	spfResolver
	LookupAddr(context.Context, string) ([]string, error)
	LookupCNAME(context.Context, string) (string, error)
	LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error)
}

func checkGuideRecord(ctx context.Context, r guideResolver, rec *GuideRecord, domain, mail string, v4, v6 netip.Addr, proxy bool) {
	var err error
	match := false
	name := rec.FQDN
	switch rec.Type {
	case "A", "AAAA":
		if rec.Key == "mail_aaaa" && !v6.IsValid() {
			return
		}
		family := "ip4"
		if rec.Type == "AAAA" {
			family = "ip6"
		}
		var found []netip.Addr
		found, err = r.LookupNetIP(ctx, family, name)
		for _, addr := range found {
			rec.Actual = append(rec.Actual, addr.String())
			match = match || addr.String() == rec.Value
		}
		if rec.Key == "web_a" && proxy {
			match = len(found) > 0
		} else if rec.Value == "" {
			rec.Status = "error"
			rec.hint = "dns_guide_no_ip"
			return
		}
	case "MX":
		var found []*net.MX
		found, err = r.LookupMX(ctx, name)
		for _, mx := range found {
			rec.Actual = append(rec.Actual, fmt.Sprintf("%d %s", mx.Pref, mx.Host))
			match = match || (*rec.Priority == mx.Pref && fqdn(mx.Host) == rec.Value)
		}
	case "CNAME":
		var found string
		found, err = r.LookupCNAME(ctx, name)
		if err == nil && fqdn(found) != fqdn(name) {
			rec.Actual = append(rec.Actual, found)
			match = fqdn(found) == rec.Value
		}
	case "SRV":
		var found []*net.SRV
		_, found, err = r.LookupSRV(ctx, "", "", name)
		for _, srv := range found {
			value := fmt.Sprintf("%d %d %s", srv.Weight, srv.Port, fqdn(srv.Target))
			rec.Actual = append(rec.Actual, fmt.Sprintf("%d %s", srv.Priority, value))
			match = match || (*rec.Priority == srv.Priority && value == rec.Value)
		}
	case "PTR":
		matched := true
		checked := false
		for _, addr := range []netip.Addr{v4, v6} {
			if addr.IsValid() {
				checked = true
				found, _ := r.LookupAddr(ctx, addr.String())
				foundMatch := false
				for _, target := range found {
					rec.Actual = append(rec.Actual, target)
					foundMatch = foundMatch || fqdn(target) == fqdn(mail)
				}
				matched = matched && foundMatch
			}
		}
		if checked {
			rec.Matches = &matched
			if !matched {
				rec.hint = "dns_guide_ptr_fix"
			}
		}
		return
	case "TXT":
		var found []string
		found, err = r.LookupTXT(ctx, name)
		switch rec.Key {
		case "spf":
			rec.Actual = spfTexts(found)
			if len(rec.Actual) > 1 {
				rec.Status = "mismatch"
				rec.hint = "dns_guide_spf_merge"
				rec.SuggestedValue = suggestSPF(rec.Actual)
				return
			}
			if len(rec.Actual) == 1 {
				match = true
				checked := false
				for _, addr := range []netip.Addr{v4, v6} {
					if addr.IsValid() {
						checked = true
						eval := spfEvaluator{r: r, remaining: 10}
						match = eval.check(ctx, domain, rec.Actual[0], addr) == spfPass && match
					}
				}
				if !checked {
					rec.Status = "error"
					rec.hint = "dns_guide_no_ip"
					return
				}
				if !match {
					rec.hint = "dns_guide_spf_authorize"
					rec.SuggestedValue = suggestSPF(rec.Actual)
				}
			}
		case "dkim":
			rec.Actual = append(rec.Actual, found...)
			for _, item := range found {
				match = match || normalizeDKIM(item) == normalizeDKIM(rec.Value)
			}
		case "dmarc":
			for _, item := range found {
				fields := txtTags(item)
				if strings.EqualFold(fields["v"], "DMARC1") {
					rec.Actual = append(rec.Actual, item)
					match = true
					if strings.EqualFold(fields["p"], "none") {
						rec.hint = "dns_guide_dmarc_none"
					}
				}
			}
		}
	}
	var dnsErr *net.DNSError
	switch {
	case match:
		rec.Status = "ok"
	case err != nil && (!errors.As(err, &dnsErr) || !dnsErr.IsNotFound):
		rec.Status = "error"
		rec.hint = "dns_lookup_error"
	case len(rec.Actual) == 0:
		rec.Status = "missing"
	default:
		rec.Status = "mismatch"
	}
	if rec.hint == "" && (rec.Status == "missing" || rec.Status == "mismatch") {
		rec.hint = "dns_guide_check_panel"
	}
}

func txtTags(value string) map[string]string {
	values := map[string]string{}
	for part := range strings.SplitSeq(value, ";") {
		key, value, ok := strings.Cut(part, "=")
		if ok {
			values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	return values
}
func normalizeDKIM(value string) string {
	tags := txtTags(value)
	keyType := strings.ToLower(tags["k"])
	if keyType == "" {
		keyType = "rsa"
	}
	return strings.ToLower(tags["v"]) + ";" + keyType + ";" + strings.Join(strings.Fields(tags["p"]), "")
}

func localizeGuide(guide DNSGuide, lang string) DNSGuide {
	guide.Records = append([]GuideRecord(nil), guide.Records...)
	for i := range guide.Records {
		rec := &guide.Records[i]
		value := rec.Value
		if value == "" {
			value = i18n.Text(lang, "dns_guide_unknown_value")
		}
		if rec.Priority != nil {
			value = strconv.Itoa(int(*rec.Priority)) + " " + value
		}
		switch {
		case rec.Key == "ptr":
			rec.Description = i18n.Text(lang, "dns_guide_ptr", guide.MailHost)
		case rec.Key == "web_a" && guide.proxy:
			rec.Description = i18n.Text(lang, "dns_guide_proxy", rec.Zone, rec.Host)
		case rec.Key == "spf" && len(rec.Actual) > 0:
			rec.Description = i18n.Text(lang, "dns_guide_spf_existing", rec.Zone, rec.Host)
		case rec.Key == "autoconfig" || rec.Key == "autodiscover":
			rec.Description = i18n.Text(lang, "dns_guide_optional_web", rec.Zone, rec.Host, value)
		default:
			rec.Description = i18n.Text(lang, "dns_guide_panel", rec.Zone, rec.Type, rec.Host, value)
		}
		if rec.hint != "" {
			rec.FixHint = i18n.Text(lang, rec.hint)
		}
	}
	return guide
}

func Zone(ctx context.Context, id int, lang string) (string, string, error) {
	row, key, err := guideInput(id)
	if err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	v4, v6 := ip.PublicAddresses(ctx, key.v4, key.v6)
	guide := localizeGuide(buildGuide(row, key, v4, v6), lang)
	return guide.Domain, renderZone(guide, lang), nil
}

func renderZone(guide DNSGuide, lang string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "; %s\n$ORIGIN %s\n$TTL 3600\n\n", i18n.Text(lang, "dns_guide_zone_intro"), fqdn(guide.Domain))
	for _, rec := range guide.Records {
		fmt.Fprintf(&b, "; %s\n", rec.Description)
		if rec.Type == "PTR" || rec.Zone != guide.Domain || rec.Value == "" {
			continue
		}
		if !rec.Required {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s %d IN %s ", rec.Host, rec.TTL, rec.Type)
		if rec.Priority != nil {
			fmt.Fprintf(&b, "%d ", *rec.Priority)
		}
		if rec.Type == "TXT" {
			chunks := txtChunks(rec.Value)
			for i, part := range chunks {
				if i > 0 {
					b.WriteByte(' ')
				}
				fmt.Fprintf(&b, "\"%s\"", strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(part))
			}
		} else {
			b.WriteString(rec.Value)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
