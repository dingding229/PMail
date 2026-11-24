package translation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"
)

func TestParseBingTranslatorPage(t *testing.T) {
	now := time.Unix(1800000000, 0)
	const page = `<script>var _G={IG:"abcDE123"};</script><!-- <div data-iid="translator.0000"> --><div data-iid="other.34"></div><span data-iid='translator.5028'>here</span><script>var params_AbusePreventionHelper = [1700000000000, 'token_-+%=', 3600000];</script>`
	value, err := parseBingPage([]byte(page), now)
	if err != nil || value.ig != "abcDE123" || value.iid != "translator.5028" || value.key != "1700000000000" || value.token != "token_-+%=" || !value.refreshAt.Equal(now.Add(59*time.Minute)) {
		t.Fatal("parsed Bing page fields incorrectly")
	}
	for _, invalid := range []string{
		`<html>Search for something else</html>`,
		strings.Replace(page, "translator.5028", "other.5028", 1),
		strings.Replace(page, "IG:", "DIFFERENT:", 1),
		strings.Replace(page, "3600000", "0", 1),
		strings.Replace(page, "params_AbusePreventionHelper", "changed", 1),
	} {
		if _, err := parseBingPage([]byte(invalid), now); !errors.Is(err, ErrProvider) {
			t.Fatalf("altered page did not return provider error: %v", err)
		}
	}
}

func fakeBing(t *testing.T) (*Service, policyRecord, *atomic.Int32, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	s := testService()
	p := defaults()
	p.Provider = "edge_web"
	var pages, posts, fail atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != bingUA || r.Header.Get("Authorization") != "" || r.Header.Get("X-Forwarded-For") != "" {
			t.Error("Bing sends unexpected identity")
		}
		if r.URL.Path == "/page" {
			if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" {
				t.Error("Bing did not establish fresh, anonymous page session")
			}
			n := pages.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "bing_fixture", Value: fmt.Sprint(n), Path: "/"})
			_, _ = fmt.Fprintf(w, `<div data-iid="translator.50%d"></div><script>var params_AbusePreventionHelper=[%d,"token%d",3600000]</script><script>var _G={IG:"ig%d"};</script>`, n, n, n, n)
			return
		}
		posts.Add(1)
		if r.URL.Path != "/tr" || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.ParseForm() != nil {
			t.Error("bad Bing post")
			w.WriteHeader(400)
			return
		}
		q := r.URL.Query()
		n := r.PostForm.Get("key")
		cookie, err := r.Cookie("bing_fixture")
		if q.Get("isVertical") != "1" || q.Get("IG") != "ig"+n || q.Get("IID") != "translator.50"+n || r.PostForm.Get("token") != "token"+n || err != nil || cookie.Value != n || r.PostForm.Get("fromLang") != "auto-detect" {
			t.Error("Bing session fields or Cookie not reused together")
			w.WriteHeader(400)
			return
		}
		text := r.PostForm.Get("text")
		if len(utf16.Encode([]rune(text))) > 1000 {
			t.Error("Bing chunk too large")
		}
		target := r.PostForm.Get("to")
		if target != "zh-Hant" && target != "zh-Hans" && target != "ja" {
			t.Error("wrong Bing language mapping")
		}
		switch fail.Load() {
		case 401, 403:
			code := fail.Swap(0)
			w.WriteHeader(int(code))
			return
		case 205:
			fail.Store(0)
			_, _ = w.Write([]byte(`{"statusCode":205}`))
			return
		case 992:
			_, _ = w.Write([]byte(`[{"statusCode":"205"}]`))
			return // continues after refresh
		case 429:
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"detectedLanguage": map[string]string{"language": "en"}, "translations": []map[string]string{{"text": strings.ToUpper(text), "to": target}}}})
	}))
	t.Cleanup(server.Close)
	s.end.bingPage, s.end.bingTranslate = server.URL+"/page", server.URL+"/tr"
	return s, p, &pages, &posts, &fail
}

func TestBingTranslationChunkingSessionReuseAndExpiry(t *testing.T) {
	s, p, pages, posts, fail := fakeBing(t)
	now := time.Unix(1800000000, 0)
	s.now = func() time.Time { return now }
	text := strings.Repeat("a", 900) + "\r\n\r\n" + strings.Repeat("🍏", 1104) + " words"
	call := func() {
		t.Helper()
		sub, out, src, err := s.translateWeb(t.Context(), p, "topic", text, "zh-TW")
		if err != nil || sub != "TOPIC" || out != strings.ToUpper(text) || src != "en" {
			t.Fatalf("Bing chunk result: %v", err)
		}
	}
	call()
	now = now.Add(59*time.Minute - time.Second)
	call()
	if pages.Load() != 1 || posts.Load() <= 2 {
		t.Fatal("Bing session not reused across chunks and calls")
	}
	now = now.Add(time.Second)
	call()
	if pages.Load() != 2 {
		t.Fatal("Bing session did not refresh one minute before expiration")
	}
	fail.Store(429)
	if _, _, _, err := s.translateWeb(t.Context(), p, "", "hello", "ja"); !errors.Is(err, ErrUpstreamRate) || pages.Load() != 2 {
		t.Fatalf("Bing rate limit caused a refresh or wrong error: %v", err)
	}
}

func TestBingRefreshOnceOnTokenFailure(t *testing.T) {
	for _, code := range []int32{401, 403, 205, 992} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			s, p, pages, posts, fail := fakeBing(t)
			fail.Store(code)
			_, text, _, err := s.translateWeb(t.Context(), p, "", "hello", "zh-CN")
			if code == 992 {
				if !errors.Is(err, ErrProvider) {
					t.Fatalf("failed refresh classification: %v", err)
				}
			} else if err != nil || text != "HELLO" {
				t.Fatalf("refresh failed: %v", err)
			}
			if pages.Load() != 2 || posts.Load() != 2 {
				t.Fatalf("expect just one refresh and retry, pages=%d posts=%d", pages.Load(), posts.Load())
			}
		})
	}
}

func TestBingChangedPageAndWaitCancellation(t *testing.T) {
	s := testService()
	p := defaults()
	p.Provider = "edge_web"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`<html>new structure</html>`)) }))
	t.Cleanup(server.Close)
	s.end.bingPage = server.URL
	if _, _, _, err := s.translateWeb(t.Context(), p, "", "hello", "zh-CN"); !errors.Is(err, ErrProvider) {
		t.Fatalf("changed page classification: %v", err)
	}
	s.bing.refreshing = make(chan struct{})
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := s.bingSession(ctx, nil, nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("stalled refresh did not honor deadline: %v", err)
	}
}
