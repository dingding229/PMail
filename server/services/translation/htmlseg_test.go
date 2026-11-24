package translation

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
