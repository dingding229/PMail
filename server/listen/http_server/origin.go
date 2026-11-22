package http_server

import (
	"net/http"
	"net/url"
	"strings"

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
	return u.Scheme == scheme && strings.EqualFold(u.Hostname(), host.Hostname()) && port(u) == port(host)
}
