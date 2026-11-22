package parsemail

import (
	"bytes"
	"mime"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

var (
	patchEnvelope = regexp.MustCompile(`(?m)^From [0-9a-fA-F]{7,64} Mon Sep 17 00:00:00 2001\r?$`)
	patchHeaders  = regexp.MustCompile(`(?m)^--- [^\r\n]+\r?\n\+\+\+ [^\r\n]+\r?\n@@`)
	codeFence     = regexp.MustCompile(`(?m)^[ \t]{0,3}(` + "`{3,}" + `|~{3,})`)
)

func patchBody(text string) bool {
	return strings.HasPrefix(text, "diff --git ") || strings.Contains(text, "\ndiff --git ") ||
		patchEnvelope.MatchString(text) || patchHeaders.MatchString(text)
}

// CodeMetadata detects legacy rows too, but respects an explicit MIME format.
func CodeMetadata(text, bodyHTML, declared string) (format string, hasCode bool) {
	patch := declared == "patch" || patchBody(text)
	fence := codeFence.MatchString(text)
	switch {
	case patch:
		format = "patch"
	case fence || declared == "markdown":
		format = "markdown"
	case declared == "flowed":
		format = "flowed"
	default:
		format = "plain"
	}
	if patch || fence {
		return format, true
	}
	tokens := html.NewTokenizer(strings.NewReader(bodyHTML))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return format, false
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := tokens.TagName()
			if string(name) == "pre" || string(name) == "code" {
				return format, true
			}
		}
	}
}

// HTMLText uses text tokens, not a whitespace-collapsing tag stripper. This
// preserves code indentation and interprets entities exactly once.
func HTMLText(body string) string {
	var out strings.Builder
	var last byte
	write := func(s string) {
		if s != "" {
			out.WriteString(s)
			last = s[len(s)-1]
		}
	}
	tokens := html.NewTokenizer(strings.NewReader(body))
	for {
		tokenType := tokens.Next()
		switch tokenType {
		case html.ErrorToken:
			return strings.Trim(out.String(), "\r\n")
		case html.TextToken:
			write(string(tokens.Text()))
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			name, _ := tokens.TagName()
			switch string(name) {
			case "br":
				write("\n")
			case "p", "div", "pre", "li", "tr", "blockquote", "h1", "h2", "h3", "h4", "h5", "h6":
				if out.Len() > 0 && last != '\n' {
					write("\n")
				}
			case "td", "th":
				if tokenType == html.EndTagToken {
					write("\t")
				}
			}
		}
	}
}

func flowedText(raw string, delsp bool) string {
	if patchBody(raw) { // patch line endings carry meaning even with a bad format header
		return raw
	}
	newline := "\n"
	if strings.Contains(raw, "\r\n") {
		newline = "\r\n"
	}
	type line struct {
		depth int
		body  string
		soft  bool
		fixed bool
	}
	var lines []line
	var fence byte
	for _, s := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		var depth int
		for depth < len(s) && s[depth] == '>' {
			depth++
		}
		body := strings.TrimPrefix(s[depth:], " ") // RFC 3676 space-stuffing
		trimmed := strings.TrimLeft(body, " \t")
		marker := codeFence.MatchString(body)
		fixed := fence != 0 || marker || body == "-- " || strings.HasPrefix(body, " ") || strings.HasPrefix(body, "\t")
		if marker && fence == 0 {
			fence = trimmed[0]
		} else if marker && fence == trimmed[0] {
			fence = 0
		}
		lines = append(lines, line{depth, body, !fixed && strings.HasSuffix(body, " "), fixed})
	}
	var out strings.Builder
	for i := 0; i < len(lines); i++ {
		current := lines[i]
		if current.depth > 0 {
			out.WriteString(strings.Repeat(">", current.depth))
			if current.body != "" {
				out.WriteByte(' ')
			}
		}
		for current.soft && i+1 < len(lines) && lines[i+1].depth == current.depth && !lines[i+1].fixed && lines[i+1].body != "" {
			if delsp {
				out.WriteString(strings.TrimSuffix(current.body, " "))
			} else {
				out.WriteString(current.body)
			}
			i++
			current = lines[i]
		}
		out.WriteString(current.body)
		if i+1 < len(lines) {
			out.WriteString(newline)
		}
	}
	return out.String()
}

