package parsemail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	stdmail "net/mail"
	"net/textproto"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
	"github.com/microcosm-cc/bluemonday"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

type User struct {
	EmailAddress string `json:"EmailAddress"`
	Name         string `json:"Name"`
}

func (u User) Build() string {
	if u.Name != "" {
		return fmt.Sprintf("\"%s\" <%s>", mime.QEncoding.Encode("utf-8", u.Name), u.EmailAddress)
	}
	return fmt.Sprintf("<%s>", u.EmailAddress)
}

func (u User) GetDomainAccount() (string, string) {
	infos := strings.Split(u.EmailAddress, "@")
	if len(infos) >= 2 {
		return infos[0], infos[1]
	}

	return "", ""
}

type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
	ContentID   string
	Charset     string `json:",omitempty"`
}

// EmailAuthentication 表示收信时的发件人身份认证结果。
type EmailAuthentication struct {
	SPFPassed  bool
	DKIMPassed bool
	Dangerous  bool
}

// NewEmailAuthentication 根据 SPF 和 DKIM 结果创建统一的邮件认证结论。
func NewEmailAuthentication(spfPassed, dkimPassed bool) *EmailAuthentication {
	return &EmailAuthentication{
		SPFPassed:  spfPassed,
		DKIMPassed: dkimPassed,
		Dangerous:  !spfPassed && !dkimPassed,
	}
}

// Email is the type used for email messages
type Email struct {
	ReplyTo    []*User
	From       *User
	To         []*User
	Bcc        []*User
	Cc         []*User
	Subject    string
	Text       []byte // Plaintext message (optional)
	TextFormat string
	HTML       []byte // Html message (optional)
	Sender     *User  // override From as SMTP envelope sender (optional)
	// Headers contains custom transport headers that must survive generated mail.
	Headers        textproto.MIMEHeader
	Attachments    []*Attachment
	ReadReceipt    []string
	Date           string
	Status         int // 0未发送，1已发送，2发送失败，3删除，5广告邮件
	MessageId      int64
	MsgID          string // RFC-compliant Message-ID, persisted in DB
	Size           int
	Authentication *EmailAuthentication
	// EnvelopeFrom is set by SMTP sessions when the envelope sender is known.
	EnvelopeFrom      string `json:"-"`
	EnvelopeFromKnown bool   `json:"-"`
	// RawData is an optional in-memory copy used by forwarding paths to keep
	// transport headers (including loop markers) through local delivery.
	RawData []byte `json:"-"`
}

// HeaderValue returns a header value from the RFC 5322 header section only.
func HeaderValue(raw []byte, name string) string {
	message, err := stdmail.ReadMessage(bytes.NewReader(raw))
	if err != nil || message == nil {
		return ""
	}
	return message.Header.Get(name)
}

// IsAutomaticMessage identifies DSNs and other automatically generated mail.
// It deliberately ignores the body so text mentioning a header cannot suppress forwarding.
func IsAutomaticMessage(raw []byte) bool {
	auto := strings.TrimSpace(HeaderValue(raw, "Auto-Submitted"))
	if auto != "" && !strings.EqualFold(auto, "no") {
		return true
	}
	contentType := strings.ToLower(strings.TrimSpace(HeaderValue(raw, "Content-Type")))
	if strings.HasPrefix(contentType, "multipart/report") {
		return true
	}
	dsn := strings.TrimSpace(HeaderValue(raw, "X-PMail-DSN"))
	return dsn != ""
}

func IsDaemonAddress(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "<>" {
		return true
	}
	if parsed, err := stdmail.ParseAddress(raw); err == nil {
		raw = parsed.Address
	}
	local, _, ok := strings.Cut(raw, "@")
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(local), "mailer-daemon") ||
		strings.EqualFold(strings.TrimSpace(local), "postmaster")
}

