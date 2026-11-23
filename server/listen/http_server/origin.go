package http_server

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/Jinnrry/pmail/utils/clientip"
)

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

func sameOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	refs := r.Header.Values("Referer")
	var raw string
	if len(origins) > 0 {
		if len(origins) != 1 {
			return false
		}
		raw = origins[0]
	} else {
		if len(refs) != 1 {
			return false
		}
		raw = refs[0]
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Opaque != "" {
		return false
	}
	if len(origins) > 0 && (u.Path != "" || u.RawQuery != "" || u.Fragment != "") {
		return false
	}
	scheme := "http"
	if clientip.HTTPS(r) {
		scheme = "https"
	}
	host, err := url.Parse(scheme + "://" + r.Host)
	if err != nil || host.User != nil || host.Hostname() == "" || host.Path != "" {
		return false
	}
	port := func(v *url.URL) string {
		if p := v.Port(); p != "" {
			return p
		}
		if v.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	if !strings.EqualFold(u.Hostname(), host.Hostname()) {
		return false
	}
	originPort, requestPort := port(u), port(host)
	if u.Scheme == scheme && originPort == requestPort {
		return true
	}
	// A trusted reverse proxy may see plaintext HTTP when the browser used
	// HTTPS (for example, Cloudflare Flexible). Allow only the default-port
	// pair in this direction; do not treat the request itself as HTTPS.
	return u.Scheme == "https" && scheme == "http" && originPort == "443" && requestPort == "80" && clientip.ViaTrustedProxy(r)
}

var proxyHint struct {
	sync.Mutex
	last time.Time
}

// noteOriginRejection explains the most common cause of origin_forbidden in
// production: a reverse proxy terminates HTTPS but its address is not in
// PMAIL_TRUSTED_PROXIES, so X-Forwarded-Proto is ignored and the browser's
// https:// origin no longer matches. Logged at most every 10 minutes.
func noteOriginRejection(r *http.Request) {
	addr, ok := clientip.UntrustedForwarder(r)
	if !ok {
		return
	}
	proxyHint.Lock()
	defer proxyHint.Unlock()
	if time.Since(proxyHint.last) < 10*time.Minute {
		return
	}
	proxyHint.last = time.Now()
	log.Warnf("Rejected a cross-origin write (origin_forbidden): the request came through a reverse proxy at %s that is not trusted, so X-Forwarded-Proto was ignored. If this is your proxy (NPM/Nginx), add its exact address as a /32 (IPv4) or /128 (IPv6) to PMAIL_TRUSTED_PROXIES and restart PMail (Docker proxy mode trusts 172.16.0.0/12 only when the setting is unset); otherwise logout, sending and other writes from the browser fail with HTTP 403.", addr)
}
