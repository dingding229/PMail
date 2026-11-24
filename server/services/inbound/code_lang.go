package inbound

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Labels used next to one-time codes in languages other than Chinese and
// English. Each label is matched case-insensitively and must stand as a whole
// word in scripts that separate words with spaces.
var multilingualLabels = []string{
	// Spanish, Portuguese, Catalan, Galician
	"código de verificación", "código de seguridad", "código de confirmación", "código de acceso",
	"código de verificação", "código de segurança", "código de confirmação", "código", "codi de verificació",
	// French, Italian, Romanian
	"code de vérification", "code de sécurité", "code de confirmation", "code d'accès",
	"codice di verifica", "codice di sicurezza", "codice", "cod de verificare", "cod de confirmare",
	// German, Dutch, Nordic
	"bestätigungscode", "verifizierungscode", "sicherheitscode", "anmeldecode", "einmalpasswort", "einmalcode",
	"verificatiecode", "beveiligingscode", "bevestigingscode",
	"verifieringskod", "bekräftelsekod", "säkerhetskod", "engångskod",
	"bekreftelseskode", "verifiseringskode", "engangskode", "bekræftelseskode", "sikkerhedskode",
	"vahvistuskoodi", "turvakoodi", "koodi", "koodisi",
	// Central and Eastern Europe
	"kod weryfikacyjny", "kod potwierdzający", "kod bezpieczeństwa", "kod",
	"ověřovací kód", "potvrzovací kód", "overovací kód", "kód", "ellenőrző kód", "megerősítő kód",
	"verifikacijski kod", "potvrdni kod",
	// Cyrillic, Greek
	"код подтверждения", "проверочный код", "одноразовый пароль", "код", "кода", "коду", "кодом",
	"код підтвердження", "код верифікації", "код за потвърждение",
	"κωδικός επαλήθευσης", "κωδικός επιβεβαίωσης", "κωδικός",
	// Turkish, Indonesian, Malay, Vietnamese, Filipino
	"doğrulama kodunuz", "doğrulama kodu", "onay kodunuz", "onay kodu", "güvenlik kodu", "kodu", "kodunuz",
	"kode verifikasi", "kode otp", "kode", "kod pengesahan", "kod keselamatan",
	"mã xác minh", "mã xác thực", "mã xác nhận", "mã otp", "mã bảo mật",
	// Arabic, Persian, Hebrew, Urdu
	"رمز التحقق", "رمز التأكيد", "كود التحقق", "رمز", "كود",
	"کد تأیید", "کد تایید", "کد امنیتی", "کد",
	"קוד אימות", "קוד",
	"تصدیقی کوڈ", "کوڈ",
	// Indic scripts
	"सत्यापन कोड", "पुष्टिकरण कोड", "ओटीपी", "कोड",
	"যাচাইকরণ কোড", "কোড", "சரிபார்ப்புக் குறியீடு", "குறியீடு", "ధృవీకరణ కోడ్", "కోడ్",
	// Thai, Lao, Khmer, Burmese
	"รหัสยืนยัน", "รหัสผ่านครั้งเดียว", "รหัส otp", "รหัส",
	"ລະຫັດຢືນຢັນ", "លេខកូដផ្ទៀងផ្ទាត់", "အတည်ပြုကုဒ်",
	// Japanese, Korean
	"認証コード", "確認コード", "検証コード", "セキュリティコード", "ワンタイムパスワード", "認証番号", "確認番号", "ワンタイムコード", "コード",
	"인증번호", "인증 번호", "인증코드", "인증 코드", "보안코드", "보안 코드", "확인 코드", "확인코드", "코드",
}

// Multi-word labels strong enough to mark a subject as a verification mail.
var multilingualStrongSubject = func() []string {
	var out []string
	for _, l := range multilingualLabels {
		if strings.ContainsRune(l, ' ') || utf8.RuneCountInString(l) >= 6 {
			out = append(out, l)
		}
	}
	return out
}()

