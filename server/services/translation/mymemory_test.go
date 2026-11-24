package translation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestMyMemoryUTF8ChunkingOrderAndContact(t *testing.T) {
	s := testService()
	p := defaults()
	p.Provider = "mymemory"
	p.MyMemoryEmail = "contact+quota@example.test"
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		text := q.Get("q")
		if r.Method != http.MethodGet || q.Get("langpair") != "ja|zh-CN" || q.Get("de") != p.MyMemoryEmail || len(text) > 500 || text == "" {
			t.Error("wrong MyMemory request")
		}
		mu.Lock()
		requests = append(requests, text)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"responseStatus": "200", "responseData": map[string]string{"translatedText": "[" + text + "]"}})
	}))
	t.Cleanup(server.Close)
	s.end.myMemory = server.URL
	first := strings.Repeat("ね", 155) + "。"
	long := strings.Repeat("お", 270) + "🍎"
	txt := first + "\n" + long
	_, out, src, err := s.translateWeb(t.Context(), p, "", txt, "zh-CN")
	mu.Lock()
	defer mu.Unlock()
	if err != nil || src != "ja" || len(requests) != 3 || requests[0] != first || strings.Join(requests[1:], "") != long || out != "["+requests[0]+"]\n["+requests[1]+"]["+requests[2]+"]" {
		t.Fatalf("chunk/order/detection mismatch: %v", err)
	}
	if pair := strings.Join(requests, ""); strings.ContainsRune(pair, '\uFFFD') {
		t.Fatal("chunk split a UTF-8 character")
	}
}

func TestMyMemoryLanguageAndUpstreamRateSignals(t *testing.T) {
	for _, tc := range []struct{ sample, language string }{
		{"Hello World", "en"}, {"123 ?", "en"}, {"这是测试邮件", "zh-CN"}, {"日本語のテスト", "ja"}, {"한국어 예제", "ko"}, {"Проверка перевода", "ru"},
	} {
		if got := detectWebLanguage(tc.sample); got != tc.language {
			t.Errorf("detected %q instead of %q", got, tc.language)
		}
	}
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"quota", `{"quotaFinished":true}`, 200},
		{"quota_error_http", `{"quotaFinished":true}`, 403},
		{"numeric", `{"responseStatus":429}`, 200},
		{"quoted", `{"responseStatus":"429"}`, 200},
		{"warning", `{"responseStatus":200,"responseData":{"translatedText":"MYMEMORY WARNING: daily limit"}}`, 200},
		{"http", `{}`, 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService()
			p := defaults()
			p.Provider = "mymemory"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("langpair") != "en|zh-CN" || r.URL.Query().Has("de") {
					t.Error("wrong language or empty de was sent")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)
			s.end.myMemory = server.URL
			if _, _, _, err := s.translateWeb(t.Context(), p, "", "Hello translated world!", "zh-CN"); !errors.Is(err, ErrUpstreamRate) {
				t.Fatalf("rate signal returned: %v", err)
			}
		})
	}
}

func TestMyMemoryEmailSettings(t *testing.T) {
	fixture(t)
	ctx := t.Context()
	for _, bad := range []string{"plain", "name <someone@example.test>", "wrong@", "no-space@example.test\nheader", strings.Repeat("a", 250) + "@example.test"} {
		if _, err := SetPolicy(ctx, PolicyPatch{MyMemoryEmail: &bad}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid contact address accepted")
		}
	}
	cfg, err := SetPolicy(ctx, PolicyPatch{Provider: ptr("mymemory"), MyMemoryEmail: ptr("Contact+Quota@example.test")})
	if err != nil || cfg.MyMemoryEmail != "Contact+Quota@example.test" {
		t.Fatalf("save MyMemory policy: %v", err)
	}
	cfg, err = GetPolicy(ctx)
	if err != nil || cfg.MyMemoryEmail != "Contact+Quota@example.test" || cfg.APIKeySet {
		t.Fatal("contact address must be returned as a normal setting")
	}
	cfg, err = SetPolicy(ctx, PolicyPatch{MyMemoryEmail: ptr("")})
	if err != nil || cfg.MyMemoryEmail != "" {
		t.Fatal("explicit empty value did not clear optional field")
	}
}

func TestMyMemoryUsesConfiguredProxy(t *testing.T) {
	s := testService()
	p := defaults()
	p.Provider, p.ProxyMode = "mymemory", "telegram"
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() || r.URL.Host != "mymemory-provider.example.test" || r.URL.Query().Get("langpair") != "en|zh-CN" {
			t.Error("MyMemory was not routed through the configured proxy")
		}
		_, _ = w.Write([]byte(`{"responseStatus":200,"responseData":{"translatedText":"译文"}}`))
	}))
	t.Cleanup(proxy.Close)
	s.systemTelegram = func(context.Context) (*url.URL, error) { return url.Parse(proxy.URL) }
	s.end.myMemory = "http://mymemory-provider.example.test/get"
	_, out, _, err := s.translateWeb(t.Context(), p, "", "Hello!", "zh-CN")
	if err != nil || out != "译文" {
		t.Fatalf("MyMemory proxy: %v", err)
	}
}
