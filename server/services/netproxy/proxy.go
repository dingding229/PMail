package netproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/utils/seal"
	"golang.org/x/net/idna"
)

var ErrInvalid = errors.New("invalid proxy settings")

type Fields struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
}
type Input struct {
	Fields
	Password *string `json:"password,omitempty"`
}
type Stored struct {
	Fields
	Ciphertext string `json:"password_ciphertext,omitempty"`
}
type View struct {
	Fields
	PasswordSet bool `json:"password_set"`
}

func (s Stored) View() *View {
	if s.Type == "" {
		return nil
	}
	return &View{s.Fields, s.Ciphertext != ""}
}

func Safe(s string, lim int) bool {
	return len(s) <= lim && utf8.ValidString(s) && !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) || unicode.In(r, unicode.Cf) })
}

func NormalizeHost(raw string) (string, error) {
	if raw == "" || !Safe(raw, 253) || strings.ContainsAny(raw, " /\\@?#%") {
		return "", ErrInvalid
	}
	if a, err := netip.ParseAddr(raw); err == nil && a.Zone() == "" {
		return a.String(), nil
	}
	h, err := idna.Lookup.ToASCII(raw)
	if err != nil || len(h) > 253 {
		return "", ErrInvalid
	}
	for label := range strings.SplitSeq(strings.TrimSuffix(h, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrInvalid
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return "", ErrInvalid
			}
		}
	}
	return strings.ToLower(h), nil
}

func (f Fields) URL(password string) (*url.URL, error) {
	if f.Type != "http" && f.Type != "https" && f.Type != "socks5" || f.Port < 1 || f.Port > 65535 || !Safe(f.Username, 255) || !Safe(password, 512) {
		return nil, ErrInvalid
	}
	host, err := NormalizeHost(strings.TrimSuffix(strings.TrimPrefix(f.Host, "["), "]"))
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(f.Host, "[]") && !(strings.HasPrefix(f.Host, "[") && strings.HasSuffix(f.Host, "]") && strings.Contains(host, ":")) {
		return nil, ErrInvalid
	}
	u := &url.URL{Scheme: f.Type, Host: net.JoinHostPort(host, strconv.Itoa(f.Port))}
	if password != "" {
		u.User = url.UserPassword(f.Username, password)
	} else if f.Username != "" {
		u.User = url.User(f.Username)
	}
	if len(u.String()) > 1024 {
		return nil, ErrInvalid
	}
	return u, nil
}

// ParseLegacy accepts the historic optional trailing slash and implicit ports.
func ParseLegacy(raw string) (*Input, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if !Safe(raw, 1024) || strings.ContainsAny(raw, "?#") {
		return nil, ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Hostname() == "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return nil, ErrInvalid
	}
	port := map[string]int{"http": 80, "https": 443, "socks5": 1080}[u.Scheme]
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
		if err != nil {
			return nil, ErrInvalid
		}
	}
	p := Input{Fields: Fields{Type: u.Scheme, Host: u.Hostname(), Port: port}}
	password := ""
	if u.User != nil {
		p.Username = u.User.Username()
		password, _ = u.User.Password()
	}
	p.Password = &password
	if _, err = p.Fields.URL(password); err != nil {
		return nil, err
	}
	return &p, nil
}

// Patch supports one legacy API version, including null to clear a proxy.
func Patch(raw json.RawMessage, clear, legacy bool) (*Input, bool, error) {
	raw = bytes.TrimSpace(raw)
	if clear {
		if len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
			return nil, false, ErrInvalid
		}
		return nil, true, nil
	}
	if len(raw) == 0 {
		return nil, false, nil
	}
	if bytes.Equal(raw, []byte("null")) {
		return nil, true, nil
	}
	if raw[0] == '"' && legacy {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, false, ErrInvalid
		}
		v, err := ParseLegacy(s)
		return v, v == nil, err
	}
	var p Input
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF {
		return nil, false, ErrInvalid
	}
	return &p, false, nil
}

const namespace = "pmail/netproxy/v1"

func (s Stored) URL(field string) (*url.URL, error) {
	if s.Type == "" {
		return nil, nil
	}
	pass, err := seal.Decode(namespace, field, s.Ciphertext)
	if err != nil {
		return nil, err
	}
	return s.Fields.URL(pass)
}

func Apply(s Stored, p *Input, clear bool, field string) (Stored, error) {
	if clear {
		return Stored{}, nil
	}
	if p == nil {
		return s, nil
	}
	pass := ""
	var err error
	if p.Password != nil {
		pass = *p.Password
	} else {
		pass, err = seal.Decode(namespace, field, s.Ciphertext)
		if err != nil {
			return Stored{}, err
		}
	}
	u, err := p.Fields.URL(pass)
	if err != nil {
		return Stored{}, err
	}
	encrypted := s.Ciphertext
	if p.Password != nil {
		encrypted, err = seal.Encode(namespace, field, pass)
		if err != nil {
			return Stored{}, err
		}
	}
	f := p.Fields
	f.Host = u.Hostname()
	return Stored{f, encrypted}, nil
}
