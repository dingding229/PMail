package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"
)

const bingPageEndpoint = "https://www.bing.com/translator"
const bingTranslateEndpoint = "https://www.bing.com/ttranslatev3"
const bingUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

var bingIG = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])['"]?IG['"]?\s*[:=]\s*['"]([A-Za-z0-9_-]{1,128})['"]`)
var bingIID = regexp.MustCompile(`^translator\.[0-9]+(?:\.[0-9]+)*$`)
var bingHelper = regexp.MustCompile(`\bparams_AbusePreventionHelper\s*=\s*\[\s*['"]?([0-9]{1,20})['"]?\s*,\s*['"]([A-Za-z0-9+/_=.:~%-]+)['"]\s*,\s*['"]?([0-9]{1,10})['"]?\s*\]`)

type bingCredential struct {
	ig, iid, key, token string
	refreshAt           time.Time
	jar                 http.CookieJar
	proxy               [32]byte
}
type bingCache struct {
	sync.Mutex
	credential *bingCredential
	refreshing chan struct{}
}

func parseBingPage(raw []byte, fetched time.Time) (*bingCredential, error) {
	out := &bingCredential{}
	z := html.NewTokenizer(bytes.NewReader(raw))
	inScript := false
	for kind := z.Next(); kind != html.ErrorToken; kind = z.Next() {
		switch kind {
		case html.StartTagToken, html.SelfClosingTagToken:
			tag := z.Token()
			inScript = kind == html.StartTagToken && tag.Data == "script"
			for _, a := range tag.Attr {
				if out.iid == "" && a.Key == "data-iid" && bingIID.MatchString(a.Val) {
					out.iid = a.Val
				}
			}
		case html.EndTagToken:
			inScript = false
		case html.TextToken:
			// Inspect only script data, never evaluate JavaScript or extract
			// credentials from HTML comments and unrelated text elements.
			if !inScript {
				continue
			}
			script := z.Text()
			if out.ig == "" {
				if m := bingIG.FindSubmatch(script); len(m) > 0 {
					out.ig = string(m[1])
				}
			}
			if out.token == "" {
				if m := bingHelper.FindSubmatch(script); len(m) > 0 {
					ttl, e := strconv.ParseInt(string(m[3]), 10, 64)
					key, keyErr := strconv.ParseUint(string(m[1]), 10, 64)
					if e == nil && ttl > 0 && keyErr == nil && key > 0 && len(m[2]) <= 2048 {
						out.key, out.token = string(m[1]), string(m[2])
						out.refreshAt = fetched.Add(time.Duration(ttl)*time.Millisecond - time.Minute)
					}
				}
			}
		}
		if out.iid != "" && out.ig != "" && out.token != "" {
			return out, nil
		}
	}
	return nil, ErrProvider
}

func (s *Service) bingSession(ctx context.Context, c *http.Client, proxy *url.URL) (*bingCredential, error) {
	identity := proxyIdentity(proxy)
	for {
		s.bing.Lock()
		if token := s.bing.credential; token != nil && token.proxy == identity && s.now().Before(token.refreshAt) {
			s.bing.Unlock()
			return token, nil
		}
		if ready := s.bing.refreshing; ready != nil {
			s.bing.Unlock()
			select {
			case <-ready:
				continue
			case <-ctx.Done():
				return nil, transportError(ctx.Err())
			}
		}
		s.bing.refreshing = make(chan struct{})
		s.bing.Unlock()
		break
	}
	defer func() { s.bing.Lock(); defer s.bing.Unlock(); close(s.bing.refreshing); s.bing.refreshing = nil }()
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, ErrProvider
	}
	client := *c
	client.Jar = jar
	raw, _, err := exchange(ctx, &client, http.MethodGet, s.end.bingPage, nil, map[string]string{"User-Agent": bingUA})
	if err != nil {
		return nil, err
	}
	token, err := parseBingPage(raw, s.now())
	if err != nil {
		return nil, err
	}
	token.jar, token.proxy = jar, identity
	s.bing.Lock()
	s.bing.credential = token
	s.bing.Unlock()
	return token, nil
}

func (s *Service) invalidateBing(used *bingCredential) {
	s.bing.Lock()
	defer s.bing.Unlock()
	if s.bing.credential == used {
		s.bing.credential = nil
	}
}

type bingResult struct {
	Status   json.RawMessage `json:"statusCode"`
	Detected struct {
		Language string `json:"language"`
	} `json:"detectedLanguage"`
	Translations []struct {
		Text string `json:"text"`
	} `json:"translations"`
}

func (s *Service) bingChunk(ctx context.Context, c *http.Client, text, lang string, proxy *url.URL) (string, string, error) {
	switch lang {
	case "zh-CN":
		lang = "zh-Hans"
	case "zh-TW":
		lang = "zh-Hant"
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, err := s.bingSession(ctx, c, proxy)
		if err != nil {
			return "", "", err
		}
		client := *c
		client.Jar = token.jar
		endpoint := withQuery(s.end.bingTranslate, url.Values{"isVertical": {"1"}, "IG": {token.ig}, "IID": {token.iid}})
		form := url.Values{"fromLang": {"auto-detect"}, "to": {lang}, "text": {text}, "token": {token.token}, "key": {token.key}}
		raw, status, err := exchangeResponse(ctx, &client, http.MethodPost, endpoint, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", map[string]string{"User-Agent": bingUA}, true)
		var rows []bingResult
		var response bingResult
		decoded := true
		if json.Unmarshal(raw, &rows) == nil && len(rows) > 0 {
			response = rows[0]
		} else {
			decoded = json.Unmarshal(raw, &response) == nil
		}
		code := numericStatus(response.Status)
		invalid := func(n int) bool { return n == 205 || n == 401 || n == 403 }
		if invalid(status) || invalid(code) {
			s.invalidateBing(token)
			continue
		}
		if code == 429 {
			return "", "", ErrUpstreamRate
		}
		if err != nil {
			return "", "", err
		}
		if code != 0 && code != 200 {
			return "", "", ErrProvider
		}
		if !decoded || len(response.Translations) != 1 || strings.TrimSpace(response.Translations[0].Text) == "" {
			return "", "", ErrFormat
		}
		return response.Translations[0].Text, response.Detected.Language, nil
	}
	return "", "", ErrProvider
}