// ShouldSkipForward centralizes the automatic-message guard used by global and rule forwarding.
func ShouldSkipForward(email *Email, raw []byte) bool {
	if email == nil || email.From == nil || strings.TrimSpace(email.From.EmailAddress) == "" {
		return true
	}
	if email.EnvelopeFromKnown && strings.TrimSpace(email.EnvelopeFrom) == "" {
		return true
	}
	if IsDaemonAddress(email.From.EmailAddress) || IsAutomaticMessage(raw) {
		return true
	}
	if email.Headers != nil {
		auto := strings.TrimSpace(email.Headers.Get("Auto-Submitted"))
		if auto != "" && !strings.EqualFold(auto, "no") {
			return true
		}
		if strings.TrimSpace(email.Headers.Get("X-PMail-DSN")) != "" {
			return true
		}
	}
	return false
}

// GenerateMsgID creates an RFC-compliant Message-ID unique enough to avoid spam filters.
// It must be called once at email creation time and the result persisted to DB.
func GenerateMsgID(domain string) string {
	randBytes := make([]byte, 16)
	_, _ = rand.Read(randBytes)
	return fmt.Sprintf("%x.%d@%s", randBytes, time.Now().UnixNano(), domain)
}

// Xss filter policy
var (
	strictPolicy  *bluemonday.Policy
	relaxedPolicy *bluemonday.Policy
)

func init() {
	strictPolicy = bluemonday.StrictPolicy()

	relaxedPolicy = bluemonday.NewPolicy()

	relaxedPolicy.AllowElements("p", "br", "strong", "em", "u", "b", "i", "h1", "h2", "h3", "h4", "h5", "h6")
	relaxedPolicy.AllowElements("div", "span", "center")
	relaxedPolicy.AllowElements("ul", "ol", "li")
	relaxedPolicy.AllowElements("blockquote", "cite")

	relaxedPolicy.AllowElements("table", "tbody", "thead", "tfoot", "caption", "colgroup", "col", "tr", "td", "th")
	relaxedPolicy.AllowElements("pre", "code", "kbd", "samp", "var", "tt")
	codeClass := `((language|lang)-[A-Za-z0-9_+#.-]{1,32}|hljs|highlight|sourceCode)`
	relaxedPolicy.AllowAttrs("class").Matching(regexp.MustCompile(`^`+codeClass+`([ \t]+`+codeClass+`)*$`)).OnElements("pre", "code")
	relaxedPolicy.AllowStyles("white-space").Matching(regexp.MustCompile(`^(pre|pre-wrap|pre-line|break-spaces|normal|nowrap)$`)).OnElements("pre")
	relaxedPolicy.AllowStyles("font-family").Matching(regexp.MustCompile(`^[A-Za-z0-9 ,_'"-]{1,128}$`)).OnElements("pre")
	relaxedPolicy.AllowStyles("tab-size").Matching(regexp.MustCompile(`^([1-9]|[1-3][0-9])$`)).OnElements("pre")
	relaxedPolicy.AllowStyles("color", "background-color", "text-align", "font-size", "font-weight", "font-style", "text-decoration", "border-collapse", "padding", "margin", "width", "height").OnElements("p", "div", "span", "table", "td", "th", "tr", "a", "img")
	relaxedPolicy.AllowAttrs("width", "height", "border", "cellpadding", "cellspacing").OnElements("table")
	relaxedPolicy.AllowAttrs("align", "valign", "colspan", "rowspan").OnElements("td", "th")
	relaxedPolicy.AllowAttrs("align").OnElements("tr")

	relaxedPolicy.AllowAttrs("class").Matching(regexp.MustCompile(`^[A-Za-z0-9_ -]{1,240}$`)).OnElements("div", "span", "table", "thead", "tbody", "tr", "td", "th")

	relaxedPolicy.AllowAttrs("bgcolor", "color").Globally()
	relaxedPolicy.AllowAttrs("align").OnElements("p", "div", "h1", "h2", "h3", "h4", "h5", "h6")

	relaxedPolicy.AllowElements("img")
	relaxedPolicy.AllowAttrs("src", "alt", "width", "height", "align").OnElements("img")

	relaxedPolicy.AllowElements("a")
	relaxedPolicy.AllowAttrs("href").OnElements("a")
	relaxedPolicy.RequireNoReferrerOnLinks(true)
	relaxedPolicy.AddTargetBlankToFullyQualifiedLinks(true)
	relaxedPolicy.RequireNoFollowOnLinks(true)

	relaxedPolicy.AllowElements("font")
	relaxedPolicy.AllowAttrs("size", "color", "face").OnElements("font")

	relaxedPolicy.RequireParseableURLs(true)
	relaxedPolicy.AllowURLSchemes("http", "https", "mailto", "cid")

	relaxedPolicy.SkipElementsContent("style", "script", "object", "embed", "iframe", "frame", "frameset")
}

