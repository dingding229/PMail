package compose

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"xorm.io/xorm"
)

const SignatureSize = 64 << 10
const SignatureLimit = 20

type Signature struct {
	ID        int    `json:"id,omitempty"`
	Name      string `json:"name"`
	HTML      string `json:"html"`
	Addresses []int  `json:"address_ids"`
	Default   bool   `json:"is_default"`
}

var imageData = regexp.MustCompile(`(?i)^data:(image/(?:png|jpeg|gif|webp));base64,([a-z0-9+/=]+)$`)
var colorStyle = regexp.MustCompile(`(?i)^(?:#[0-9a-f]{3,8}|[a-z]{1,24}|rgba?\([0-9.,%\s]+\)|hsla?\([0-9.,%\s]+\))$`)
var alignStyle = regexp.MustCompile(`^(?:left|right|center|justify|start|end)$`)
var safeClasses = regexp.MustCompile(`^(?:(?:language|lang)-[A-Za-z0-9_+#.-]{1,32}|hljs|highlight|sourceCode)$`)
var signaturePolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements(strings.Fields("p br div span h1 h2 h3 h4 h5 h6 strong b em i u s del mark sub sup a blockquote hr ul ol li input pre code table thead tbody tfoot tr th td img")...)
	p.AllowURLSchemes("http", "https", "mailto", "data")
	p.RequireParseableURLs(true)
	p.RequireNoFollowOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("target", "rel").OnElements("a")
	p.AllowAttrs("src", "alt").OnElements("img")
	p.AllowAttrs("type", "checked").OnElements("input")
	p.AllowAttrs("data-type", "data-checked").OnElements("ul", "li")
	p.AllowAttrs("data-image-placeholder").OnElements("span")
	p.AllowAttrs("rowspan", "colspan").Matching(regexp.MustCompile(`^[1-9][0-9]{0,2}$`)).OnElements("td", "th")
	p.AllowAttrs("class").Matching(safeClasses).OnElements("pre", "code")
	p.AllowStyles("color", "background-color").Matching(colorStyle).Globally()
	p.AllowStyles("text-align").Matching(alignStyle).Globally()
	p.AllowStyles("font-size").Matching(regexp.MustCompile(`^(?:1[0-9]|2[0-8]|3[0-2])px$`)).Globally()
	p.AllowStyles("font-weight").Matching(regexp.MustCompile(`^(?:normal|bold|bolder|lighter|[1-9]00)$`)).Globally()
	p.AllowStyles("font-style").Matching(regexp.MustCompile(`^(?:normal|italic|oblique)$`)).Globally()
	p.AllowStyles("text-decoration").Matching(regexp.MustCompile(`^(?:none|underline|line-through|underline line-through|line-through underline)$`)).Globally()
	return p
}()

