package clientip

import (
	"net/http/httptest"
	"os"
	"testing"
)

func TestUntrustedForwarder(t *testing.T) {
	t.Setenv("PMAIL_TRUSTED_PROXIES", "172.18.0.0/16")
	r := httptest.NewRequest("POST", "/api/logout", nil)
	r.RemoteAddr = "172.22.0.1:5555"
	if _, ok := UntrustedForwarder(r); ok {
		t.Fatal("no forwarding headers must not be reported")
	}
	r.Header.Set("X-Forwarded-Proto", "https")
	if addr, ok := UntrustedForwarder(r); !ok || addr.String() != "172.22.0.1" {
		t.Fatalf("got %v %v", addr, ok)
	}
	r.RemoteAddr = "172.18.0.2:5555"
	if _, ok := UntrustedForwarder(r); ok {
		t.Fatal("trusted proxy must not be reported")
	}
}

func TestPrivatePeersRequireExplicitTrust(t *testing.T) {
	t.Setenv("PMAIL_TRUSTED_PROXIES", "")
	if err := os.Unsetenv("PMAIL_TRUSTED_PROXIES"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"", "proxy", "public"} {
		t.Setenv("PMAIL_WEB_MODE", mode)
		r := httptest.NewRequest("POST", "/api/register", nil)
		r.Header.Set("X-Forwarded-For", "198.51.100.1")
		r.Header.Set("X-Forwarded-Proto", "https")
		for _, addr := range []string{"172.18.0.1", "192.168.1.1"} {
			r.RemoteAddr = addr + ":5555"
			if got := Source(r); got != addr || HTTPS(r) {
				t.Fatalf("mode %q trusted an unconfigured private peer: %s", mode, got)
			}
		}
	}
}