// SanitizeHTML applies the same relaxed HTML policy used by inbound parsing.
func SanitizeHTML(htmlContent string) string { return sanitizeHTML(htmlContent) }

func sanitizeHTML(htmlContent string) string {
	if htmlContent == "" {
		return ""
	}

	sanitized := relaxedPolicy.Sanitize(htmlContent)
	if strings.Contains(htmlContent, "\r\n") && !strings.Contains(strings.ReplaceAll(htmlContent, "\r\n", ""), "\n") {
		sanitized = strings.ReplaceAll(strings.ReplaceAll(sanitized, "\r\n", "\n"), "\n", "\r\n")
	}
	return sanitized
}

// Sanitize Text
func sanitizeText(text string) string {
	return strictPolicy.Sanitize(text)
}

func users2String(users []*User) string {
	ret := ""
	for _, user := range users {
		if ret != "" {
			ret += ", "
		}
		ret += user.Build()
	}
	return ret
}

func (e *Email) BuildTo2String() string {
	return users2String(e.To)
}

func (e *Email) BuildCc2String() string {
	return users2String(e.Cc)
}

func (e *Email) BuildBcc2String() string {
	return users2String(e.Bcc)
}

func NewEmailFromModel(d models.Email) *Email {

	var To []*User
	json.Unmarshal([]byte(d.To), &To)

	var ReplyTo []*User
	json.Unmarshal([]byte(d.ReplyTo), &ReplyTo)

	var Sender *User
	json.Unmarshal([]byte(d.Sender), &Sender)

	var Bcc []*User
	json.Unmarshal([]byte(d.Bcc), &Bcc)

	var Cc []*User
	json.Unmarshal([]byte(d.Cc), &Cc)

	var Attachments []*Attachment
	json.Unmarshal([]byte(d.Attachments), &Attachments)

	ret := &Email{
		MessageId: cast.ToInt64(d.Id),
		MsgID:     d.MsgID,
		From: &User{
			Name:         d.FromName,
			EmailAddress: d.FromAddress,
		},
		To:          To,
		Subject:     d.Subject,
		Text:        []byte(d.Text.String),
		TextFormat:  d.TextFormat,
		HTML:        []byte(d.Html.String),
		Sender:      Sender,
		ReplyTo:     ReplyTo,
		Bcc:         Bcc,
		Cc:          Cc,
		Attachments: Attachments,
		Date:        d.SendDate.Format(time.RFC3339),
	}
	if d.Type == 0 {
		ret.Authentication = NewEmailAuthentication(d.SPFCheck == 1, d.DKIMCheck == 1)
	}
	return ret
}

