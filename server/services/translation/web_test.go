package translation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
