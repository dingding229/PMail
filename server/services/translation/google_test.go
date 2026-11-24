package translation

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func TestGoogleBackupPreferenceAndPeriodicPrimaryProbe(t *testing.T) {
	s := testService()
	p := defaults()
	now := time.Unix(1800000000, 0)
	s.now = func() time.Time { return now }
	var primary, backup atomic.Int32
	var mode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || q.Get("sl") != "auto" || q.Get("tl") != "zh-CN" || q.Get("q") == "" {
			t.Error("incorrect Google request")
		}
		text := q.Get("q")
		if utf8.RuneCountInString(text) > 4500 || len(r.URL.RawQuery) > 7200 {
			t.Error("Google chunk exceeds limit")
		}
		switch r.URL.Path {
		case "/primary":
			if q.Get("client") != "gtx" || q.Get("dt") != "t" {
				t.Error("wrong gtx query")
			}
			primary.Add(1)
			if mode.Load() == 0 {
				w.WriteHeader(429)
				return
			}
			googleReply(w, strings.ToUpper(text))
		case "/backup":
			if q.Get("client") != "dict-chrome-ex" || q.Has("dt") {
				t.Error("wrong clients5 query")
			}
			backup.Add(1)
			if mode.Load() == 2 {
				w.WriteHeader(503)
				return
			}
			cut := len(text) / 2
			_ = json.NewEncoder(w).Encode([][]string{{strings.ToUpper(text[:cut]), "en"}, {strings.ToUpper(text[cut:]), "en"}})
		default:
			t.Error("unexpected path")
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	s.end.google, s.end.googleBackup = server.URL+"/primary", server.URL+"/backup"
	call := func() {
		t.Helper()
		_, out, src, err := s.translateWeb(t.Context(), p, "", "first second", "zh-CN")
		if err != nil || out != "FIRST SECOND" || src != "en" {
			t.Fatalf("Google translation: %q/%s %v", out, src, err)
		}
	}
	call()
	if primary.Load() != 1 || backup.Load() != 1 {
		t.Fatal("gtx 429 did not fail over")
	}
	now = now.Add(9 * time.Minute)
	call()
	if primary.Load() != 1 || backup.Load() != 2 {
		t.Fatal("backup preference not remembered")
	}
	now = now.Add(time.Minute)
	mode.Store(1)
	call()
	if primary.Load() != 2 || backup.Load() != 2 {
		t.Fatal("backup request extended the grace period")
	}
	mode.Store(0)
	call()
	mode.Store(2)
	call()
	if primary.Load() != 4 || backup.Load() != 4 {
		t.Fatal("preferred backup failure didn't try gtx")
	}
	// Restore the same Unicode/CRLF boundary behavior of the original gtx path.
	mode.Store(1)
	text := strings.Repeat("a", 4140) + "\r\n\r\n" + strings.Repeat("Ｂ", 4790) + " b"
	_, out, src, err := s.translateWeb(t.Context(), p, "", text, "zh-CN")
	if err != nil || out != strings.ToUpper(text) || src != "en" {
		t.Fatalf("Google chunk reconstruction: %v", err)
	}
}

func TestGoogleFailoverAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		one, two int
		want     error
	}{
		{"primary format incompatible", 200, 201, nil},
		{"primary server error", 503, 201, nil},
		{"one endpoint limited", 429, 502, ErrUpstreamRate},
		{"backup limited", 502, 429, ErrUpstreamRate},
		{"both limited", 429, 429, ErrUpstreamRate},
		{"both malformed", 200, 200, ErrProvider},
		{"both unavailable", 502, 503, ErrProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService()
			var called atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := tc.one
				if r.URL.Path == "/backup" {
					n = tc.two
					called.Add(1)
				}
				w.WriteHeader(n)
				if n == 201 {
					_, _ = w.Write([]byte(`[["你好","en"]]`))
				} else {
					_, _ = w.Write([]byte(`{"unsupported":true}`))
				}
			}))
			t.Cleanup(server.Close)
			s.end.google, s.end.googleBackup = server.URL+"/primary", server.URL+"/backup"
			_, out, src, err := s.translateWeb(t.Context(), defaults(), "", "Hello", "zh-CN")
			if !errors.Is(err, tc.want) || called.Load() != 1 || tc.want == nil && (out != "你好" || src != "en") {
				t.Fatalf("wrong fallback result %q/%s: %v, calls=%d", out, src, err, called.Load())
			}
		})
	}
}
