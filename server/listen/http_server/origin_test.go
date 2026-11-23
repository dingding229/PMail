package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOriginWithTrustedHTTPForwarding(t *testing.T) {
	t.Setenv("PMAIL_TRUSTED_PROXIES", "172.24.0.2/32")
	const (
		webHost       = "mail.example.test"
		httpsOrigin   = "https://" + webHost
		trustedPeer   = "172.24.0.2:50000"
		untrustedPeer = "198.51.100.15:50000"
	)
	for _, tc := range []struct {
		name, host, origin, referer, peer, proto, cfVisitor string
		want                                                bool
	}{
		{name: "flexible", origin: httpsOrigin, peer: trustedPeer, proto: "http", want: true},
		{name: "explicit default ports and case-insensitive host", host: "MAIL.Example.Test:80", origin: httpsOrigin + ":443", peer: trustedPeer, proto: "http", want: true},
		{name: "different hostname", origin: "https://other.example.test", peer: trustedPeer, proto: "http"},
		{name: "https origin nondefault port", origin: httpsOrigin + ":8443", peer: trustedPeer, proto: "http"},
		{name: "http host nondefault port", host: webHost + ":8080", origin: httpsOrigin, peer: trustedPeer, proto: "http"},
		{name: "plain HTTP without proxy", origin: httpsOrigin, peer: untrustedPeer},
		{name: "untrusted peer with forwarded header", origin: httpsOrigin, peer: untrustedPeer, proto: "http"},
		{name: "trusted peer without forwarded header", origin: httpsOrigin, peer: trustedPeer},
		{name: "Cloudflare header alone ignored", origin: httpsOrigin, peer: trustedPeer, cfVisitor: `{"scheme":"https"}`},
		{name: "https request with http origin", origin: "http://" + webHost, peer: trustedPeer, proto: "https"},
		{name: "referer path with query", referer: httpsOrigin + "/settings?tab=account", peer: trustedPeer, proto: "http", want: true},
		{name: "referer with nondefault port", referer: httpsOrigin + ":8443/settings", peer: trustedPeer, proto: "http"},
		{name: "referer from untrusted peer", referer: httpsOrigin + "/settings", peer: untrustedPeer, proto: "http"},
		{name: "bad origin cannot be replaced with referer", origin: "https://other.example.test", referer: httpsOrigin + "/settings", peer: trustedPeer, proto: "http"},
		{name: "origin path still rejected", origin: httpsOrigin + "/settings", peer: trustedPeer, proto: "http"},
		{name: "strict HTTP match", origin: "http://" + webHost, peer: untrustedPeer, want: true},
		{name: "strict HTTPS match on same custom port", host: webHost + ":8443", origin: httpsOrigin + ":8443", peer: trustedPeer, proto: "https", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+webHost+"/api/logout", nil)
			if tc.host != "" {
				r.Host = tc.host
			}
			r.RemoteAddr = tc.peer
			for key, value := range map[string]string{"Origin": tc.origin, "Referer": tc.referer, "X-Forwarded-Proto": tc.proto, "CF-Visitor": tc.cfVisitor} {
				if value != "" {
					r.Header.Set(key, value)
				}
			}
			if got := sameOrigin(r); got != tc.want {
				t.Errorf("sameOrigin = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestFlexibleOriginDomainSetup(t *testing.T) {
	t.Setenv("PMAIL_TRUSTED_PROXIES", "172.24.0.2/32")
	key := testDomainSetupKey(t)
	var saved string
	applied := false
	handler := domainSetupHandler("test", func(domain string) error {
		saved = domain
		return nil
	}, func() { applied = true })
	payload, err := json.Marshal(domainSetupBody{Key: key, Domain: "example.test"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://mail.example.test/api/setup/domain", bytes.NewReader(payload))
	r.RemoteAddr = "172.24.0.2:50000"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://mail.example.test")
	r.Header.Set("X-Forwarded-Proto", "http")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK || saved != "example.test" || !applied {
		t.Fatalf("domain setup = HTTP %d, saved=%q, applied=%t: %s", w.Code, saved, applied, w.Body.String())
	}
}
