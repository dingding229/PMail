package translation

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
)

const sampleHTML = `<div style="padding:8px"><h1>Weekly <b>report</b></h1>` +
	`<p>Hello <span style="color:red">team</span>, here is the update.</p>` +
	`<p><img src="/api/attachments/5.0" alt="chart"><br>Click <a href="https://example.com/x">here</a> to open it.</p>` +
	`<pre><code>go test ./...
exit 0</code></pre><p>Run <code>make build</code> first.</p>` +
	`<table><tr><td>Total</td><td>$1,200</td></tr></table>` +
	`<p translate="no">PMail Pro</p></div>`

func TestHTMLSegmentsKeepStructure(t *testing.T) {
	d := parseHTMLSegments(sampleHTML)
	if d == nil {
		t.Fatal("no segments")
	}
	var got []string
	for _, s := range d.segments {
		got = append(got, s.text)
	}
	want := []string{"Weekly report", "Hello team, here is the update.", "Click here to open it.", "Run", "first.", "Total"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("segments = %q", got)
	}
	src, n, cut := d.source(1000)
	if n != len(want) || cut || src != strings.Join(want, "\n") {
		t.Fatalf("source = %q %d %v", src, n, cut)
	}
	out := d.render("每周报告\n大家好，以下是更新。\n点击打开。\n运行\n首先。\n合计", n)
	for _, keep := range []string{
		`<img src="/api/attachments/5.0" alt="chart"/>`,
		"<pre><code>go test ./...\nexit 0</code></pre>",
		"<code>make build</code>",
		`<a href="https://example.com/x">here</a>`,
		"<td>$1,200</td>",
		`<p translate="no">PMail Pro</p>`,
		`<div style="padding:8px">`,
		"每周报告", "大家好，以下是更新。", "<td>合计</td>",
	} {
		if !strings.Contains(out, keep) {
			t.Errorf("missing %q in %s", keep, out)
		}
	}
	for _, gone := range []string{"Weekly", "Hello", "update."} {
		if strings.Contains(out, gone) {
			t.Errorf("untranslated %q in %s", gone, out)
		}
	}
}

func TestHTMLSegmentsMismatchAndBudget(t *testing.T) {
	d := parseHTMLSegments(sampleHTML)
	if out := d.render("only one line", 6); out != "" {
		t.Fatal("line count mismatch must fall back to plain text")
	}
	d = parseHTMLSegments(sampleHTML)
	src, n, cut := d.source(len("Weekly report") + 1 + len("Hello team, here is the update."))
	if n != 2 || !cut || strings.Count(src, "\n") != 1 {
		t.Fatalf("budget: %q %d %v", src, n, cut)
	}
	out := d.render("A\nB", n)
	if !strings.Contains(out, "Click") || !strings.Contains(out, ">A") {
		t.Fatalf("partial render: %s", out)
	}
	for _, empty := range []string{"", "  ", "<p>123 — 456</p>", "<pre>code only</pre>"} {
		if parseHTMLSegments(empty) != nil {
			t.Errorf("%q should have no segments", empty)
		}
	}
}

func TestTranslateHTMLMail(t *testing.T) {
	s, _, a, _, mail := fixture(t)
	ctx := t.Context()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.Contains(q, "<") || strings.Contains(q, "go test") {
			t.Errorf("markup or code sent for translation: %q", q)
		}
		googleReply(w, strings.ToUpper(q))
	}))
	t.Cleanup(server.Close)
	s.end.google = server.URL
	if _, err := SetPolicy(ctx, PolicyPatch{WebEnabled: ptr(true), MaxChars: ptr(5000), DailyLimit: ptr(10)}); err != nil {
		t.Fatal(err)
	}
	req := TranslateRequest{ID: mail.Id, TargetLang: "zh-CN", HTML: sampleHTML}
	got, err := s.Translate(ctx, a, "en", req)
	if err != nil || got.HTML == "" || !strings.Contains(got.HTML, "HELLO TEAM") || !strings.Contains(got.HTML, `<img src="/api/attachments/5.0"`) || !strings.Contains(got.HTML, "go test ./...") {
		t.Fatalf("html translation: %v %q", err, got.HTML)
	}
	cached, err := s.Translate(ctx, a, "en", req)
	if err != nil || !cached.Cached || cached.HTML != got.HTML {
		t.Fatalf("cached html: %v", err)
	}
	plain, err := s.Translate(ctx, a, "en", TranslateRequest{ID: mail.Id, TargetLang: "zh-CN"})
	if err != nil || plain.HTML != "" || plain.Cached {
		t.Fatalf("plain layout must not reuse html cache: %v %+v", err, plain.Cached)
	}
}

