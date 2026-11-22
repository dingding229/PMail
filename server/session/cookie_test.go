package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestC7SessionCookieSecureBehindTrustedProxy(t *testing.T) {
	t.Setenv("PMAIL_TRUSTED_PROXIES", "10.1.2.0/24")
	h := SecureCookies("session", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.SetCookie(w, &http.Cookie{Name: "other", Value: "fixture"})
		w.WriteHeader(200)
	}))
	for _, tc := range []struct {
		url, addr string
		secure    bool
	}{{"https://mail.example.test", "198.51.100.8:8888", true}, {"http://mail.example.test", "10.1.2.9:8888", true}, {"http://mail.example.test", "198.51.100.8:8888", false}} {
		r := httptest.NewRequest("POST", tc.url, nil)
		r.RemoteAddr = tc.addr
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		c := w.Result().Cookies()
		if len(c) != 2 || c[0].Secure != tc.secure || !c[0].HttpOnly || c[0].SameSite != http.SameSiteLaxMode || c[1].Secure {
			t.Fatalf("unexpected cookies: %v", c)
		}
	}
}
