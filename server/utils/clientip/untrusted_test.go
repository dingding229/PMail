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

func TestDockerProxyDefaultAndExplicitOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, mode, netmask, peer  string
		container, explicit, trust bool
	}{
		{"docker default", "", "", "172.18.0.1", true, false, true},
		{"docker proxy", "proxy", "", "172.31.0.1", true, false, true},
		{"common LAN excluded", "proxy", "", "192.168.1.1", true, false, false},
		{"outside default", "proxy", "", "172.32.0.1", true, false, false},
		{"public default", "public", "", "172.18.0.1", true, false, false},
		{"native default", "proxy", "", "172.18.0.1", false, false, false},
		{"explicit empty", "proxy", "", "172.18.0.1", true, true, false},
		{"explicit LAN", "public", "192.168.1.1/32", "192.168.1.1", true, true, true},
		{"explicit replaces default", "proxy", "192.168.1.1/32", "172.18.0.1", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := inContainer
			t.Cleanup(func() { inContainer = old })
			inContainer = func() bool { return tc.container }
			t.Setenv("PMAIL_WEB_MODE", tc.mode)
			t.Setenv("PMAIL_TRUSTED_PROXIES", tc.netmask)
			if !tc.explicit {
				if err := os.Unsetenv("PMAIL_TRUSTED_PROXIES"); err != nil {
					t.Fatal(err)
				}
			}
			r := httptest.NewRequest("POST", "/api/logout", nil)
			r.RemoteAddr = tc.peer + ":5555"
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("X-Forwarded-For", "198.51.100.1")
			want := tc.peer
			if tc.trust {
				want = "198.51.100.1"
			}
			if got := Source(r); got != want || HTTPS(r) != tc.trust {
				t.Fatalf("source=%s, HTTPS=%t; want %s, %t", got, HTTPS(r), want, tc.trust)
			}
		})
	}
}