func NewEmailFromReader(to []string, r io.Reader, size int) *Email {
	ret := &Email{}
	m, errRead := message.Read(r)
	if errRead != nil {
		log.Errorf("email解析错误！ Error %+v", errRead)
	}

	ret.Size = size
	if m != nil {
		ret.Headers = make(textproto.MIMEHeader)
		for _, key := range []string{"Auto-Submitted", "X-PMail-DSN"} {
			if values := m.Header.Values(key); len(values) > 0 {
				ret.Headers[key] = append([]string(nil), values...)
			}
		}
	}
	if m == nil {
		return ret
	}
	// Preserve the original Message-ID from the sender so it is stored and reused consistently.
	if mid := m.Header.Get("Message-Id"); mid != "" {
		ret.MsgID = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(mid), ">"), "<")
	}
	ret.From = buildUser(m.Header.Get("From"))

	smtpTo := buildUsers(to)

	ret.To = buildUsers(m.Header.Values("To"))

	ret.Bcc = []*User{}

	for _, user := range smtpTo {
		in := false
		for _, u := range ret.To {
			if u.EmailAddress == user.EmailAddress {
				in = true
				break
			}
		}
		if !in {
			ret.Bcc = append(ret.Bcc, user)
		}

	}

	ret.Cc = buildUsers(m.Header.Values("Cc"))
	ret.ReplyTo = buildUsers(m.Header.Values("Reply-To"))
	ret.Sender = buildUser(m.Header.Get("Sender"))
	if ret.Sender == nil {
		ret.Sender = ret.From
	}

	subject, _ := m.Header.Text("Subject")
	ret.Subject = strictPolicy.Sanitize(subject)

	sendTime, err := time.Parse(time.RFC1123Z, m.Header.Get("Date"))
	if err != nil {
		sendTime = time.Now()
	}
	ret.Date = sendTime.Format(time.RFC3339)
	_ = m.Walk(func(path []int, entity *message.Entity, err error) error {
		if len(path) == 0 {
			err = errRead
		}
		return formatContent(entity, ret, err)
	})
	if len(ret.Text) == 0 && len(ret.HTML) != 0 {
		ret.Text = []byte(HTMLText(string(ret.HTML)))
	}
	ret.TextFormat, _ = CodeMetadata(string(ret.Text), string(ret.HTML), ret.TextFormat)

	if ret.From != nil {
		ret.From.Name = strictPolicy.Sanitize(ret.From.Name)
		ret.From.EmailAddress = strictPolicy.Sanitize(ret.From.EmailAddress)
	}

	return ret
}

func formatContent(entity *message.Entity, ret *Email, walkErr error) error {
	if entity == nil {
		return nil
	}
	contentType, params, err := entity.Header.ContentType()
	if err != nil || strings.HasPrefix(contentType, "multipart/") {
		return nil
	}
	kind, _, _ := mime.ParseMediaType(entity.Header.Get("Content-Disposition"))
	isAttachment := kind == "attachment"
	body := readMIMEBody(entity.Body)
	label := params["charset"]
	if strings.HasPrefix(contentType, "text/") && label != "" && !message.IsUnknownCharset(walkErr) && utf8.Valid(body) {
		// go-message has already decoded declared text charsets.
		label = "utf-8"
	}
	addAttachment := func() {
		filename := getFileName(entity, params)
		if filename == "no_name_file" && patchMIME(contentType) {
			filename = "inline.patch"
		}
		ret.Attachments = append(ret.Attachments, &Attachment{Filename: sanitizeText(filename), ContentType: contentType, Content: body, Charset: label,
			ContentID: strings.Trim(strings.TrimSpace(entity.Header.Get("Content-Id")), "<>")})
	}
	if isAttachment {
		addAttachment()
		return nil
	}
	switch {
	case patchMIME(contentType):
		text, _ := DecodeText(body, label)
		if len(ret.Text) != 0 {
			ret.Text = append(ret.Text, '\n')
			ret.Text = append(ret.Text, text...)
		} else {
			ret.Text = []byte(text)
		}
		ret.TextFormat = "patch"
		addAttachment()
	case contentType == "text/plain", contentType == "text/markdown", contentType == "text/x-markdown":
		if ret.TextFormat == "patch" {
			break
		}
		text, _ := DecodeText(body, label)
		ret.TextFormat = "plain"
		if strings.EqualFold(params["format"], "flowed") {
			text = flowedText(text, strings.EqualFold(params["delsp"], "yes"))
			ret.TextFormat = "flowed"
		} else if contentType != "text/plain" {
			ret.TextFormat = "markdown"
		}
		ret.Text = []byte(text)
	case contentType == "text/html":
		text, _ := DecodeText(body, label)
		ret.HTML = []byte(sanitizeHTML(text))
	default:
		addAttachment()
	}
	return nil
}