func TestHTMLSegmentsEscapeProviderText(t *testing.T) {
	d := parseHTMLSegments("<p>Hello</p>")
	out := d.render(`<img src=x onerror=alert(1)><script>alert(1)</script>`, 1)
	if strings.Contains(out, "<img") || strings.Contains(out, "<script") || !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("provider text must be escaped: %s", out)
	}
}

func TestHTMLSegmentsKeepSentenceStyles(t *testing.T) {
	src := `<p style="background-color:#eef4ff">The workshop begins at nine. <b>Bring your notebook.</b> <span style="color:#a32622">Coffee is available.</span> This is <b>very</b> important.</p>`
	d := parseHTMLSegments(src)
	var got []string
	for _, s := range d.segments {
		got = append(got, s.text)
	}
	want := "The workshop begins at nine.|Bring your notebook.|Coffee is available.|This is very important."
	if strings.Join(got, "|") != want {
		t.Fatalf("segments = %q", got)
	}
	out := d.render("工作坊九点开始。\n带上笔记本。\n提供咖啡。\n这非常重要。", len(got))
	for _, keep := range []string{"<b>带上笔记本。</b>", `<span style="color:#a32622">提供咖啡。</span>`, "工作坊九点开始。"} {
		if !strings.Contains(out, keep) {
			t.Errorf("missing %q in %s", keep, out)
		}
	}
}

func TestSanitizerKeepsNoTranslateMarkers(t *testing.T) {
	clean := parsemail.SanitizeHTML(`<p translate="no">Keep A</p><p class="notranslate">Keep B</p><p translate="evil">x</p><p class="evil">y</p>`)
	if !strings.Contains(clean, `translate="no"`) || !strings.Contains(clean, `class="notranslate"`) || strings.Contains(clean, `translate="evil"`) {
		t.Fatalf("sanitized: %s", clean)
	}
	d := parseHTMLSegments(clean)
	if d == nil || len(d.segments) != 2 {
		t.Fatalf("protected paragraphs must not be segments: %+v", d)
	}
}

func TestTargetLanguageDefaultsToServerLanguage(t *testing.T) {
	t.Setenv("PMAIL_DEFAULT_LANG", "")
	if got, _ := targetLanguage("", "", ""); got != "zh-CN" {
		t.Fatalf("empty account language should use the server default, got %q", got)
	}
	t.Setenv("PMAIL_DEFAULT_LANG", "en")
	if got, _ := targetLanguage("", "", ""); got != "en" {
		t.Fatalf("got %q", got)
	}
	if got, _ := targetLanguage("", "ja", "en"); got != "ja" {
		t.Fatalf("preference must win, got %q", got)
	}
}

func TestAvailableReadsBodyForHeaderOnlyMail(t *testing.T) {
	s, e, a, _, mail := fixture(t)
	_ = s
	t.Setenv("PMAIL_DEFAULT_LANG", "")
	if _, err := SetPolicy(t.Context(), PolicyPatch{WebEnabled: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	zh := models.Email{Subject: "Weekly", Text: sql.NullString{String: "你好，这是一封中文邮件，请查收附件中的报告。", Valid: true}}
	if _, err := e.Insert(&zh); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.UserEmail{EmailID: zh.Id, UserID: a}); err != nil {
		t.Fatal(err)
	}
	got := Available(t.Context(), a, zh.Id, "", models.Email{Subject: zh.Subject})
	if !got.TranslationAvailable || got.NeedsTranslation {
		t.Fatalf("Chinese body must not need translation: %+v", got)
	}
	got = Available(t.Context(), a, mail.Id, "", models.Email{Subject: mail.Subject})
	if !got.NeedsTranslation {
		t.Fatalf("English body needs translation: %+v", got)
	}
}
