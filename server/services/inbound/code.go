package inbound

import (
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"golang.org/x/net/html"
)

const codeBodyLimit = 8 << 10

var codeKeyword = regexp.MustCompile(`(?i)验证码|校验码|动态码|确认码|安全码|\b(?:verification(?:\s+code)?|one[- ]time(?:\s+(?:code|password))?|passcode|OTP|2FA|code)\b`)
var candidates = regexp.MustCompile(`[A-Za-z0-9]{4,}`)
var yearToken = regexp.MustCompile(`^(?:19|20)[0-9]{2}$`)
var droppedCodes = []*regexp.Regexp{
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
	value, source string
	confidence    float64
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
		if len(value) > 8 || yearToken.MatchString(value) || codeExpiryTail.MatchString(s[at[1]:]) {
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
		best := 0.0
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
			if len(middle) > 90 || strings.Contains(middle, "\n") || strings.ContainsAny(middle, ".。!?！？;") || unrelatedBetween.MatchString(middle) {
				continue
			}
			distance := utf8.RuneCountInString(middle)
			if distance > 30 || (!after && distance > 18) {
				continue
			}
			if digits < len(value) && strings.ToUpper(value) != value && (distance > 12 || digits < 2) {
				continue
			}
			strength := 0.85 - 0.011*float64(distance)
			if source == "subject" {
				strength += 0.10
			}
			if !after {
				strength -= 0.06
			}
			if strings.ContainsAny(middle, ":：=【") {
				strength += 0.04
			}
			best = max(best, strength)
		}
		if best >= 0.56 {
			out = append(out, codeHit{value, source, best})
		}
	}
	return out
}

func recognizeCode(email *parsemail.Email) *VerificationCode {
	plain := string(clippedBytes(email.Text, codeBodyLimit))
	if email.TextFromHTML {
		plain = codeHTML(email.HTML)
	}
	all := append(findCodes(string(clippedBytes([]byte(email.Subject), 1024)), "subject"), findCodes(unquoted(plain), "body")...)
	distinct := map[string]bool{}
	var best codeHit
	for _, c := range all {
		distinct[c.value] = true
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
	return &VerificationCode{best.value, max(0, min(1, confidence)), best.source}
}
