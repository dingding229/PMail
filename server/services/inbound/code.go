package inbound

import (
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"golang.org/x/net/html"
)

const (
	codeBodyLimit                   = 8 << 10
	mixedCodeWithNumericBodyPenalty = 0.12
)

var codeKeyword = regexp.MustCompile(`(?i)短信验证码|验证码|校验码|动态密码|动态码|确认码|安全码|登录码|授权码|取件码|\b(?:verification(?:\s+code)?|one[- ]time(?:\s+(?:code|password))?|security[ \t]+code|login[ \t]+code|confirmation[ \t]+code|passcode|OTP|2FA|PIN|code)\b`)
var invalidSuffix = regexp.MustCompile(`(?i)^[ \t]*(?:[：:，,.-][ \t]*)?(?:无效|作废|已失效|已过期|invalid\b|expired\b|do not use\b)`)
var contextualNumber = regexp.MustCompile(`(?i)(?:房间(?:号)?|会议(?:室|号)?|room|invoice(?:[ \t]+(?:no|number|id))?|reference(?:[ \t]+code)?)\s*[:：#-]?\s*[A-Za-z0-9-]{4,}|\b[A-Za-z]{2,10}-(?:19|20)[0-9]{2}\b`)
var separateLine = regexp.MustCompile(`^[ \t\r:：=【】]*(?:是|为|is)?[ \t\r:：=【】]*(?:\n[ \t\r]*){1,3}$`)
var strongSubject = regexp.MustCompile(`(?i)(?:验证码|校验码|动态密码|动态码|确认码|安全码|登录码|授权码|取件码|\b(?:sign[- ]in[ \t]+code|log[- ]?in[ \t]+code|verification[ \t]+code|security[ \t]+code|confirmation[ \t]+code|one[- ]time[ \t]+(?:code|password)|passcode|OTP|2FA|PIN)\b)`)
var subjectFallbackLine = regexp.MustCompile(`(?i)^\s*(?:(?:use|enter|type|输入|使用|请使用|请填写)[ \t]*)?([A-Za-z0-9]{4,8})(?:[.。!！ \t]*|[ \t]+(?:to sign in|to log in|to continue|完成登录|完成验证)[.。!！ \t]*)$`)
var candidates = regexp.MustCompile(`[A-Za-z0-9]{4,}`)
var yearToken = regexp.MustCompile(`^(?:19|20)[0-9]{2}$`)
var droppedCodes = []*regexp.Regexp{
	contextualNumber,
	regexp.MustCompile(`(?i)\b(?:https?://|www\.)[^\s<>]+`),
	regexp.MustCompile(`\b[0-9]{2,4}[./-][0-9]{1,2}[./-][0-9]{1,4}\b|\b[0-9]{1,2}:[0-9]{2}(?::[0-9]{2})?\b`),
	regexp.MustCompile(`(?i)(?:[¥￥$€£]|\b(?:usd|cny|rmb|eur)\s*)\s*[0-9][0-9,.]*|\b[0-9][0-9,.]*\s*(?:元|美元|人民币|usd\b|cny\b|rmb\b|eur\b)`),
	regexp.MustCompile(`(?:\+\d{1,3}[ -]?)?\(?\d{2,4}\)?[ .-]\d{3,4}[ .-]\d{4}\b|\b\d{3,4}-\d{4,8}\b`),
	regexp.MustCompile(`(?i)(?:\border\s*(?:number|no\.?|id|#)?|\b(?:contact|telephone|phone|mobile|tracking|invoice)(?:\s+(?:number|no\.?|id))?|订单号?|运单号?|手机号|联系电话|电话号码)\s*[:：#=]?\s*[A-Za-z0-9-]{4,}`),
}
var falseCodeKey = regexp.MustCompile(`(?i)(?:\b(?:postal|zip|order|error|status|product|discount|promo|coupon|country|area|tracking|reference|invoice|referral)\s+)$`)
var codeExpiryTail = regexp.MustCompile(`(?i)^(?:\s*(?:minutes?|hours?|seconds?|days?)\b|\s*(?:分钟|小时|秒钟|天|年|月|日))`)
var unrelatedBetween = regexp.MustCompile(`(?i)\b(?:expires?|valid for|order|invoice|tracking|phone|mobile|amount|date|time is|last code|old code|do not use)\b|(?:过期|有效期|剩余|请拨打|订单|运单|金额|旧码|之前)`)
var quoteStart = regexp.MustCompile(`(?i)^(?:on .{1,180} wrote:|[- ]*(?:original|forwarded) message[- :]*|[- ]*(?:原始邮件|转发邮件)|在.{0,120}(?:写道|中写道)[：:]?|发件人[：:])`)

func unquoted(s string) string {
	var out strings.Builder
	for line := range strings.SplitSeq(s, "\n") {
		trim := strings.TrimSpace(line)
		if quoteStart.MatchString(trim) {
			break
		}
		if !strings.HasPrefix(trim, ">") {
			out.WriteString(line)
		}
		out.WriteByte('\n')
	}
	return out.String()
}
func codeHTML(text []byte) string {
	z := html.NewTokenizer(strings.NewReader(string(clippedBytes(text, 4*codeBodyLimit))))
	var out strings.Builder
	var skip int
	for out.Len() < codeBodyLimit {
		k := z.Next()
		if k == html.ErrorToken {
			if z.Err() == io.EOF {
				break
			}
			break
		}
		if k == html.StartTagToken || k == html.EndTagToken {
			name, _ := z.TagName()
			switch string(name) {
			case "blockquote", "script", "style":
				if k == html.StartTagToken {
					skip++
				} else if skip > 0 {
					skip--
				}
			case "p", "br", "div", "li", "td", "pre":
				if skip == 0 {
					out.WriteByte('\n')
				}
			}
			continue
		}
		if k == html.TextToken && skip == 0 {
			out.Write(clippedBytes(z.Text(), codeBodyLimit-out.Len()))
		}
	}
	return out.String()
}

