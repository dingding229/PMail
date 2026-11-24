package parsemail

import (
	"strings"
	"testing"
)

func TestSanitizeDropsTrackingImages(t *testing.T) {
	in := `<p>Hi</p>` +
		`<img src="https://cdn.example/logo.png" width="120" height="40">` +
		`<img src="https://t.example/open?id=1" width="1" height="1">` +
		`<img src="https://t.example/h.gif" style="display: none">` +
		`<img src="https://t.example/w.gif" style="width:1px;height:1px">` +
		`<img src="https://t.example/o.gif" style="opacity:0">` +
		`<img src="cid:logo" width="1" height="1">` +
		`<img src="https://cdn.example/spacer.gif" width="1" height="40">`
	out := SanitizeHTML(in)
	if strings.Contains(out, "t.example") {
		t.Fatalf("tracking image kept: %s", out)
	}
	for _, keep := range []string{"cdn.example/logo.png", "cid:logo", "spacer.gif", "<p>Hi</p>"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("%s removed: %s", keep, out)
		}
	}
}
