package netproxy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
)

func TestFieldsAndLegacyValidation(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, host, want string
		port                       int
	}{
		{"IPv4", "http", "203.0.113.9", "http://203.0.113.9:80", 80},
		{"IPv6", "socks5", "[2001:db8::1]", "socks5://[2001:db8::1]:1080", 1080},
		{"domain and upper port", "https", "Proxy.Example", "https://proxy.example:65535", 65535},
		{"bad scheme", "socks4", "proxy.example", "", 1080},
		{"zero port", "http", "proxy.example", "", 0},
		{"bad port", "http", "proxy.example", "", 65536},
		{"path", "http", "proxy.example/path", "", 80},
		{"query", "http", "proxy.example?name", "", 80},
		{"fragment", "http", "proxy.example#name", "", 80},
		{"zone", "http", "fe80::1%lo0", "", 80},
		{"overlong", "http", strings.Repeat("a", 254), "", 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, err := (Fields{Type: tc.protocol, Host: tc.host, Port: tc.port}).URL("")
			if tc.want == "" {
				if err == nil {
					t.Fatal("accepted invalid proxy")
				}
				return
			}
			if err != nil || u.String() != tc.want {
				t.Fatal("incorrect normalized URL")
			}
		})
	}
	for _, raw := range []string{"http://proxy.example/path", "https://proxy.example?", "http://proxy.example#", "ftp://proxy.example", "http://proxy.example:99999", "http://" + strings.Repeat("a", 1020)} {
		if _, err := ParseLegacy(raw); err == nil {
			t.Fatal("accepted invalid legacy URL")
		}
	}
	v, clear, err := Patch(json.RawMessage(`"https://proxy.example/"`), false, true)
	if err != nil || clear || v.Port != 443 {
		t.Fatal("legacy URL not converted")
	}
	if _, _, err := Patch(json.RawMessage(`"https://proxy.example"`), false, false); err == nil {
		t.Fatal("unexpected legacy support")
	}
	if _, clear, err := Patch(nil, true, true); err != nil || !clear {
		t.Fatal("clear did not apply")
	}
	if _, _, err := Patch(json.RawMessage(`{"type":"http","extra":1}`), false, true); err == nil {
		t.Fatal("accepted unknown field")
	}
}

func TestStoredPasswordOmissionAndRedaction(t *testing.T) {
	old := config.Instance
	config.Instance = &config.Config{SecretKey: "fixture-only"}
	t.Cleanup(func() { config.Instance = old })
	pass := "fixture-proxy-password:/@"
	p := Input{Fields: Fields{Type: "https", Host: "proxy.example", Port: 443, Username: "tester"}, Password: &pass}
	s, err := Apply(Stored{}, &p, false, "test")
	if err != nil || s.Ciphertext == "" || strings.Contains(s.Ciphertext, pass) {
		t.Fatal("password not sealed")
	}
	p.Password = nil
	p.Port = 8080
	s, err = Apply(s, &p, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.URL("test")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := u.User.Password(); got != pass || u.Port() != "8080" {
		t.Fatal("patch lost password or port")
	}
	view, _ := json.Marshal(s.View())
	if strings.Contains(string(view), pass) || strings.Contains(string(view), s.Ciphertext) || !strings.Contains(string(view), `"password_set":true`) {
		t.Fatal("proxy view not redacted")
	}
	empty := ""
	p.Password = &empty
	s, err = Apply(s, &p, false, "test")
	if err != nil || s.View().PasswordSet {
		t.Fatal("explicit empty password did not clear")
	}
}
