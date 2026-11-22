package http_server

import (
	"compress/gzip"
	"encoding/json"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticWebFilesAndFallback(t *testing.T) {
	t.Setenv("PMAIL_DEFAULT_LANG", "zh-CN")
	root := t.TempDir()
	t.Setenv("PMAIL_WEB_ROOT", root)
	if err := os.Mkdir(filepath.Join(root, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"index.html":            "<title>PMail</title>",
		"assets/ui-CcOsICiy.js": "console.log('PMail')",
		"assets/about.js":       "console.log('about')",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("never serve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	handler := webMiddleware(staticWeb())
	for _, tc := range []struct {
		path, accept, cache, contentType string
		status                           int
	}{
		{"/", "", "no-cache", "text/html", 200},
		{"/settings/profile", "", "no-cache", "text/html", 200},
		{"/mail/alice.example", "text/html", "no-cache", "text/html", 200},
		{"/assets/ui-CcOsICiy.js", "", "public, max-age=31536000, immutable", "javascript", 200},
		{"/assets/about.js", "", "", "javascript", 200},
		{"/assets/missing.js", "*/*", "", "text/plain", 404},
		{"/api/plugin/list", "text/html", "no-store", "application/json", 404},
		{"/attachments/missing", "text/html", "", "text/plain", 404},
		{"/.well-known/acme-challenge/missing", "text/html", "", "text/plain", 404},
		{"/assets/%2e%2e/escape", "", "", "text/plain", 404},
		{"/escape", "text/html", "", "text/plain", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			r.Header.Set("Accept", tc.accept)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != tc.cache ||
				!strings.Contains(w.Header().Get("Content-Type"), tc.contentType) || w.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
				t.Fatalf("status=%d headers=%v", w.Code, w.Header())
			}
			if tc.contentType == "application/json" {
				assertUnknownAPI(t, w, i18n.Text(i18n.Default(), "not_found"))
			}
			if tc.contentType == "text/html" && w.Header().Get("Content-Security-Policy") != webCSP {
				t.Fatalf("missing SPA CSP: %v", w.Header())
			}
		})
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Header().Get("Content-Encoding") != "gzip" || w.Header().Get("Content-Length") != "" {
		t.Fatalf("gzip: status=%d headers=%v", w.Code, w.Header())
	}
	gz, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	if data, err := io.ReadAll(gz); err != nil || string(data) != "<title>PMail</title>" {
		t.Fatalf("gzip: body=%q error=%v", data, err)
	}
}

func TestMissingWebRootDoesNotBlockAPIOrCORS(t *testing.T) {
	t.Setenv("PMAIL_WEB_ROOT", filepath.Join(t.TempDir(), "not-built"))
	t.Setenv("PMAIL_CORS_ORIGINS", " http://localhost:5173, https://dev.example.test ")
	mux := http.NewServeMux()
	mux.HandleFunc("/", staticWeb())
	mux.HandleFunc("/api/example", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := webMiddleware(mux)
	for _, tc := range []struct {
		method, path, origin, requestedMethod, allow string
		status                                       int
	}{
		{"GET", "/inbox", "", "", "", 503},
		{"GET", "/api/missing", "", "", "", 404},
		{"GET", "/api/example", "http://localhost:5173", "", "http://localhost:5173", 200},
		{"OPTIONS", "/api/example", "https://dev.example.test", "POST", "https://dev.example.test", 204},
		{"OPTIONS", "/api/example", "https://dev.example.test.attacker.invalid", "POST", "", 403},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Access-Control-Request-Method", tc.requestedMethod)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status || w.Header().Get("Access-Control-Allow-Origin") != tc.allow ||
			(w.Header().Get("Access-Control-Allow-Credentials") == "true") != (tc.allow != "") {
			t.Fatalf("%s %s (%s): status=%d headers=%v", tc.method, tc.path, tc.origin, w.Code, w.Header())
		}
	}
}

func assertUnknownAPI(t *testing.T, w *httptest.ResponseRecorder, message string) {
	t.Helper()
	var api response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &api); err != nil || w.Code != 404 || w.Header().Get("Content-Type") != "application/json" || api.ErrorNo != response.RouteNotFound || api.ErrorMsg != message || api.Data != "" {
		t.Fatalf("unknown API: HTTP %d; headers=%v; body=%s", w.Code, w.Header(), w.Body.String())
	}
}

func TestUnknownAPIUsesLanguageWithoutServingSPA(t *testing.T) {
	t.Setenv("PMAIL_WEB_ROOT", filepath.Join(t.TempDir(), "not-built"))
	mux := http.NewServeMux()
	router(mux, "test")
	r := httptest.NewRequest(http.MethodPost, "/api/not-a-route", nil)
	r.Header.Set("X-PMail-Lang", "en")
	r.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	webMiddleware(mux).ServeHTTP(w, r)
	assertUnknownAPI(t, w, i18n.Text(i18n.English, "not_found"))
}