func labelAlternation(labels []string) string {
	sorted := slices.Clone(labels)
	// Longer labels first so a full phrase wins over its generic tail.
	slices.SortFunc(sorted, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	parts := make([]string, len(sorted))
	for i, l := range sorted {
		parts[i] = strings.ReplaceAll(regexp.QuoteMeta(l), " ", `[ \t]+`)
	}
	return strings.Join(parts, "|")
}

// Scripts that do not put spaces between words, or attach particles to nouns.
func spacelessRune(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar)
}

func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r)
}

// labelBounded rejects labels that are only part of a longer word, such as
// "код" inside "кодекс". Go's \b only understands ASCII.
func labelBounded(s string, at []int) bool {
	first, _ := utf8.DecodeRuneInString(s[at[0]:at[1]])
	last, _ := utf8.DecodeLastRuneInString(s[at[0]:at[1]])
	if before, _ := utf8.DecodeLastRuneInString(s[:at[0]]); at[0] > 0 && !spacelessRune(first) && wordRune(before) {
		return false
	}
	if after, _ := utf8.DecodeRuneInString(s[at[1]:]); at[1] < len(s) && !spacelessRune(last) && wordRune(after) {
		return false
	}
	return true
}

// ---- Language-independent fallback ----

var structuralToken = regexp.MustCompile(`^(?:[0-9]{4,8}|[A-Z0-9]{5,8})$`)

const lineDecoration = " \t\u00a0:：=*#>-–—•·▼▽▶►→←【】[]「」『』()（）<《》〈〉\"'“”‘’|｜"

// Labels in the line above a lone number that make it an identifier, not a code.
var identifierLine = regexp.MustCompile(`(?i)order|invoice|tracking|reference|ticket|case|customer|member|account (?:no|number)|zip|postal|amount|total|price|` +
	`订单|訂單|运单|運單|单号|單號|编号|編號|发票|發票|客户|客戶|会员|會員|金额|金額|合计|合計|価格|注文|請求|주문|금액|` +
	`pedido|factura|fatura|commande|facture|bestell|rechnung|ordine|fattura|bestelling|factuur|zamówien|faktura|заказ|сч[её]т|sipariş|pesanan|đơn hàng|طلب|فاتورة|הזמנה`)

var automatedSender = regexp.MustCompile(`(?i)^(?:[^@]*[._+-])?(?:no-?reply|do-?not-?reply|donotreply|noreply|verify|verification|security|secure|account|accounts|auth|login|signin|otp|identity|id|notify|notification|notifications|alert|alerts|mailer)(?:[._+-][^@]*)?@|@(?:[^@]*\.)?(?:accounts?|id|auth|login|security|verify|identity|signin)\.`)

var subjectBracketed = regexp.MustCompile(`[【\[「『(（<《]\s*([0-9]{4,8}|[A-Z0-9]{5,8})\s*[】\]」』)）>》]`)

func tokenDigits(v string) int {
	n := 0
	for _, c := range v {
		if c >= '0' && c <= '9' {
			n++
		}
	}
	return n
}

func structuralCandidate(v string) bool {
	if !structuralToken.MatchString(v) || yearToken.MatchString(v) {
		return false
	}
	d := tokenDigits(v)
	if d == len(v) {
		return true
	}
	// Mixed codes need both letters and digits, so words in capitals are skipped.
	return d >= 2 && len(v)-d >= 1
}

// isolatedTokens returns codes printed on a line of their own, ignoring
// decoration, unless the line above names an identifier.
func isolatedTokens(body string) []string {
	var out []string
	prev := ""
	for line := range strings.SplitSeq(body, "\n") {
		trim := strings.Trim(line, lineDecoration)
		if trim == "" {
			continue
		}
		if structuralCandidate(trim) && !identifierLine.MatchString(prev) {
			out = append(out, trim)
		}
		prev = trim
	}
	return out
}

var emphasisStyle = regexp.MustCompile(`(?i)letter-spacing\s*:\s*[1-9]|font-weight\s*:\s*(?:bold|[6-9]00)|font-family\s*:[^;]*mono`)
var fontSize = regexp.MustCompile(`(?i)font-size\s*:\s*([0-9]+(?:\.[0-9]+)?)\s*(px|pt|em|rem)`)

