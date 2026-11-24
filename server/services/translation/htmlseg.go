package translation

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// htmlLayoutVersion is part of the cache key so that translations stored by
// the plain-text layout are never reused as HTML segments.
const htmlLayoutVersion = "html-segments-v1"

// keep is never translated: code, preformatted text and anything that is
// not visible text. These elements also end the current text run.
var keep = map[atom.Atom]bool{
	atom.Pre: true, atom.Code: true, atom.Kbd: true, atom.Samp: true, atom.Tt: true, atom.Var: true,
	atom.Script: true, atom.Style: true, atom.Textarea: true, atom.Title: true, atom.Head: true,
	atom.Noscript: true, atom.Svg: true, atom.Math: true, atom.Template: true, atom.Select: true,
}

// inline elements are merged into one run so a sentence split by <b> or
// <span> is translated as a whole.
var inline = map[atom.Atom]bool{
	atom.A: true, atom.Abbr: true, atom.B: true, atom.Bdi: true, atom.Bdo: true, atom.Big: true,
	atom.Cite: true, atom.Data: true, atom.Del: true, atom.Dfn: true, atom.Em: true, atom.Font: true,
	atom.I: true, atom.Ins: true, atom.Label: true, atom.Mark: true, atom.Q: true, atom.S: true,
	atom.Small: true, atom.Span: true, atom.Strike: true, atom.Strong: true, atom.Sub: true,
	atom.Sup: true, atom.Time: true, atom.U: true, atom.Nobr: true, atom.Wbr: true,
}

type htmlSegment struct {
	nodes []*html.Node
	text  string
}

type htmlDoc struct {
	root     *html.Node
	segments []htmlSegment
}

// parseHTMLSegments splits a sanitized HTML body into translatable runs of
// visible text. It returns nil when the document has nothing to translate.
func parseHTMLSegments(src string) *htmlDoc {
	if strings.TrimSpace(src) == "" {
		return nil
	}
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(src), ctx)
	if err != nil {
		return nil
	}
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	d := &htmlDoc{root: root}
	var run []*html.Node
	flush := func() {
		if len(run) == 0 {
			return
		}
		var b strings.Builder
		for _, n := range run {
			b.WriteString(n.Data)
		}
		text := strings.Join(strings.Fields(b.String()), " ")
		if translatable(text) {
			d.segments = append(d.segments, htmlSegment{nodes: run, text: text})
		}
		run = nil
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				run = append(run, c)
			case html.ElementNode:
				if keep[c.DataAtom] || noTranslate(c) {
					flush()
					continue
				}
				if !inline[c.DataAtom] {
					flush()
					walk(c)
					flush()
					continue
				}
				walk(c)
			}
		}
	}
	walk(root)
	flush()
	if len(d.segments) == 0 {
		return nil
	}
	return d
}

func noTranslate(n *html.Node) bool {
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "translate":
			if strings.EqualFold(strings.TrimSpace(a.Val), "no") {
				return true
			}
		case "class":
			for _, c := range strings.Fields(a.Val) {
				if strings.EqualFold(c, "notranslate") {
					return true
				}
			}
		}
	}
	return false
}

// translatable skips runs without letters, such as prices, dates, separators
// and bare numbers, which keep their original form.
func translatable(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// source returns the text sent for translation, one segment per line, limited
// to budget runes. It also returns how many segments fit.
func (d *htmlDoc) source(budget int) (string, int, bool) {
	var b strings.Builder
	used := 0
	for i, s := range d.segments {
		n := utf8.RuneCountInString(s.text)
		if i > 0 {
			n++
		}
		if used+n > budget {
			return b.String(), i, true
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s.text)
		used += n
	}
	return b.String(), len(d.segments), false
}

// render puts translated lines back into the first count segments. It
// returns "" when the provider did not keep one line per segment.
func (d *htmlDoc) render(translated string, count int) string {
	lines := splitLines(translated)
	if count <= 0 || len(lines) != count || count > len(d.segments) {
		return ""
	}
	for i, line := range lines {
		d.segments[i].apply(line)
	}
	var b strings.Builder
	for c := d.root.FirstChild; c != nil; c = c.NextSibling {
		if html.Render(&b, c) != nil {
			return ""
		}
	}
	return b.String()
}

func splitLines(s string) []string {
	raw := strings.Split(strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// apply writes the translation into the longest text node of the run and
// clears the others. A link whose text was cleared keeps its original text
// so it stays visible and clickable.
func (s htmlSegment) apply(translated string) {
	best := 0
	for i, n := range s.nodes {
		if len(strings.TrimSpace(n.Data)) > len(strings.TrimSpace(s.nodes[best].Data)) {
			best = i
		}
	}
	first, last := s.nodes[0].Data, s.nodes[len(s.nodes)-1].Data
	lead := len(first) > 0 && first != strings.TrimLeftFunc(first, unicode.IsSpace)
	trail := len(last) > 0 && last != strings.TrimRightFunc(last, unicode.IsSpace)
	for i, n := range s.nodes {
		if i == best {
			continue
		}
		if a := linkAncestor(n, s.nodes[best]); a != nil && strings.TrimSpace(n.Data) != "" {
			continue
		}
		n.Data = ""
	}
	text := translated
	if lead {
		text = " " + text
	}
	if trail {
		text += " "
	}
	s.nodes[best].Data = text
}

// linkAncestor returns the <a> around n when best is not inside that link.
func linkAncestor(n, best *html.Node) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.DataAtom == atom.A {
			for q := best.Parent; q != nil; q = q.Parent {
				if q == p {
					return nil
				}
			}
			return p
		}
	}
	return nil
}