func SanitizeSignature(raw string) string {
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(raw), root)
	if err != nil {
		return ""
	}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	var walk func(*html.Node)
	walk = func(parent *html.Node) {
		for n := parent.FirstChild; n != nil; {
			next := n.NextSibling
			if n.Type == html.ElementNode {
				switch n.Data {
				case "script", "style", "svg", "xml", "iframe", "object", "audio", "video", "form":
					parent.RemoveChild(n)
				case "img":
					src, label := "", "外部图片"
					for _, a := range n.Attr {
						if a.Key == "alt" && strings.TrimSpace(a.Val) != "" {
							label = a.Val
						}
						if a.Key == "src" {
							src = a.Val
						}
					}
					if m := imageData.FindStringSubmatch(src); len(m) != 3 {
						replaceImage(parent, n, label)
					} else if b, err := base64.StdEncoding.DecodeString(m[2]); err != nil || raster(b) != strings.ToLower(m[1]) {
						replaceImage(parent, n, label)
					} else {
						n.Attr = []html.Attribute{{Key: "src", Val: src}, {Key: "alt", Val: label}}
					}
				case "a":
					attrs := []html.Attribute{{Key: "rel", Val: "noopener noreferrer"}}
					for _, a := range n.Attr {
						v := strings.TrimSpace(a.Val)
						lower := strings.ToLower(v)
						if a.Key == "href" && (strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "mailto:")) {
							attrs = append(attrs, html.Attribute{Key: "href", Val: v})
						}
					}
					n.Attr = attrs
					walk(n)
				case "input":
					valid := false
					for _, a := range n.Attr {
						if a.Key == "type" && a.Val == "checkbox" {
							valid = true
						}
					}
					if !valid {
						parent.RemoveChild(n)
					}
				default:
					walk(n)
				}
			} else if n.Type == html.CommentNode {
				parent.RemoveChild(n)
			}
			n = next
		}
	}
	walk(root)
	var out bytes.Buffer
	for n := root.FirstChild; n != nil; n = n.NextSibling {
		_ = html.Render(&out, n)
	}
	// Output is bounded independently because escaping/attributes may expand.
	return signaturePolicy.Sanitize(out.String())
}
func replaceImage(parent, n *html.Node, label string) {
	if len(label) > 512 {
		label = strings.ToValidUTF8(label[:512], "")
	}
	repl := &html.Node{Type: html.ElementNode, Data: "span", DataAtom: atom.Span, Attr: []html.Attribute{{Key: "data-image-placeholder", Val: "true"}}}
	repl.AppendChild(&html.Node{Type: html.TextNode, Data: "[图片：" + label + "]"})
	parent.InsertBefore(repl, n)
	parent.RemoveChild(n)
}
func fromModel(s models.Signature) Signature {
	out := Signature{ID: s.ID, Name: s.Name, HTML: s.HTML, Default: s.Default, Addresses: []int{}}
	_ = json.Unmarshal([]byte(s.Addresses), &out.Addresses)
	if out.Addresses == nil {
		out.Addresses = []int{}
	}
	return out
}
func Signatures(ctx context.Context, uid int) ([]Signature, error) {
	var rows []models.Signature
	if err := db.Instance.Context(ctx).Where("user_id=?", uid).Asc("id").Find(&rows); err != nil {
		return nil, err
	}
	out := make([]Signature, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromModel(r))
	}
	return out, nil
}
func SaveSignature(ctx context.Context, uid, id int, s Signature) (Signature, error) {
	s.Name = strings.TrimSpace(s.Name)
	if len(s.HTML) > SignatureSize || len(s.Addresses) > 100 || !utf8.ValidString(s.Name) || utf8.RuneCountInString(s.Name) > 100 || s.Name == "" {
		return s, ErrSignature
	}
	for _, c := range s.Name {
		if unicode.IsControl(c) || unicode.In(c, unicode.Cf) {
			return s, ErrSignature
		}
	}
	s.HTML = SanitizeSignature(s.HTML)
	if len(s.HTML) > SignatureSize {
		return s, ErrSignature
	}
	s.Addresses = slices.Clone(s.Addresses)
	slices.Sort(s.Addresses)
	s.Addresses = slices.Compact(s.Addresses)
	if s.Addresses == nil {
		s.Addresses = []int{}
	}
	data, _ := json.Marshal(s.Addresses)
	var row models.Signature
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if _, err := quota.Lock(tx, uid); err != nil {
			return err
		}
		if id > 0 {
			ok, err := tx.ID(id).Where("user_id=?", uid).Exist(&models.Signature{})
			if err != nil {
				return err
			}
			if !ok {
				return ErrNotFound
			}
		} else {
			n, err := tx.Where("user_id=?", uid).Count(&models.Signature{})
			if err != nil {
				return err
			}
			if n >= SignatureLimit {
				return ErrSignatureLimit
			}
		}
		if len(s.Addresses) > 0 {
			n, err := tx.Where("user_id=? AND status=?", uid, models.AddressActive).In("id", s.Addresses).Count(&models.MailboxAddress{})
			if err != nil {
				return err
			}
			if int(n) != len(s.Addresses) {
				return ErrSignature
			}
		}
		if s.Default {
			if _, err := tx.Table(&models.Signature{}).Where("user_id=? AND is_default=?", uid, true).Update(map[string]any{"is_default": false}); err != nil {
				return err
			}
		}
		row = models.Signature{ID: id, UserID: uid, Name: s.Name, HTML: s.HTML, Addresses: string(data), Default: s.Default}
		var err error
		if id == 0 {
			_, err = tx.Insert(&row)
		} else {
			_, err = tx.ID(id).Where("user_id=?", uid).Cols("name", "html", "addresses", "is_default").Update(&row)
		}
		return err
	})
	return fromModel(row), err
}
func DeleteSignature(ctx context.Context, uid, id int) error {
	n, err := db.Instance.Context(ctx).ID(id).Where("user_id=?", uid).Delete(&models.Signature{})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// SelectSignature is the server mirror of U7's deterministic selection, when
// a caller needs it. Sending never appends a second signature to editor HTML.
func SelectSignature(rows []Signature, addressID int) *Signature {
	var def *Signature
	for _, r := range rows {
		if slices.Contains(r.Addresses, addressID) {
			return &r
		}
		if def == nil && r.Default {
			def = &r
		}
	}
	return def
}