// DecodeText converts raw bytes not already decoded by the MIME reader. With
// no usable label, UTF-8 is preferred, then plausible Chinese encodings, then
// a Western single-byte fallback. Unlabelled legacy data is inherently ambiguous.
func DecodeText(body []byte, label string) (string, string) {
	label = strings.ToLower(strings.TrimSpace(label))
	if label != "" && label != "us-ascii" && label != "ascii" && label != "utf-8" && label != "utf8" {
		enc, err := ianaindex.MIME.Encoding(label)
		if err != nil || enc == nil {
			enc, _ = htmlindex.Get(label)
		}
		if enc != nil {
			if decoded, err := enc.NewDecoder().Bytes(body); err == nil && utf8.Valid(decoded) {
				return strings.TrimPrefix(string(decoded), "\ufeff"), label
			}
		}
	}
	if utf8.Valid(body) {
		return strings.TrimPrefix(string(body), "\ufeff"), "utf-8"
	}
	bestScore, best, bestLabel := 0, "", ""
	for _, candidate := range []struct {
		name string
		enc  encoding.Encoding
	}{{"gb18030", simplifiedchinese.GB18030}, {"big5", traditionalchinese.Big5}} {
		decoded, err := candidate.enc.NewDecoder().Bytes(body)
		if err != nil || bytes.Contains(decoded, []byte("\ufffd")) {
			continue
		}
		han, common := 0, 0
		for _, r := range string(decoded) {
			if unicode.Is(unicode.Han, r) {
				han++
				if strings.ContainsRune("的一是在不了有和人这這中大为為上个個国國我以要他時时来來用們们生到作地于於出就分对于對成会會可主没沒發发工后後用文件函數数测试測試代码碼你好世界郵邮件變量標標頭編編輯編輯傳傳送", r) {
					common++
				}
			}
		}
		if score := han + common*3; han > 0 && (han >= 2 || common > 0) && score > bestScore {
			bestScore, best, bestLabel = score, string(decoded), candidate.name
		}
	}
	if bestScore != 0 {
		return best, bestLabel
	}
	decoded, _ := charmap.Windows1252.NewDecoder().Bytes(body)
	return string(decoded), "windows-1252"
}

const PreviewLimit = 512 * 1024

type TextPreview struct {
	Filename  string `json:"filename"`
	Language  string `json:"language"`
	Charset   string `json:"charset"`
	Truncated bool   `json:"truncated"`
	Size      int    `json:"size"`
	Content   string `json:"content"`
}

var previewExtensions = map[string]string{
	".py": "python", ".js": "javascript", ".mjs": "javascript", ".jsx": "jsx", ".ts": "typescript", ".tsx": "tsx", ".go": "go", ".rs": "rust", ".c": "c", ".h": "c", ".cpp": "cpp", ".cc": "cpp", ".hpp": "cpp", ".java": "java", ".kt": "kotlin", ".cs": "csharp", ".rb": "ruby", ".php": "php", ".swift": "swift", ".lua": "lua", ".pl": "perl", ".r": "r", ".sh": "bash", ".bash": "bash", ".zsh": "bash", ".ps1": "powershell", ".sql": "sql", ".json": "json", ".jsonl": "json", ".yaml": "yaml", ".yml": "yaml", ".toml": "toml", ".ini": "ini", ".conf": "ini", ".xml": "xml", ".html": "html", ".htm": "html", ".css": "css", ".csv": "csv", ".tsv": "plaintext", ".md": "markdown", ".markdown": "markdown", ".diff": "diff", ".patch": "diff", ".txt": "plaintext", ".log": "plaintext", ".dockerfile": "dockerfile", ".vue": "vue",
}

var previewTypes = map[string]string{
	"application/json": "json", "application/ld+json": "json", "application/yaml": "yaml", "application/x-yaml": "yaml", "application/toml": "toml", "application/xml": "xml", "application/javascript": "javascript", "application/x-sh": "bash", "application/sql": "sql", "application/x-patch": "diff", "application/x-diff": "diff", "text/x-patch": "diff", "text/x-diff": "diff", "text/markdown": "markdown", "text/html": "html", "text/css": "css", "text/csv": "csv", "text/xml": "xml", "text/yaml": "yaml", "text/javascript": "javascript",
}

func patchMIME(media string) bool { return previewTypes[media] == "diff" }

func previewLanguage(att *Attachment) (string, string, bool) {
	media, params, _ := mime.ParseMediaType(att.ContentType)
	ext := strings.ToLower(filepath.Ext(att.Filename))
	lang, viaExt := previewExtensions[ext]
	if strings.EqualFold(filepath.Base(att.Filename), "Dockerfile") {
		lang, viaExt = "dockerfile", true
	}
	label := att.Charset
	if label == "" {
		label = params["charset"]
	}
	if byMIME := previewTypes[media]; byMIME != "" {
		return byMIME, label, true
	}
	if strings.HasPrefix(media, "text/") || (viaExt && (media == "" || media == "application/octet-stream")) {
		if lang == "" {
			lang = "plaintext"
		}
		return lang, label, true
	}
	return "", "", false
}

func Previewable(att *Attachment) bool {
	if att == nil {
		return false
	}
	if _, _, ok := previewLanguage(att); !ok {
		return false
	}
	for _, b := range att.Content {
		if (b < 0x20 && b != '\t' && b != '\n' && b != '\r' && b != '\f') || b == 0x7f {
			return false
		}
	}
	return true
}

func PreviewText(att *Attachment) (TextPreview, bool) {
	if !Previewable(att) {
		return TextPreview{}, false
	}
	lang, label, _ := previewLanguage(att)
	raw := att.Content
	truncated := len(raw) > PreviewLimit
	if truncated {
		// Decode a small lookahead so a multibyte rune crossing the input
		// boundary is not mistaken for a legacy charset or malformed suffix.
		raw = raw[:min(len(raw), PreviewLimit+4)]
		if utf8.Valid(att.Content) {
			for !utf8.Valid(raw) {
				raw = raw[:len(raw)-1]
			}
		}
	}
	text, charset := DecodeText(raw, label)
	if len(text) > PreviewLimit {
		end := PreviewLimit
		for !utf8.RuneStart(text[end]) {
			end--
		}
		text, truncated = text[:end], true
	}
	return TextPreview{Filename: att.Filename, Language: lang, Charset: charset, Truncated: truncated, Size: len(att.Content), Content: text}, true
}