func emphasizedStyle(style string) bool {
	if emphasisStyle.MatchString(style) {
		return true
	}
	if m := fontSize.FindStringSubmatch(style); m != nil {
		n, _ := strconv.ParseFloat(m[1], 64)
		switch strings.ToLower(m[2]) {
		case "px":
			return n >= 18
		case "pt":
			return n >= 14
		default:
			return n >= 1.3
		}
	}
	return false
}

var voidTags = map[string]bool{"br": true, "img": true, "hr": true, "meta": true, "link": true, "input": true, "wbr": true, "col": true, "source": true}

// emphasizedTokens returns codes that HTML shows on their own in bold, large,
// spaced or monospaced type, the way most services present a one-time code.
func emphasizedTokens(doc []byte) []string {
	if len(doc) == 0 {
		return nil
	}
	type open struct {
		name     string
		emphasis bool
	}
	z := html.NewTokenizer(strings.NewReader(string(clippedBytes(doc, 4*codeBodyLimit))))
	var stack []open
	var out []string
	var prev string
	skip := 0
	for {
		k := z.Next()
		if k == html.ErrorToken {
			return out
		}
		switch k {
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			emph := tag == "b" || tag == "strong" || tag == "h1" || tag == "h2" || tag == "h3" || tag == "big" || tag == "code" || tag == "kbd"
			for hasAttr {
				var key, val []byte
				key, val, hasAttr = z.TagAttr()
				if string(key) == "style" && emphasizedStyle(string(val)) {
					emph = true
				}
			}
			if tag == "blockquote" || tag == "script" || tag == "style" {
				skip++
			}
			if k == html.StartTagToken && !voidTags[tag] {
				stack = append(stack, open{tag, emph})
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if (tag == "blockquote" || tag == "script" || tag == "style") && skip > 0 {
				skip--
			}
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].name == tag {
					stack = stack[:i]
					break
				}
			}
		case html.TextToken:
			if skip > 0 {
				continue
			}
			text := strings.Trim(html.UnescapeString(string(z.Text())), lineDecoration+"\r\n")
			if text == "" {
				continue
			}
			if structuralCandidate(text) && !identifierLine.MatchString(prev) && slices.ContainsFunc(stack, func(o open) bool { return o.emphasis }) {
				out = append(out, text)
			}
			prev = text
		}
	}
}

func uniqueTokens(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		for _, v := range l {
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}

func tokenIn(s, v string) bool {
	for _, at := range candidates.FindAllStringIndex(s, -1) {
		if s[at[0]:at[1]] == v {
			return true
		}
	}
	return false
}

// structuralCode finds a code without understanding the language: the code
// stands out (own line, emphasis, or brackets in the subject) and another
// independent signal agrees.
func structuralCode(subject, body string, htmlDoc []byte, from string) (codeHit, bool) {
	isolated := isolatedTokens(body)
	emphasized := emphasizedTokens(htmlDoc)
	standout := uniqueTokens(isolated, emphasized)
	var bracketed []string
	for _, m := range subjectBracketed.FindAllStringSubmatch(subject, -1) {
		if structuralCandidate(m[1]) {
			bracketed = append(bracketed, m[1])
		}
	}
	bracketed = uniqueTokens(bracketed)
	pick := func(v string, conf float64) (codeHit, bool) {
		return codeHit{value: v, source: "body", confidence: conf, numeric: tokenDigits(v) == len(v)}, true
	}
	inSubject := slices.DeleteFunc(slices.Clone(standout), func(v string) bool { return !tokenIn(subject, v) })
	if len(inSubject) == 1 {
		return pick(inSubject[0], 0.72)
	}
	if len(bracketed) == 1 && tokenIn(body, bracketed[0]) {
		return pick(bracketed[0], 0.66)
	}
	if len(standout) != 1 || !automatedSender.MatchString(from) {
		return codeHit{}, false
	}
	v := standout[0]
	if slices.Contains(isolated, v) && slices.Contains(emphasized, v) {
		return pick(v, 0.62)
	}
	if tokenDigits(v) == len(v) && len(v) >= 6 {
		return pick(v, 0.58)
	}
	return codeHit{}, false
}