func getFileName(entity *message.Entity, p map[string]string) string {
	fileName := p["name"]

	if fileName != "" {
		return fileName
	}

	contentDisposition := entity.Header.Get("Content-Disposition")
	if _, params, err := mime.ParseMediaType(contentDisposition); err == nil && params["filename"] != "" {
		return params["filename"]
	}

	filenameRegex := regexp.MustCompile(`filename\s*=\s*"?([^";]+)"?`)
	matches := filenameRegex.FindStringSubmatch(contentDisposition)

	if len(matches) >= 2 {
		return strings.Trim(strings.TrimSpace(matches[1]), `"`)
	}

	return "no_name_file"
}

func BuilderUser(str string) *User {
	return buildUser(str)
}

var emailAddressRe = regexp.MustCompile(`<(.*@.*)>`)

func buildUser(str string) *User {
	str = strings.TrimSpace(str)
	if str == "" {
		return &User{}
	}

	user := &User{}

	addr, err := mail.ParseAddress(str)
	if err == nil {
		user.EmailAddress = strings.TrimSpace(addr.Address)

		name := strings.TrimSpace(addr.Name)
		if name != "" {
			decoder := mime.WordDecoder{}
			if decoded, err := decoder.Decode(name); err == nil {
				name = decoded
			}
			user.Name = strictPolicy.Sanitize(name)
		}
		return user
	}

	matched := emailAddressRe.FindStringSubmatch(str)
	if len(matched) == 2 {
		user.EmailAddress = strings.TrimSpace(matched[1])
		namePart := strings.ReplaceAll(str, matched[0], "")
		namePart = strings.Trim(strings.TrimSpace(namePart), "\"")

		decoder := mime.WordDecoder{}
		if decoded, err := decoder.Decode(strings.ReplaceAll(namePart, "\"", "")); err == nil {
			user.Name = strictPolicy.Sanitize(strings.TrimSpace(decoded))
		} else {
			user.Name = strictPolicy.Sanitize(strings.TrimSpace(namePart))
		}
	} else {
		user.EmailAddress = strictPolicy.Sanitize(str)
	}

	return user
}

func buildUsers(strs []string) []*User {
	var ret []*User
	for _, line := range strs {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Split(line, ",")
		for _, part := range parts {
			if u := buildUser(strings.TrimSpace(part)); u != nil {
				ret = append(ret, u)
			}
		}
	}
	return ret
}