type codeHit struct {
	value, source, kind string
	confidence          float64
	numeric             bool
	afterKeyword        bool
}

func findCodes(in, source string) []codeHit {
	masked := []byte(in)
	for _, r := range droppedCodes {
		for _, at := range r.FindAllStringIndex(in, -1) {
			for i := at[0]; i < at[1]; i++ {
				masked[i] = ' '
			}
		}
	}
	s := string(masked)
	labels := codeKeyword.FindAllStringIndex(s, -1)
	if len(labels) == 0 {
		return nil
	}
	out := []codeHit{}
	for _, at := range candidates.FindAllStringIndex(s, -1) {
		value := s[at[0]:at[1]]
		if len(value) > 8 || yearToken.MatchString(value) || codeExpiryTail.MatchString(s[at[1]:]) || invalidSuffix.MatchString(s[at[1]:]) {
			continue
		}
		digits := 0
		for _, c := range value {
			if c >= '0' && c <= '9' {
				digits++
			}
		}
		if digits == 0 {
			continue
		}
		best := codeHit{value: value, source: source, numeric: digits == len(value)}
		for _, key := range labels {
			if falseCodeKey.MatchString(s[max(0, key[0]-28):key[0]]) {
				continue
			}
			var middle string
			after := at[0] >= key[1]
			switch {
			case after:
				middle = s[key[1]:at[0]]
			case at[1] <= key[0]:
				middle = s[at[1]:key[0]]
			default:
				continue
			}
			if len(middle) > 90 || (strings.Contains(middle, "\n") && (!after || !separateLine.MatchString(middle))) || strings.ContainsAny(middle, ".。!?！？;") || unrelatedBetween.MatchString(middle) {
				continue
			}
			distance := utf8.RuneCountInString(strings.Join(strings.Fields(middle), " "))
			if distance > 30 || (!after && distance > 18) {
				continue
			}
			if digits < len(value) && strings.ToUpper(value) != value && (distance > 12 || digits < 2) {
				continue
			}
			strength := 0.85 - 0.011*float64(distance)
			if source == "subject" && after {
				strength += 0.10
			}
			if !after {
				strength -= 0.06
			}
			if strings.ContainsAny(middle, ":：=【") {
				strength += 0.04
			}
			if strength > best.confidence {
				best.confidence = strength
				best.afterKeyword = after
				best.kind = codeKind(s[key[0]:key[1]])
			}
		}
		if best.confidence >= 0.56 {
			out = append(out, best)
		}
	}
	return out
}

func codeKind(label string) string {
	if strings.Contains(label, "取件码") {
		return "pickup"
	}
	return ""
}

func subjectOnlyCode(subject, body string) (codeHit, bool) {
	if !strongSubject.MatchString(subject) || falseCodeKey.MatchString(subject) {
		return codeHit{}, false
	}
	var matches []string
	for line := range strings.SplitSeq(body, "\n") {
		found := subjectFallbackLine.FindStringSubmatch(line)
		if len(found) < 2 || yearToken.MatchString(found[1]) || !strings.ContainsAny(found[1], "0123456789") {
			continue
		}
		matches = append(matches, found[1])
	}
	if len(matches) != 1 {
		return codeHit{}, false
	}
	value := matches[0]
	return codeHit{value: value, source: "body", kind: codeKind(subject), confidence: 0.69, numeric: strings.Trim(value, "0123456789") == ""}, true
}

func recognizeCode(email *parsemail.Email) *VerificationCode {
	plain := string(clippedBytes(email.Text, codeBodyLimit))
	if email.TextFromHTML {
		plain = codeHTML(email.HTML)
	}
	subject := string(clippedBytes([]byte(email.Subject), 1024))
	body := unquoted(plain)
	subjectHits, bodyHits := findCodes(subject, "subject"), findCodes(body, "body")
	afterKeyword := func(c codeHit) bool { return c.afterKeyword }
	preferBodyOverPrefix := slices.ContainsFunc(bodyHits, afterKeyword)
	if len(bodyHits) == 0 && !slices.ContainsFunc(subjectHits, afterKeyword) {
		// A subject prefix can be an identifier even when the body contains
		// only the code. In that contest require a single numeric fallback.
		if hit, ok := subjectOnlyCode(subject, body); ok && (len(subjectHits) == 0 || hit.numeric) {
			bodyHits = append(bodyHits, hit)
			preferBodyOverPrefix = true
		}
	}
	bodyHasNumeric := slices.ContainsFunc(bodyHits, func(c codeHit) bool { return c.numeric })
	distinct := map[string]bool{}
	var best codeHit
	for _, c := range append(subjectHits, bodyHits...) {
		distinct[c.value] = true
		if preferBodyOverPrefix && c.source == "subject" && !c.afterKeyword {
			continue
		}
		if bodyHasNumeric && !c.numeric {
			c.confidence -= mixedCodeWithNumericBodyPenalty
		}
		if c.confidence > best.confidence {
			best = c
		}
	}
	if best.value == "" {
		return nil
	}
	confidence := best.confidence
	if len(distinct) == 1 {
		confidence += 0.035
	} else {
		confidence -= 0.12
	}
	return &VerificationCode{Code: best.value, Confidence: max(0, min(1, confidence)), Source: best.source, Kind: best.kind}
}
