package inbound

import (
	"context"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"xorm.io/xorm"
)

var weights = map[string]float64{
	"auth_spf_fail": 2, "auth_spf_softfail": 1, "auth_spf_permerror": 0.5,
	"auth_dkim_fail": 1.5, "auth_dkim_permerror": 0.5, "auth_dkim_unaligned": 1,
	"auth_dmarc_fail": 3.5, "auth_dmarc_permerror": 0.5, "missing_auth": 0.5,
	"dnsbl_listed": 4, "reply_to_domain_mismatch": 1.25, "local_display_name_spoof": 3,
	"html_only": 0.75, "suspicious_links": 1.75, "url_shortener": 0.75, "many_links": 1.25,
	"upper_subject": 0.75, "keyword_match": 2, "empty_subject_attachment": 1.5, "executable_attachment": 4,
}
var urlPattern = regexp.MustCompile(`(?i)https?://[^\s<>"'\x60]+`)
var claimedDomain = regexp.MustCompile(`(?i)[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?\.[a-z]{2,63}`)
var shorteners = map[string]bool{"bit.ly": true, "tinyurl.com": true, "t.co": true, "is.gd": true, "ow.ly": true, "rb.gy": true, "cutt.ly": true, "shorturl.at": true}

const defaultSpamKeywords = "guaranteed returns,claim your prize,free cryptocurrency,act now to claim,点击领取奖金,稳赚不赔,博彩送彩金,代开发票,刷单返利"

func wordSample(e *parsemail.Email) string {
	return string(clippedBytes([]byte(e.Subject), 1024)) + "\n" + string(clippedBytes(e.Text, 4096))
}
func keywordConfig() []string {
	raw, ok := os.LookupEnv("PMAIL_SPAM_KEYWORDS")
	if !ok {
		raw = defaultSpamKeywords
	}
	raw = string(clippedBytes([]byte(raw), 8192))
	var out []string
	for v := range strings.SplitSeq(raw, ",") {
		v = strings.TrimSpace(strings.ToLower(v))
		if safeString(v, 80) {
			out = append(out, v)
		}
		if len(out) >= 64 {
			break
		}
	}
	return out
}
func baseReasons(e *parsemail.Email, auth *models.MailAuthResults) []Reason {
	out := []Reason{}
	add := func(code string) { out = append(out, Reason{Code: code, Weight: weights[code]}) }
	if auth != nil {
		switch auth.SPF.Result {
		case "fail":
			add("auth_spf_fail")
		case "softfail":
			add("auth_spf_softfail")
		case "permerror":
			add("auth_spf_permerror")
		}
		switch auth.DKIM.Result {
		case "fail":
			add("auth_dkim_fail")
		case "permerror":
			add("auth_dkim_permerror")
		case "pass":
			if !auth.DKIM.Aligned {
				add("auth_dkim_unaligned")
			}
		}
		switch auth.DMARC.Result {
		case "fail":
			add("auth_dmarc_fail")
		case "permerror":
			add("auth_dmarc_permerror")
		}
		if auth.SPF.Result == "none" && auth.DKIM.Result == "none" && auth.DMARC.Result == "none" {
			add("missing_auth")
		}
	}
	if e.From != nil {
		from := addressHost(e.From.EmailAddress)
		for _, r := range e.ReplyTo {
			if r != nil && from != "" && addressHost(r.EmailAddress) != "" && !sameDomain(from, addressHost(r.EmailAddress)) {
				add("reply_to_domain_mismatch")
				break
			}
		}
		if from != "" && !domain.IsLocal(from) {
			for _, name := range claimedDomain.FindAllString(string(clippedBytes([]byte(e.From.Name), 1024)), 8) {
				if domain.IsLocal(normalizeDomain(name)) {
					add("local_display_name_spoof")
					break
				}
			}
		}
	}
	if len(e.HTML) > 0 && (len(e.Text) == 0 || e.TextFromHTML) {
		add("html_only")
	}
	input := string(clippedBytes(e.Text, 16384)) + " " + string(clippedBytes(e.HTML, 32768))
	urls := map[string]bool{}
	suspicious, short := 0, 0
	for _, link := range urlPattern.FindAllString(input, 150) {
		link = strings.TrimRight(link, ").,;]}")
		if urls[link] {
			continue
		}
		u, err := url.Parse(link)
		if err != nil || u.Hostname() == "" {
			continue
		}
		host := normalizeDomain(u.Hostname())
		if e.From != nil && sameDomain(host, addressHost(e.From.EmailAddress)) {
			continue
		}
		urls[link] = true
		if shorteners[host] {
			short++
			suspicious++
		} else if ip, err := netip.ParseAddr(host); err == nil && ip.IsValid() {
			suspicious++
		}
	}
	if short > 0 {
		add("url_shortener")
	}
	if suspicious > 0 && suspicious*2 >= len(urls) {
		add("suspicious_links")
	}
	if len(urls) >= 12 {
		add("many_links")
	}
	upper, lower := 0, 0
	for _, c := range string(clippedBytes([]byte(e.Subject), 1024)) {
		if unicode.IsUpper(c) {
			upper++
		}
		if unicode.IsLower(c) {
			lower++
		}
	}
	if upper >= 10 && lower == 0 {
		add("upper_subject")
	}
	sample := strings.ToLower(wordSample(e))
	for _, v := range keywordConfig() {
		if strings.Contains(sample, v) {
			add("keyword_match")
			break
		}
	}
	if strings.TrimSpace(e.Subject) == "" && len(e.Attachments) > 0 {
		add("empty_subject_attachment")
	}
	for _, a := range e.Attachments {
		if a != nil && (dangerousExt[extLower(a.Filename)] || executableMagic(a.Content)) {
			add("executable_attachment")
			break
		}
	}
	return out
}
func extLower(s string) string {
	dot := strings.LastIndexAny(s, "./\\")
	if dot < 0 || s[dot] != '.' {
		return ""
	}
	return strings.ToLower(s[dot:])
}
func sameDomain(a, b string) bool {
	return a != "" && b != "" && (a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a))
}

