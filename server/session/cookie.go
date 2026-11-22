package session

import (
	"net/http"

	"github.com/Jinnrry/pmail/utils/clientip"
)

// Cookie security is fixed per response. Never mutate the shared SCS manager
// when concurrent requests arrive through different transports.
func SecureCookies(name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !clientip.HTTPS(r) {
			next.ServeHTTP(w, r)
			return
		}
		sw := &secureCookieWriter{ResponseWriter: w, name: name}
		next.ServeHTTP(sw, r)
		if !sw.written {
			sw.secure()
		}
	})
}

type secureCookieWriter struct {
	http.ResponseWriter
	name    string
	written bool
}

func (w *secureCookieWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *secureCookieWriter) secure() {
	values := w.Header().Values("Set-Cookie")
	for i, v := range values {
		c, err := http.ParseSetCookie(v)
		if err == nil && c.Name == w.name {
			c.Secure = true
			values[i] = c.String()
		}
	}
}
func (w *secureCookieWriter) WriteHeader(code int) {
	if !w.written {
		w.secure()
		if code >= 200 || code == 101 {
			w.written = true
		}
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *secureCookieWriter) Write(p []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}
