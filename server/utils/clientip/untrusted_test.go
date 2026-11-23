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

func TestTrustedProxiesDefault(t *testing.T) {
	old := inContainer
	t.Cleanup(func() { inContainer = old })
	inContainer = func() bool { return true }
	t.Setenv("PMAIL_WEB_MODE", "")
	os.Unsetenv("PMAIL_TRUSTED_PROXIES")
	if got := TrustedProxies(); got != DockerProxyRanges {
		t.Fatalf("container proxy default: %q", got)
	}
	t.Setenv("PMAIL_WEB_MODE", "public")
	if got := TrustedProxies(); got != "" {
		t.Fatalf("public mode must not trust by default: %q", got)
	}
	t.Setenv("PMAIL_WEB_MODE", "")
	t.Setenv("PMAIL_TRUSTED_PROXIES", "")
	if got := TrustedProxies(); got != "" {
		t.Fatalf("explicit empty must win: %q", got)
	}
	os.Unsetenv("PMAIL_TRUSTED_PROXIES")
	inContainer = func() bool { return false }
	if got := TrustedProxies(); got != "" {
		t.Fatalf("outside a container: %q", got)
	}
}
