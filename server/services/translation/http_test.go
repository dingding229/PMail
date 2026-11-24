package translation

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Jinnrry/pmail/services/netproxy"
)

type fakeResolver func(context.Context, string, string) ([]netip.Addr, error)

func (f fakeResolver) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return f(ctx, network, host)
}

func TestSSRFDialRejectsDeniedAddressesAndPinsDNS(t *testing.T) {
	r := fakeResolver(func(_ context.Context, n, host string) ([]netip.Addr, error) {
		switch host {
		case "mixed.test":
			return []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("192.168.0.1")}, nil
		case "public.test":
			return []netip.Addr{netip.MustParseAddr("2606:4700::1111")}, nil
		default:
			t.Error("unexpected DNS lookup")
			return nil, nil
		}
	})
	gotAddress := ""
	fake := func(_ context.Context, _, addr string) (net.Conn, error) { gotAddress = addr; return nil, ErrConnect }
	dial := checkedDial(r, fake, false)
	for _, host := range []string{"127.0.0.1", "10.55.1.2", "169.254.169.254", "::1", "100.64.8.9", "fd00::123", "0.0.0.0", "::", "224.0.0.1", "ff02::1", "172.17.0.1", "fe80::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "mixed.test"} {
		if _, err := dial(t.Context(), "tcp", net.JoinHostPort(host, "80")); !errors.Is(err, ErrNetwork) || gotAddress != "" {
			t.Fatalf("blocked address was dialed or misclassified (%s): %v", host, err)
		}
	}
	if _, err := dial(t.Context(), "tcp", "public.test:443"); !errors.Is(err, ErrConnect) || gotAddress != "[2606:4700::1111]:443" {
		t.Fatal("dialer did not pin the approved address")
	}
}

func TestAIHTTPPrivateToggleRedirectAndBoundedErrors(t *testing.T) {
	s, _, a, _, _ := fixture(t)
	var hits atomic.Int32
	mode := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/trap" {
			t.Error("followed redirect")
			w.WriteHeader(500)
			return
		}
		hits.Add(1)
		switch mode.Load() {
		case 1:
			http.Redirect(w, r, "/trap", http.StatusTemporaryRedirect)
		case 2:
			_, _ = io.WriteString(w, strings.Repeat("x", responseLimit+1))
		case 3:
			w.WriteHeader(401)
			_, _ = io.WriteString(w, "synthetic upstream secret")
		case 4:
			aiReply(w, `{"ok":true,"OK":true}`)
		default:
			aiReply(w, `{"ok":true}`)
		}
	}))
	t.Cleanup(server.Close)
	configureAI(t, t.Context(), a, server.URL, "model")
	if _, err := s.TestAI(t.Context(), a); !errors.Is(err, ErrNetwork) || hits.Load() != 0 {
		t.Fatalf("default sent traffic to private IP: %v", err)
	}
	// Appending a proxy with a localhost endpoint is also rejected by default.
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	proxyPort, _ := strconv.Atoi(port)
	proxy := &netproxy.Input{Fields: netproxy.Fields{Type: "http", Host: "127.0.0.1", Port: proxyPort}}
	if _, err := SetAI(t.Context(), a, AIInput{BaseURL: "http://destination.test/v1", Model: "model", ProxyMode: "custom", Proxy: proxy}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TestAI(t.Context(), a); !errors.Is(err, ErrNetwork) || hits.Load() != 0 {
		t.Fatalf("private proxy was not blocked: %v", err)
	}
	configureAI(t, t.Context(), a, server.URL, "model")
	if _, err := SetPolicy(t.Context(), PolicyPatch{AIAllowPrivateNetwork: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TestAI(t.Context(), a); err != nil {
		t.Fatalf("allowed private connection failed: %v", err)
	}
	for i, want := range []error{ErrProvider, ErrFormat, ErrAuth, ErrFormat} {
		mode.Store(int32(i + 1))
		before := hits.Load()
		if _, err := s.TestAI(t.Context(), a); !errors.Is(err, want) || strings.Contains(err.Error(), "synthetic") {
			t.Fatalf("mode %d: %v", i+1, err)
		}
		delta := int32(1)
		if i == 3 {
			delta = 2
		}
		if hits.Load() != before+delta {
			t.Fatal("unexpected number of upstream attempts")
		}
	}
}

func TestAIConcurrencyCap(t *testing.T) {
	s, _, a, b, _ := fixture(t)
	f, err := s.acquire(a)
	if err != nil {
		t.Fatal(err)
	}
	defer f()
	g, err := s.acquire(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.acquire(a); !errors.Is(err, ErrBusy) {
		t.Fatal("accepted third AI request")
	}
	h, err := s.acquire(b)
	if err != nil {
		t.Fatal("limit shared between users")
	}
	defer h()
	g()
	i, err := s.acquire(a)
	if err != nil {
		t.Fatal("slot not released")
	}
	i()
}

func TestAISystemProxyUsesWebConfigurationThenTelegram(t *testing.T) {
	s, _, a, _, _ := fixture(t)
	ctx := t.Context()
	var webHits, telegramHits atomic.Int32
	newProxy := func(hits *atomic.Int32) *httptest.Server {
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !r.URL.IsAbs() || r.URL.Host != "10.2.3.4" || r.URL.Path != "/v1/chat/completions" {
				t.Error("request did not use the configured proxy")
				w.WriteHeader(500)
				return
			}
			hits.Add(1)
			aiReply(w, `{"ok":true}`)
		}))
		t.Cleanup(proxy.Close)
		return proxy
	}
	telegram := newProxy(&telegramHits)
	web := newProxy(&webHits)
	s.systemTelegram = func(context.Context) (*url.URL, error) { return url.Parse(telegram.URL) }
	if _, err := SetPolicy(ctx, PolicyPatch{AIAllowPrivateNetwork: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetAI(ctx, a, AIInput{BaseURL: "http://10.2.3.4/v1", Model: "test-model", ProxyMode: "system"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TestAI(ctx, a); err != nil || telegramHits.Load() != 1 || webHits.Load() != 0 {
		t.Fatalf("system→Telegram fallback: %v", err)
	}
	u, _ := url.Parse(web.URL)
	port, _ := strconv.Atoi(u.Port())
	if _, err := SetPolicy(ctx, PolicyPatch{ProxyMode: ptr("custom"), Proxy: &netproxy.Input{Fields: netproxy.Fields{Type: "http", Host: u.Hostname(), Port: port}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TestAI(ctx, a); err != nil || telegramHits.Load() != 1 || webHits.Load() != 1 {
		t.Fatalf("web proxy not preferred: %v", err)
	}
	if _, err := SetPolicy(ctx, PolicyPatch{AIUserConfigEnabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TestAI(ctx, a); !errors.Is(err, ErrAIDisabled) || webHits.Load() != 1 {
		t.Fatalf("disabled user AI reached provider: %v", err)
	}
}
