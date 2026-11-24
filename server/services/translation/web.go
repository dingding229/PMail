package translation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/Jinnrry/pmail/services/netproxy"
	"github.com/Jinnrry/pmail/utils/seal"
)

type endpoints struct {
	google, googleBackup    string
	bingPage, bingTranslate string
	myMemory                string
	deepl, deeplFree        string
}

const deepLEndpoint = "https://api.deepl.com/v2/translate"
const deepLFreeEndpoint = "https://api-free.deepl.com/v2/translate"

func proxyIdentity(u *url.URL) [32]byte {
	if u == nil {
		return [32]byte{}
	}
	return sha256.Sum256([]byte(u.String()))
}

func (s *Service) webProxy(ctx context.Context, p policyRecord) (*url.URL, error) {
	switch p.ProxyMode {
	case "telegram":
		return s.systemTelegram(ctx)
	case "custom":
		return p.Proxy.URL(webProxyField)
	}
	return nil, nil
}

func (s *Service) webText(ctx context.Context, c *http.Client, p policyRecord, key, text, lang string, proxy *url.URL) (string, string, error) {
	var out strings.Builder
	source, requestSource := "", ""
	if p.Provider == "mymemory" {
		requestSource = detectWebLanguage(text)
	}
	for part, sep := range chunksText(text, p.Provider) {
		if p.Provider == "mymemory" {
			head := strings.TrimLeftFunc(part, unicode.IsSpace)
			out.WriteString(part[:len(part)-len(head)])
			part = head
		}
		if strings.TrimSpace(part) == "" {
			out.WriteString(part)
			out.WriteString(sep)
			continue
		}
		translated, detected, err := s.webChunk(ctx, c, p, key, part, requestSource, lang, proxy)
		if err != nil {
			return "", "", err
		}
		if source == "" {
			source = detected
		}
		if out.Len()+len(translated)+len(sep) > responseLimit {
			return "", "", ErrFormat
		}
		out.WriteString(translated)
		out.WriteString(sep)
	}
	return out.String(), source, nil
}
func withQuery(raw string, params url.Values) string {
	u, _ := url.Parse(raw)
	u.RawQuery = params.Encode()
	return u.String()
}

func (s *Service) webChunk(ctx context.Context, c *http.Client, p policyRecord, key, text, source, lang string, proxy *url.URL) (string, string, error) {
	var data []byte
	var err error
	switch p.Provider {
	case "google_web":
		return s.googleChunk(ctx, c, text, lang, proxy)
	case "edge_web":
		return s.bingChunk(ctx, c, text, lang, proxy)
	case "mymemory":
		return s.myMemoryChunk(ctx, c, text, source, lang, p.MyMemoryEmail)
	case "deepl":
		if key == "" {
			return "", "", ErrInvalid
		}
		endpoint := s.end.deepl
		if p.DeepLTier == "free" || (p.DeepLTier == "auto" && strings.HasSuffix(key, ":fx")) {
			endpoint = s.end.deeplFree
		}
		target := strings.ToUpper(lang)
		switch target {
		case "EN":
			target = "EN-US"
		case "ZH-CN":
			target = "ZH-HANS"
		case "ZH-TW":
			target = "ZH-HANT"
		}
		data, _, err = exchange(ctx, c, http.MethodPost, endpoint, map[string]any{"text": []string{text}, "target_lang": target}, map[string]string{"Authorization": "DeepL-Auth-Key " + key})
		if err != nil {
			return "", "", err
		}
		var reply struct {
			Translations []struct {
				Text   string `json:"text"`
				Source string `json:"detected_source_language"`
			} `json:"translations"`
		}
		if json.Unmarshal(data, &reply) != nil || len(reply.Translations) != 1 || reply.Translations[0].Text == "" {
			return "", "", ErrFormat
		}
		return reply.Translations[0].Text, reply.Translations[0].Source, nil
	case "libretranslate":
		endpoint, err := baseURL(p.URL)
		if err != nil {
			return "", "", err
		}
		if !strings.HasSuffix(strings.TrimRight(endpoint.Path, "/"), "/translate") {
			endpoint = endpoint.JoinPath("translate")
		}
		target := lang
		if target == "zh-CN" {
			target = "zh"
		}
		body := map[string]any{"q": text, "source": "auto", "target": target, "format": "text"}
		if key != "" {
			body["api_key"] = key
		}
		data, _, err = exchange(ctx, c, http.MethodPost, endpoint.String(), body, nil)
		if err != nil {
			return "", "", err
		}
		var reply struct {
			Text     string `json:"translatedText"`
			Detected struct {
				Language string `json:"language"`
			} `json:"detectedLanguage"`
		}
		if json.Unmarshal(data, &reply) != nil || reply.Text == "" {
			return "", "", ErrFormat
		}
		return reply.Text, reply.Detected.Language, nil
	}
	return "", "", ErrInvalid
}

type WebTestResult struct {
	Subject  string `json:"subject"`
	Text     string `json:"text"`
	Provider string `json:"provider"`
	Latency  int64  `json:"latency_ms"`
}

func (s *Service) TestWeb(ctx context.Context) (WebTestResult, error) {
	p, err := policy(ctx)
	if err != nil {
		return WebTestResult{}, err
	}
	start := time.Now()
	sub, txt, _, err := s.translateWeb(ctx, p, "", "Hello, this is a PMail translation test.", "zh-CN")
	if err != nil {
		return WebTestResult{}, err
	}
	return WebTestResult{sub, txt, p.Provider, time.Since(start).Milliseconds()}, nil
}

func (s *Service) translateWeb(ctx context.Context, p policyRecord, sub, txt, lang string) (string, string, string, error) {
	proxy, err := s.webProxy(ctx, p)
	if err != nil {
		return "", "", "", ErrConnect
	}
	key, err := seal.Decode(namespace, "web/key", p.Key)
	if err != nil {
		return "", "", "", ErrProvider
	}
	client := httpClient(p.Timeout, proxy, s.dial)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.Timeout)*time.Second)
	defer cancel()
	// Translate the two fields separately so the body never ends up as a subject.
	a, detected, err := s.webText(ctx, client, p, key, sub, lang, proxy)
	if err != nil {
		return "", "", "", err
	}
	b, source, err := s.webText(ctx, client, p, key, txt, lang, proxy)
	if source == "" {
		source = detected
	}
	// A malformed/custom upstream can echo request fields in a successful response.
	if key != "" && (p.Provider == "deepl" || p.Provider == "libretranslate") &&
		(strings.Contains(a, key) || strings.Contains(b, key) || strings.Contains(source, key)) {
		return "", "", "", ErrFormat
	}
	if len(source) > 24 || !netproxy.Safe(source, 24) {
		source = ""
	}
	return a, b, source, err
}

func cacheKey(values ...any) string {
	data, _ := json.Marshal(values)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
