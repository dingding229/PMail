package parsemail

import (
	"bytes"
	"io"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var (
	hiddenStyle = regexp.MustCompile(`display:none|visibility:hidden|opacity:0(?:[;!]|$)|max-(?:width|height):[01]px`)
	styleSize   = map[string]*regexp.Regexp{
		"width":  regexp.MustCompile(`(?:^|;)width:([0-9.]+)px`),
		"height": regexp.MustCompile(`(?:^|;)height:([0-9.]+)px`),
	}
)

// stripTrackingImages removes external images that exist only to report when
// and where a mail is opened: hidden images and images of at most 3x3 pixels.
// It runs before the sanitizer, which drops the styles that reveal them.
func stripTrackingImages(src string) string {
	if !strings.Contains(strings.ToLower(src), "<img") {
		return src
	}
	z := html.NewTokenizer(strings.NewReader(src))
	var out bytes.Buffer
	changed := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() != io.EOF {
				return src
			}
			break
		}
		raw := z.Raw()
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			if name, hasAttr := z.TagName(); string(name) == "img" && hasAttr {
				attrs := map[string]string{}
				for {
					k, v, more := z.TagAttr()
					attrs[strings.ToLower(string(k))] = string(v)
					if !more {
						break
					}
				}
				if trackingImage(attrs) {
					changed = true
					continue
				}
			}
		}
		out.Write(raw)
	}
	if !changed {
		return src
	}
	return out.String()
}

func trackingImage(attrs map[string]string) bool {
	s := strings.ToLower(strings.TrimSpace(attrs["src"]))
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "//") {
		return false
	}
	style := strings.ToLower(strings.Join(strings.Fields(attrs["style"]), ""))
	if hiddenStyle.MatchString(style) {
		return true
	}
	size := func(dim string) (float64, bool) {
		v := strings.TrimSuffix(strings.TrimSpace(attrs[dim]), "px")
		if m := styleSize[dim].FindStringSubmatch(style); v == "" && m != nil {
			v = m[1]
		}
		n, err := strconv.ParseFloat(v, 64)
		return n, err == nil && n >= 0
	}
	w, hasW := size("width")
	h, hasH := size("height")
	tinyW, tinyH := hasW && w <= 3, hasH && h <= 3
	return (tinyW && (!hasH || tinyH)) || (tinyH && !hasW)
}
