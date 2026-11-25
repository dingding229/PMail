package parsemail

import (
	"strings"
	"testing"
)

func TestSanitizeKeepsNewsletterStyles(t *testing.T) {
	in := `<html><head><style type="text/css">
/* brand */ .nav a{color:#666;text-decoration:none} .hero{background:url(https://cdn.example.com/hero.png) no-repeat;padding:20px}
@import url("https://evil.example/x.css");
.bad{background:url(/api/email/del?id=1)} .bad2{background-image:url( '//cdn.example.com/a.png' )}
.esc{background:\75rl(/api/x)} .set{background-image:image-set("/api/x" 1x)}
</style></head><body>
<table class="wrap" width="600" align="center" bgcolor="#fff" background="//cdn.example.com/bg.png"><tr><td class="nav" style="border-bottom:1px solid #eee;line-height:24px;font-family:Arial">` +
		`<a href="https://example.com">产品</a></td></tr>` +
		`<tr><td><img src="//cdn.example.com/logo.png" srcset="//cdn.example.com/logo@2x.png 2x, /api/attachments/1.0 3x" width="120">` +
		`<div style="background-image:url(/api/email/del);position:fixed;z-index:99">x</div></td></tr></table></body></html>`
	out := SanitizeHTML(in)
	for _, want := range []string{
		`.nav a{color:#666;text-decoration:none}`,
		`url("https://cdn.example.com/hero.png")`,
		`url("https://cdn.example.com/a.png")`,
		`class="wrap"`, `class="nav"`,
		`src="https://cdn.example.com/logo.png"`,
		`srcset="https://cdn.example.com/logo@2x.png 2x"`,
		`background="https://cdn.example.com/bg.png"`,
		`border-bottom: 1px solid #eee`, `line-height: 24px`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, bad := range []string{`="/api`, `url("/`, `url(/`, "@import", "evil.example", "position", "z-index", "image-set", "<head", "\\"} {
		if strings.Contains(out, bad) {
			t.Errorf("unsafe %q kept in\n%s", bad, out)
		}
	}
	for _, part := range strings.Split(out, "url(")[1:] {
		if !strings.HasPrefix(part, `"https://`) && !strings.HasPrefix(part, `"data:image/`) {
			t.Errorf("unsafe url(%s", part[:min(40, len(part))])
		}
	}
	if !strings.HasSuffix(out, "</style>") || strings.Count(out, "<style>") != 1 {
		t.Errorf("styles must follow the body once:\n%s", out)
	}
	if again := SanitizeHTML(out); again != out {
		t.Errorf("not idempotent:\n%s\n---\n%s", out, again)
	}
}

func TestCleanCSSBreakouts(t *testing.T) {
	for _, in := range []string{
		`a{b:c}</style><script>alert(1)</script>`,
		`a{background:url("x)y")}`,
		`a{background:url(`,
		`a{background:URL(  javascript:alert(1)  )}`,
		`a{background:ur/**/l(/api/x)}`,
	} {
		out := cleanCSS(in)
		if strings.Contains(out, "<") || strings.Contains(strings.ToLower(out), "javascript") || strings.Contains(out, "/api") || strings.Contains(out, "x)y") {
			t.Errorf("cleanCSS(%q) = %q", in, out)
		}
	}
}

func TestHTMLTextIgnoresStyles(t *testing.T) {
	text := HTMLText(SanitizeHTML(`<style>.a{color:red}</style><p class="a">Hello</p>`))
	if strings.Contains(text, "color") || !strings.Contains(text, "Hello") {
		t.Fatalf("text = %q", text)
	}
}
