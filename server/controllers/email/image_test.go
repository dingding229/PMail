package email

import "testing"

func TestURLInHTML(t *testing.T) {
	body := `<p><img src="https://t.example/p.gif?a=1&amp;b=2"><div style="background:url('//cdn.example/bg.png')"></div></p>`
	for target, want := range map[string]bool{
		"https://t.example/p.gif?a=1&b=2":     true,
		"https://t.example/p.gif?a=1&amp;b=2": true,
		"https://cdn.example/bg.png":          true,
		"https://evil.example/x.png":          false,
		"":                                    false,
	} {
		if got := URLInHTML(body, target); got != want {
			t.Errorf("%q: %v", target, got)
		}
	}
}