func classify(ctx context.Context, engine *xorm.Engine, userID int, e *parsemail.Email, env Envelope, base []Reason, threshold float64) (*Spam, error) {
	p, err := preference(ctx, engine, userID, threshold)
	if err != nil {
		return nil, err
	}
	if !p.Enabled {
		return &Spam{Verdict: "unknown", Reasons: []Reason{}}, nil
	}
	result := &Spam{Reasons: append([]Reason{}, base...)}
	for _, r := range result.Reasons {
		result.Score += r.Weight
	}
	rules, err := rules(ctx, engine, userID)
	if err != nil {
		return nil, err
	}
	sample := strings.Join(strings.Fields(strings.ToLower(wordSample(e))), " ")
	from := ""
	if e.From != nil {
		from = strings.ToLower(e.From.EmailAddress)
	}
	envelope := strings.ToLower(strings.TrimSpace(strings.Trim(env.MailFrom, "<>")))
	blocked := false
	for _, r := range rules {
		match := false
		switch r.Kind {
		case "address":
			match = r.Value == from || r.Value == envelope
		case "domain":
			for _, h := range []string{addressHost(from), addressHost(envelope)} {
				if h == r.Value || strings.HasSuffix(h, "."+r.Value) {
					match = true
					break
				}
			}
		case "keyword":
			match = strings.Contains(sample, r.Value)
		}
		if !match {
			continue
		}
		// Explicit allow wins over block if policy entries overlap.
		if r.Action == "allow" {
			result.Reasons = append(result.Reasons, Reason{Code: "allow_rule_match", Weight: -result.Score})
			result.Score = 0
			result.Verdict = "ham"
			return result, nil
		}
		blocked = true
	}
	if blocked {
		delta := max(0, p.Threshold-result.Score)
		result.Reasons = append(result.Reasons, Reason{Code: "block_rule_match", Weight: delta})
		result.Score += delta
		result.Verdict = "spam"
		return result, nil
	}
	if p.BayesEnabled {
		delta, err := bayesScore(ctx, engine, userID, string(clippedBytes([]byte(e.Subject), 1024)), string(clippedBytes(e.Text, tokenBodyLimit)))
		if err != nil {
			return nil, err
		}
		if delta != 0 {
			code := "bayes_spam"
			if delta < 0 {
				code = "bayes_ham"
			}
			result.Reasons = append(result.Reasons, Reason{Code: code, Weight: delta})
			result.Score += delta
		}
	}
	result.Verdict = "ham"
	if result.Score >= p.Threshold {
		result.Verdict = "spam"
	}
	return result, nil
}