func (e *Email) ForwardBuildBytes(ctx *context.Context, sender *models.User, forwardAddress string, senderAddresses ...string) []byte {
	senderEmailAddress := ""
	if len(senderAddresses) > 0 {
		senderEmailAddress = senderAddresses[0]
	}
	if senderEmailAddress == "" {
		senderEmailAddress = sender.Account + "@" + domain.DefaultName()
	}
	var b bytes.Buffer

	forwardUser := buildUser(forwardAddress)
	to := []*mail.Address{{Name: forwardUser.Name, Address: forwardUser.EmailAddress}}

	senderAddress := []*mail.Address{{Name: sender.Name, Address: senderEmailAddress}}
	// Create our mail header
	var h mail.Header
	h.SetDate(time.Now())
	h.SetAddressList("From", senderAddress)
	h.SetAddressList("Sender", senderAddress)
	h.SetAddressList("To", to)
	if e.From != nil && e.From.EmailAddress != "" {
		h.SetAddressList("Reply-To", []*mail.Address{{Name: e.From.Name, Address: e.From.EmailAddress}})
		h.Set("X-Original-From", e.From.Build())
	}
	h.Set("X-Forwarded-By", senderEmailAddress)
	h.Set("X-Forwarded-To", forwardUser.EmailAddress)
	h.SetText("Subject", e.Subject)
	h.SetMessageID(GenerateMsgID(domain.DefaultName()))
	if e.MsgID != "" {
		h.Set("References", fmt.Sprintf("<%s>", e.MsgID))
		h.Set("In-Reply-To", fmt.Sprintf("<%s>", e.MsgID))
	}

	// Create a new mail writer
	mw, err := mail.CreateWriter(&b, h)
	if err != nil {
		log.WithContext(ctx).Fatal(err)
	}

	// Create a text part
	tw, err := mw.CreateInline()
	if err != nil {
		log.WithContext(ctx).Fatal(err)
	}
	var th mail.InlineHeader
	th.Set("Content-Type", "text/plain")
	w, err := tw.CreatePart(th)
	if err != nil {
		log.Fatal(err)
	}
	io.WriteString(w, string(e.Text))
	w.Close()

	var html mail.InlineHeader
	html.Set("Content-Type", "text/html")
	w, err = tw.CreatePart(html)
	if err != nil {
		log.Fatal(err)
	}
	io.WriteString(w, string(e.HTML))
	w.Close()

	tw.Close()

	// Create an attachment
	for _, attachment := range e.Attachments {
		var ah mail.AttachmentHeader
		ah.Set("Content-Type", attachment.ContentType)
		ah.SetFilename(attachment.Filename)
		w, err = mw.CreateAttachment(ah)
		if err != nil {
			log.WithContext(ctx).Fatal(err)
			continue
		}
		w.Write(attachment.Content)
		w.Close()
	}

	mw.Close()

	if instance == nil {
		return b.Bytes()
	}

	// dkim 签名后返回
	return sign(b.String(), senderEmailAddress)
}

func (e *Email) BuildPart(ctx *context.Context, loc []int) []byte {
	if len(loc) == 0 {
		return nil
	}

	// 处理顶层 part (part 1 = alternative, part 2+ = attachments)
	if len(loc) == 1 {
		partIdx := loc[0]
		if partIdx == 1 {
			// Part 1 是 alternative，不能直接获取，需要获取子部分
			return nil
		}
		// Part 2, 3, ... 是附件
		attachIdx := partIdx - 2
		if attachIdx >= 0 && attachIdx < len(e.Attachments) {
			encoded := base64.StdEncoding.EncodeToString(e.Attachments[attachIdx].Content)
			encoded += "\r\n"
			return []byte(encoded)
		}
		return nil
	}

	// 处理 alternative 的子部分 (1.1, 1.2)
	if loc[0] == 1 && len(loc) >= 2 {
		subIdx := loc[1]

		// 根据 BODYSTRUCTURE 的构建顺序：先 text，后 html
		// 如果只有一个存在，那个就是 1.1
		hasText := len(e.Text) > 0
		hasHtml := len(e.HTML) > 0

		if hasText && hasHtml {
			// 两者都有: 1.1=text, 1.2=html
			if subIdx == 1 {
				encoded := base64.StdEncoding.EncodeToString(e.Text)
				encoded += "\r\n"
				return []byte(encoded)
			}
			if subIdx == 2 {
				encoded := base64.StdEncoding.EncodeToString(e.HTML)
				encoded += "\r\n"
				return []byte(encoded)
			}
		} else if hasText {
			// 只有 text: 1.1=text
			if subIdx == 1 {
				encoded := base64.StdEncoding.EncodeToString(e.Text)
				encoded += "\r\n"
				return []byte(encoded)
			}
		} else if hasHtml {
			// 只有 html: 1.1=html
			if subIdx == 1 {
				encoded := base64.StdEncoding.EncodeToString(e.HTML)
				encoded += "\r\n"
				return []byte(encoded)
			}
		}
	}

	return nil
}

