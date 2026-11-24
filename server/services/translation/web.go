package translation

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/services/netproxy"
	"github.com/Jinnrry/pmail/utils/seal"
)

const chunkChars = 4500
const googleEndpoint = "https://translate.googleapis.com/translate_a/single"
const edgeAuthEndpoint = "https://edge.microsoft.com/translate/auth"
const edgeEndpoint = "https://api-edge.cognitive.microsofttranslator.com/translate"
const deepLEndpoint = "https://api.deepl.com/v2/translate"
const deepLFreeEndpoint = "https://api-free.deepl.com/v2/translate"

type endpoints struct{ google, edgeAuth, edge, deepl, deeplFree string }
type edgeCredential struct {
	value    string
	expiry   time.Time
	proxyKey [32]byte
}
type edgeCache struct {
	sync.Mutex
	edgeCredential
	refreshing chan struct{}
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

// chunks keeps newline separators outside the upstream requests. Prefer the
// last paragraph boundary, then a line break, then a hard Unicode rune cut.
func chunks(s string, google bool, yield func(part, separator string) bool) {
	r := []rune(s)
	for len(r) > 0 {
		end := min(len(r), chunkChars)
		if google {
			// The unofficial Google endpoint receives q in the URL. Keep its escaped
			// value under the usual proxy request-line limit even for Unicode input.
			size := 0
			for i, c := range r[:end] {
				if c <= 127 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune(" -_.~", c)) {
					size++
				} else {
					size += 3 * utf8.RuneLen(c)
				}
				if size > 7000 {
					end = i
					break
				}
			}
		}
		if end < len(r) {
			line, paragraph := 0, 0
			for i := end - 1; i >= 0; i-- {
				if r[i] == '\n' {
					if line == 0 {
						line = i + 1
					}
					if i > 0 && r[i-1] == '\n' {
						paragraph = i + 1
						break
					}
				}
			}
			if paragraph >= chunkChars/2 {
				end = paragraph
			} else if line > 0 {
				end = line
			}
		}
		// Keep CRLF intact even if a boundary landed just before LF.
		if end < len(r) && r[end-1] == '\r' && r[end] == '\n' {
			end--
		}
		part := string(r[:end])
		r = r[end:]
		head := strings.TrimRight(part, "\r\n")
		if !yield(head, part[len(head):]) {
			return
		}
	}
}

func (s *Service) webText(ctx context.Context, c *http.Client, p policyRecord, key, text, lang string, proxy *url.URL) (string, string, error) {
	var out strings.Builder
	source := ""
	var err error
	for part, sep := range chunksText(text, p.Provider == "google_web") {
		if strings.TrimSpace(part) == "" {
			out.WriteString(part)
			out.WriteString(sep)
			continue
		}
		var translated, detected string
		translated, detected, err = s.webChunk(ctx, c, p, key, part, lang, proxy)
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
func chunksText(s string, google bool) func(func(string, string) bool) {
	return func(yield func(string, string) bool) { chunks(s, google, yield) }
}

func withQuery(raw string, params url.Values) string {
	u, _ := url.Parse(raw)
	u.RawQuery = params.Encode()
	return u.String()
}

func (s *Service) webChunk(ctx context.Context, c *http.Client, p policyRecord, key, text, lang string, proxy *url.URL) (string, string, error) {
	var data []byte
	var err error
	switch p.Provider {
	case "google_web":
		endpoint := withQuery(s.end.google, url.Values{"client": {"gtx"}, "sl": {"auto"}, "tl": {lang}, "dt": {"t"}, "q": {text}})
		data, _, err = exchange(ctx, c, http.MethodGet, endpoint, nil, nil)
		if err != nil {
			return "", "", err
		}
		var reply []json.RawMessage
		var rows [][]json.RawMessage
		if json.Unmarshal(data, &reply) != nil || len(reply) == 0 || json.Unmarshal(reply[0], &rows) != nil || len(rows) == 0 {
			return "", "", ErrFormat
		}
		var out strings.Builder
		for _, row := range rows {
			var line string
			if len(row) == 0 || json.Unmarshal(row[0], &line) != nil {
				return "", "", ErrFormat
			}
			out.WriteString(line)
		}
		var source string
		if len(reply) > 2 {
			_ = json.Unmarshal(reply[2], &source)
		}
		if out.Len() == 0 {
			return "", "", ErrFormat
		}
		return out.String(), source, nil
	case "edge_web":
		token, err := s.edgeToken(ctx, c, proxy)
		if err != nil {
			return "", "", err
		}
		target := lang
		if lang == "zh-CN" {
			target = "zh-Hans"
		} else if lang == "zh-TW" {
			target = "zh-Hant"
		}
		endpoint := withQuery(s.end.edge, url.Values{"api-version": {"3.0"}, "to": {target}})
		data, _, err = exchange(ctx, c, http.MethodPost, endpoint, []map[string]string{{"Text": text}}, map[string]string{"Authorization": "Bearer " + token})
		if err != nil {
			if err == ErrAuth {
				s.invalidateEdge(token)
			}
			return "", "", err
		}
		var reply []struct {
			Detected struct {
				Language string `json:"language"`
			} `json:"detectedLanguage"`
			Translations []struct {
				Text string `json:"text"`
			} `json:"translations"`
		}
		if json.Unmarshal(data, &reply) != nil || len(reply) != 1 || len(reply[0].Translations) != 1 || reply[0].Translations[0].Text == "" {
			return "", "", ErrFormat
		}
		return reply[0].Translations[0].Text, reply[0].Detected.Language, nil
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

func (s *Service) edgeToken(ctx context.Context, c *http.Client, proxy *url.URL) (string, error) {
	key := [32]byte{}
	if proxy != nil {
		key = sha256.Sum256([]byte(proxy.String()))
	}
	for {
		s.edge.Lock()
		if s.edge.value != "" && key == s.edge.proxyKey && s.now().Before(s.edge.expiry) {
			token := s.edge.value
			s.edge.Unlock()
			return token, nil
		}
		if ready := s.edge.refreshing; ready != nil {
			s.edge.Unlock()
			select {
			case <-ready:
				continue
			case <-ctx.Done():
				return "", transportError(ctx.Err())
			}
		}
		s.edge.refreshing = make(chan struct{})
		s.edge.Unlock()
		break
	}
	defer func() {
		s.edge.Lock()
		defer s.edge.Unlock()
		close(s.edge.refreshing)
		s.edge.refreshing = nil
	}()
	raw, _, err := exchange(ctx, c, http.MethodGet, s.end.edgeAuth, nil, nil)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	parts := strings.Split(token, ".")
	if len(parts) != 3 || !netproxy.Safe(token, 16384) || strings.ContainsAny(token, " \t") {
		return "", ErrFormat
	}
	payload, e := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if e != nil || json.Unmarshal(payload, &claims) != nil || claims.Exp <= s.now().Unix() {
		return "", ErrFormat
	}
	s.edge.Lock()
	s.edge.edgeCredential = edgeCredential{token, time.Unix(claims.Exp, 0), key}
	s.edge.Unlock()
	return token, nil
}
func (s *Service) invalidateEdge(token string) {
	s.edge.Lock()
	defer s.edge.Unlock()
	if s.edge.value == token {
		s.edge.edgeCredential = edgeCredential{}
	}
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
