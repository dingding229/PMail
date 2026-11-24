package translation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func TestGoogleAndEdgeChunkingAndTokenRefresh(t *testing.T) {
	for _, provider := range []string{"google_web", "edge_web"} {
		t.Run(provider, func(t *testing.T) {
			s := newService()
			p := defaults()
			p.Provider = provider
			now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
			var clock atomic.Int64
			clock.Store(now.Unix())
			s.now = func() time.Time { return time.Unix(clock.Load(), 0) }
			var auth, calls atomic.Int32
			var token atomic.Value
			var rateLimit atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/auth" {
					if r.Method != http.MethodGet {
						t.Error("wrong token request method")
					}
					auth.Add(1)
					payload := base64.RawURLEncoding.EncodeToString(fmt.Appendf(nil, `{"exp":%d}`, clock.Load()+60))
					tk := "header." + payload + ".signature"
					token.Store(tk)
					_, _ = w.Write([]byte(tk))
					return
				}
				calls.Add(1)
				if rateLimit.Load() {
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				q := r.URL.Query()
				var text string
				if provider == "google_web" {
					if r.Method != http.MethodGet || q.Get("tl") != "zh-TW" || q.Get("client") != "gtx" {
						t.Error("invalid Google request format")
					}
					text = q.Get("q")
				} else {
					var body []map[string]string
					if r.Method != http.MethodPost || q.Get("to") != "zh-Hant" || q.Get("api-version") != "3.0" || r.Header.Get("Authorization") != "Bearer "+token.Load().(string) || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 {
						t.Error("invalid Edge request format")
						w.WriteHeader(400)
						return
					}
					text = body[0]["Text"]
				}
				if n := utf8.RuneCountInString(text); n < 1 || n > chunkChars {
					t.Error("chunk size out of bounds")
				}
				if provider == "google_web" {
					googleReply(w, strings.ToUpper(text))
					return
				}
				_ = json.NewEncoder(w).Encode([]any{map[string]any{"detectedLanguage": map[string]string{"language": "en"}, "translations": []map[string]string{{"text": strings.ToUpper(text)}}}})
			}))
			t.Cleanup(srv.Close)
			s.end.google, s.end.edge, s.end.edgeAuth = srv.URL+"/translate", srv.URL+"/translate", srv.URL+"/auth"
			text := strings.Repeat("a", 4140) + "\r\n\r\n" + strings.Repeat("Ｂ", 4790) + " b"
			for i := 0; i < 3; i++ {
				if i == 2 {
					clock.Add(60)
				}
				_, out, detected, err := s.translateWeb(t.Context(), p, "", text, "zh-TW")
				if err != nil || out != strings.ToUpper(text) || detected != "en" {
					t.Fatalf("chunk reconstruction failed: %v", err)
				}
			}
			if calls.Load() < 9 {
				t.Fatal("long text not split")
			}
			if provider == "edge_web" && auth.Load() != 2 {
				t.Fatal("Edge credential was not cached and refreshed")
			}
			rateLimit.Store(true)
			if _, _, _, err := s.translateWeb(t.Context(), p, "", "hello", "zh-TW"); !errors.Is(err, ErrUpstreamRate) {
				t.Fatalf("429 misclassified: %v", err)
			}
		})
	}
}

func TestEdgeTokenWaitRespectsContext(t *testing.T) {
	s := newService()
	// Simulate an outstanding refresh without a network request.
	s.edge.refreshing = make(chan struct{})
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := s.edgeToken(ctx, nil, nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("stalled refresh did not honor deadline: %v", err)
	}
}

func TestDeepLAndLibreProviderFormatsAndRedaction(t *testing.T) {
	s, _, _, _, _ := fixture(t)
	for _, tc := range []struct{ name, path, tier, key string }{
		{"deepl", "/free", "auto", "test-web-key:fx"},
		{"deepl", "/pro", "pro", "test-web-key:fx"},
		{"libretranslate", "/srv/translate", "auto", "test-libre-key"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var in struct {
					Text                      []string `json:"text"`
					TargetLang                string   `json:"target_lang"`
					Q, Source, Target, Format string
					APIKey                    string `json:"api_key"`
				}
				if r.Method != http.MethodPost || r.URL.Path != tc.path || r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&in) != nil {
					t.Error("wrong provider request")
					w.WriteHeader(400)
					return
				}
				if tc.name == "deepl" {
					if len(in.Text) != 1 || in.TargetLang != "ZH-HANS" || r.Header.Get("Authorization") != "DeepL-Auth-Key "+tc.key || in.APIKey != "" {
						t.Error("wrong DeepL body or authentication")
					}
					_, _ = w.Write([]byte(`{"translations":[{"text":"译文","detected_source_language":"EN"}]}`))
				} else {
					if in.Q == "" || in.Source != "auto" || in.Target != "zh" || in.Format != "text" || in.APIKey != tc.key {
						t.Error("wrong LibreTranslate body or authentication")
					}
					_, _ = w.Write([]byte(`{"translatedText":"译文","detectedLanguage":{"language":"en"}}`))
				}
			}))
			defer srv.Close()
			s.end.deepl, s.end.deeplFree = srv.URL+"/pro", srv.URL+"/free"
			v, err := SetPolicy(t.Context(), PolicyPatch{WebEnabled: ptr(true), Provider: &tc.name, DeepLTier: &tc.tier, APIKey: &tc.key, URL: ptr(srv.URL + "/srv")})
			if err != nil || !v.APIKeySet {
				t.Fatalf("set provider: %v", err)
			}
			out, _ := json.Marshal(v)
			if strings.Contains(string(out), tc.key) || strings.Contains(string(out), `"api_key":`) {
				t.Fatal("web policy exposed secret")
			}
			result, err := s.TestWeb(t.Context())
			if err != nil || result.Text != "译文" || result.Provider != tc.name {
				t.Fatalf("test web: %v", err)
			}
		})
	}
}