func (e *Email) BuildBytes(ctx *context.Context, dkim bool) []byte {
	var b bytes.Buffer

	from := []*mail.Address{{Name: e.From.Name, Address: e.From.EmailAddress}}
	to := []*mail.Address{}
	for _, user := range e.To {
		to = append(to, &mail.Address{
			Name:    user.Name,
			Address: user.EmailAddress,
		})
	}

	// Create our mail header
	var h mail.Header
	if e.Date != "" {
		t, err := time.Parse(time.RFC3339, e.Date)
		if err != nil {
			// Keep compatibility with Date values stored before timezone offsets were preserved.
			t, err = time.ParseInLocation(time.DateTime, e.Date, time.Local)
		}
		if err != nil {
			log.WithContext(ctx).Errorf("Time Error ! Err:%+v", err)
			h.SetDate(time.Now())
		} else {
			h.SetDate(t)
		}
	} else {
		h.SetDate(time.Now())
	}
	if e.MsgID != "" {
		h.SetMessageID(e.MsgID)
	} else {
		h.SetMessageID(fmt.Sprintf("%d@%s", e.MessageId, domain.DefaultName()))
	}
	h.SetAddressList("From", from)
	h.SetAddressList("Sender", from)
	h.SetAddressList("To", to)
	h.SetText("Subject", e.Subject)
	if len(e.Cc) != 0 {
		cc := []*mail.Address{}
		for _, user := range e.Cc {
			cc = append(cc, &mail.Address{
				Name:    user.Name,
				Address: user.EmailAddress,
			})
		}
		h.SetAddressList("Cc", cc)
	}
	for key, values := range e.Headers {
		for _, value := range values {
			h.Add(key, value)
		}
	}

	// Create a new mail writer
	mw, err := mail.CreateWriter(&b, h)
	if err != nil {
		log.WithContext(ctx).Fatal(err)
	}

	// Create a text part
	tw, err := mw.CreateInline()
	if err != nil {
		log.WithContext(ctx).Fatal(err)
	}

	if len(e.Text) > 0 || len(e.HTML) == 0 {
		var th mail.InlineHeader
		th.Header.Set("Content-Transfer-Encoding", "base64")
		th.SetContentType("text/plain", map[string]string{"charset": "UTF-8"})
		part, err := tw.CreatePart(th)
		if err != nil {
			log.Fatal(err)
		}
		_, _ = part.Write(e.Text)
		_ = part.Close()
	}

	// Never put raw text/plain in a text/html part: code such as <script>
	// is literal data and must not become markup when a message is rebuilt.
	if len(e.HTML) > 0 {
		var body mail.InlineHeader
		body.SetContentType("text/html", map[string]string{"charset": "UTF-8"})
		body.Header.Set("Content-Transfer-Encoding", "base64")
		part, err := tw.CreatePart(body)
		if err != nil {
			log.Fatal(err)
		}
		_, _ = part.Write(e.HTML)
		_ = part.Close()
	}
	tw.Close()

	// Create an attachment
	for _, attachment := range e.Attachments {
		var ah mail.AttachmentHeader
		ah.Set("Content-Type", attachment.ContentType)
		ah.SetFilename(attachment.Filename)
		w, err := mw.CreateAttachment(ah)
		if err != nil {
			log.WithContext(ctx).Fatal(err)
			continue
		}
		w.Write(attachment.Content)
		w.Close()
	}

	mw.Close()

	if dkim {
		// dkim 签名后返回
		return sign(b.String(), e.From.EmailAddress)
	}
	return b.Bytes()
}
