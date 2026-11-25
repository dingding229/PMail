package parsemail

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

func codeFixture(t *testing.T, name string) *Email {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name + ".eml")
	if err != nil {
		t.Fatal(err)
	}
	return NewEmailFromReader(nil, bytes.NewReader(raw), len(raw))
}
func TestMailCodeFixtures(t *testing.T) {
	for _, tc := range []struct {
		name, format, text string
		code               bool
	}{
		{"github-pr", "plain", "\t  fmt.Println(x)", true},
		{"git-format-patch", "patch", "\r\n+\tvar i", true},
		{"gbk-code", "plain", "你好 世界\r\n\tif (a < b)", false},
		{"flowed-code", "flowed", "One paragraph.\r\n\r\n    if x < 3: \r\n    \t  do_work()\r\n-- \r\n", false},
		{"markdown-fence", "markdown", "```go\r\n\t  x < y", true},
		{"code-attachments", "patch", "@@ -1 +1 @@", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			email := codeFixture(t, tc.name)
			format, code := CodeMetadata(string(email.Text), string(email.HTML), email.TextFormat)
			if format != tc.format || code != tc.code || !strings.Contains(string(email.Text), tc.text) {
				t.Fatalf("format=%s has_code=%v text=%q", format, code, email.Text)
			}
		})
	}
	html := string(codeFixture(t, "github-pr").HTML)
	for _, want := range []string{`class="language-go hljs"`, `class="sourceCode"`, `colspan="2"`, `&lt;`, `tab-size`, "\r\n\t  fmt"} {
		if !strings.Contains(html, want) {
			t.Errorf("sanitized HTML lacks %q: %q", want, html)
		}
	}
	if strings.Contains(html, "<script") || strings.Contains(html, "onclick") || strings.Contains(SanitizeHTML(`<pre class="danger" style="white-space:pre;background:url(javascript:x)">x</pre>`), "javascript") {
		t.Fatalf("unsafe markup survived: %q", html)
	}
	atts := codeFixture(t, "code-attachments").Attachments
	if len(atts) != 2 {
		t.Fatalf("attachments: %d", len(atts))
	}
	for i, want := range []string{"python", "diff"} {
		got, ok := PreviewText(atts[i])
		if !ok || got.Language != want || got.Size != len(atts[i].Content) {
			t.Fatalf("preview: %#v %v", got, ok)
		}
	}
}

func TestDecodeLegacyCharsetAndPreviewLimit(t *testing.T) {
	for _, sample := range []struct {
		value, label string
		bytes        []byte
	}{
		{"olá", "iso-8859-1", []byte{'o', 'l', 0xe1}},
		{"你好吗", "gb18030", mustEncode(t, simplifiedchinese.GB18030.NewEncoder().Bytes, "你好吗")},
		{"郵件測試", "big5", mustEncode(t, traditionalchinese.Big5.NewEncoder().Bytes, "郵件測試")},
		{"你好世界", "", mustEncode(t, simplifiedchinese.GB18030.NewEncoder().Bytes, "你好世界")},
	} {
		got, _ := DecodeText(sample.bytes, sample.label)
		if got != sample.value {
			t.Errorf("%s: %q != %q", sample.label, got, sample.value)
		}
	}
	att := &Attachment{Filename: "long.log", ContentType: "text/plain", Content: []byte(strings.Repeat("测试", PreviewLimit/6+10))}
	got, ok := PreviewText(att)
	if !ok || !got.Truncated || got.Size != len(att.Content) || len(got.Content) > PreviewLimit || !utf8.ValidString(got.Content) {
		t.Fatal("invalid truncation")
	}
	att.Content = append(att.Content, 0)
	if _, ok := PreviewText(att); ok {
		t.Fatal("NUL after the preview boundary was missed")
	}
}
func mustEncode(t *testing.T, fn func([]byte) ([]byte, error), in string) []byte {
	t.Helper()
	b, err := fn([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
