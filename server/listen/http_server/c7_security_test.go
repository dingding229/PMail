package http_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/utils/clientip"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestC7SetupTokenGate(t *testing.T) {
	h := setupOnly("development-fixture", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 403}, {"wrong", 403}, {"development-fixture", 204}} {
		r := httptest.NewRequest(http.MethodPost, "/api/setup", nil)
		r.Header.Set("X-PMail-Setup-Token", tc.token)
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != tc.status || strings.Contains(w.Body.String(), "development-fixture") {
			t.Fatalf("setup HTTP %d: %s", w.Code, w.Body.String())
		}
	}
	t.Setenv("PMAIL_LEGACY_SETUP", "")
	if err := SetupStart("dev"); err == nil {
		t.Fatal("disabled setup listener accepted startup")
	}
}

func TestC7ProxyOriginAndClientIP(t *testing.T) {
	t.Setenv("PMAIL_TRUSTED_PROXIES", "10.0.0.0/24")
	r := httptest.NewRequest(http.MethodPost, "http://mail.example.test/api/logout", nil)
	r.RemoteAddr = "10.0.0.3:8400"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-For", "192.0.2.4, 198.51.100.6, 10.0.0.2")
	r.Header.Set("Origin", "https://mail.example.test")
	if !clientip.HTTPS(r) || !sameOrigin(r) || clientip.Source(r) != "198.51.100.6" {
		t.Fatal("trusted edge not resolved correctly")
	}
	r.Header.Set("Origin", "https://other.example.test")
	if sameOrigin(r) {
		t.Fatal("sibling origin accepted")
	}
	r.Header.Del("Origin")
	r.Header.Set("Referer", "https://mail.example.test/settings")
	if !sameOrigin(r) {
		t.Fatal("same-origin referer was not accepted")
	}
	r.RemoteAddr = "198.51.100.5:8400"
	if clientip.HTTPS(r) || sameOrigin(r) || clientip.Source(r) != "198.51.100.5" {
		t.Fatal("untrusted proxy headers were accepted")
	}
}

func TestC7LogoutAndJSONMediaType(t *testing.T) {
	// These policy rejections must happen before touching the session store.
	called := false
	h := contextIterceptor(func(_ *context.Context, _ http.ResponseWriter, _ *http.Request) { called = true })
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/api/logout", 405}, {"POST", "/api/login", 415}} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}")))
		if w.Code != tc.status || called {
			t.Fatalf("%s: HTTP %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}
