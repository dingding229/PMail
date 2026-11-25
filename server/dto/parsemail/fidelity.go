package parsemail

import (
	"bytes"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Mail HTML keeps its <style> sheets, CSS backgrounds and srcset images so
// newsletters look as sent. External images are still never loaded directly:
// the reader replaces them with placeholders or routes them through the
// image proxy, and the mail frame's CSP only allows same-origin images.
// Because same-origin requests carry the user's session, every URL kept here
// must be absolute (http, https, protocol-relative made https) or a data:
// image; relative URLs could make the reader call this server's own API.

const maxMailCSS = 128 << 10

var (
	cssComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssEscape   = regexp.MustCompile(`\\[0-9A-Fa-f]{1,6}\s?`)
	cssAtRule   = regexp.MustCompile(`(?i)@(?:import|charset|namespace)[^;{}]*`)
	cssImageSet = regexp.MustCompile(`(?i)(?:-webkit-)?image-set\((?:[^()]|\([^()]*\))*\)`)
	// Same rule as inline styles: no positioning that can lay content over
	// the rest of the mail (fake buttons, hidden warnings).
	cssOverlay  = regexp.MustCompile(`(?i)(^|[;{\s])(?:position|z-index|inset|top|right|bottom|left)\s*:[^;{}]*`)
	cssDanger   = regexp.MustCompile(`(?i)expression\s*\(|behavior\s*:|-moz-binding|javascript:|vbscript:`)
	safeCSSURL  = regexp.MustCompile(`^(?i:https?://)[^\s"'()<>\\]+$`)
	dataImage   = regexp.MustCompile(`^(?i:data:image/(?:png|gif|jpe?g|webp|bmp);base64,)[A-Za-z0-9+/=]+$`)
)

// absoluteURL returns u as an absolute http(s) URL, or "" when it is not one.
func absoluteURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	if len(u) <= 2048 && safeCSSURL.MatchString(u) {
		return u
	}
	return ""
}

// cleanCSS makes a style sheet or style attribute safe to keep. Escapes are
// removed because CSS lets "\75rl(" stand for "url(".
func cleanCSS(css string) string {
	css = cssEscape.ReplaceAllString(css, "")
	css = strings.NewReplacer("<", "", "\\", "", "\x00", "").Replace(css)
	css = cssComment.ReplaceAllString(css, "")
	css = cssAtRule.ReplaceAllString(css, "")
	css = cssImageSet.ReplaceAllString(css, "none")
	css = cssDanger.ReplaceAllString(css, "")
	css = cssOverlay.ReplaceAllString(css, "$1")
	css = cleanURLs(css)
	if len(css) > maxMailCSS {
		css = css[:strings.LastIndex(css[:maxMailCSS], "}")+1]
	}
	return strings.TrimSpace(css)
}

// cleanURLs rewrites every url(...) to an absolute or data: image URL, or
// to "none". It scans instead of matching a pattern so that malformed calls
// such as url(a('b')) cannot leave a relative URL behind.
func cleanURLs(css string) string {
	var out strings.Builder
	lower := strings.ToLower(css)
	i := 0
	for {
		j := strings.Index(lower[i:], "url(")
		if j < 0 {
			out.WriteString(css[i:])
			return out.String()
		}
		out.WriteString(css[i : i+j])
		k := i + j + 4
		depth, quote := 1, byte(0)
		end := -1
		for p := k; p < len(css); p++ {
			c := css[p]
			switch {
			case quote != 0:
				if c == quote {
					quote = 0
				}
			case c == '"' || c == '\'':
				quote = c
			case c == '(':
				depth++
			case c == ')':
				depth--
			}
			if depth == 0 {
				end = p
				break
			}
		}
		if end < 0 {
			// Unterminated: drop the rest rather than guess.
			out.WriteString("none")
			return out.String()
		}
		target := strings.Trim(strings.TrimSpace(css[k:end]), `"'`)
		switch {
		case len(target) <= 200000 && dataImage.MatchString(target):
			out.WriteString(`url("` + target + `")`)
		case absoluteURL(target) != "":
			out.WriteString(`url("` + absoluteURL(target) + `")`)
		default:
			out.WriteString("none")
		}
		i = end + 1
	}
}

// cleanSrcset keeps only absolute candidates of an srcset attribute.
func cleanSrcset(v string) string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		fields := strings.Fields(part)
		if len(fields) == 0 || len(fields) > 2 {
			continue
		}
		u := absoluteURL(fields[0])
		if u == "" {
			continue
		}
		if len(fields) == 2 {
			if !srcsetDescriptor.MatchString(fields[1]) {
				continue
			}
			u += " " + fields[1]
		}
		out = append(out, u)
	}
	return strings.Join(out, ", ")
}

var srcsetDescriptor = regexp.MustCompile(`^[0-9]{1,5}(?:\.[0-9]{1,3})?[wx]$`)

// extractStyles removes <style> elements from src and returns their cleaned
// CSS. It also cleans style attributes and makes protocol-relative image
// URLs absolute, so the sanitizer that runs next can keep them.
func extractStyles(src string) (string, []string) {
	lower := strings.ToLower(src)
	if !strings.Contains(lower, "<style") && !strings.Contains(lower, "style=") && !strings.Contains(lower, "srcset") && !strings.Contains(lower, "//") {
		return src, nil
	}
	z := html.NewTokenizer(strings.NewReader(src))
	var out bytes.Buffer
	var sheets []string
	inStyle := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() != io.EOF {
				return src, nil
			}
			break
		}
		raw := z.Raw()
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			if tok.Data == "style" {
				inStyle = tt == html.StartTagToken
				continue
			}
			changed := false
			for i := range tok.Attr {
				a := &tok.Attr[i]
				switch strings.ToLower(a.Key) {
				case "style":
					if v := cleanCSS(a.Val); v != a.Val {
						a.Val, changed = v, true
					}
				case "srcset":
					if v := cleanSrcset(a.Val); v != a.Val {
						a.Val, changed = v, true
					}
				case "src", "background":
					if strings.HasPrefix(strings.TrimSpace(a.Val), "//") {
						a.Val, changed = absoluteURL(a.Val), true
					}
				}
			}
			if changed {
				out.WriteString(tok.String())
				continue
			}
		case html.TextToken:
			if inStyle {
				if css := cleanCSS(string(z.Text())); css != "" {
					sheets = append(sheets, css)
				}
				continue
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "style" {
				inStyle = false
				continue
			}
		}
		out.Write(raw)
	}
	return out.String(), sheets
}

// appendStyles puts the style sheets after the body. A leading <style>
// would move into <head> when the reader parses the fragment and be lost.
func appendStyles(body string, sheets []string) string {
	if len(sheets) == 0 {
		return body
	}
	css := strings.Join(sheets, "\n")
	if len(css) > maxMailCSS {
		css = css[:strings.LastIndex(css[:maxMailCSS], "}")+1]
	}
	if css == "" {
		return body
	}
	return body + "<style>" + css + "</style>"
}
