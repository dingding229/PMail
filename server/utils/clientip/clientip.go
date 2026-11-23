package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"

	"github.com/Jinnrry/pmail/config"
)

func peer(raw string) netip.Addr {
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	addr, _ := netip.ParseAddr(raw)
	return addr.Unmap()
}

func trusted(addr netip.Addr, proxies []netip.Prefix) bool {
	for _, p := range proxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func proxies() []netip.Prefix {
	values, _ := config.CIDRs(TrustedProxies())
	return values // configuration is also validated at startup
}

// DockerProxyRanges are Docker's default private address pools. In the
// bundled Compose file the web port is bound only to the Docker bridge, so
// any peer is a local reverse proxy (NPM, host Nginx) on one of these ranges.
const DockerProxyRanges = "172.16.0.0/12,192.168.0.0/16"

// TrustedProxies returns PMAIL_TRUSTED_PROXIES. When it is unset, PMail runs
// in a container and the web mode is not "public", Docker's private ranges
// are trusted so a reverse proxy works without any configuration.
func TrustedProxies() string {
	if value, ok := os.LookupEnv("PMAIL_TRUSTED_PROXIES"); ok {
		return value
	}
	if os.Getenv("PMAIL_WEB_MODE") == "public" || !inContainer() {
		return ""
	}
	return DockerProxyRanges
}

var inContainer = func() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// Source trusts only hops listed by the operator. Starting from the socket
// peer, stop at the first untrusted hop rather than using the leftmost value.
func Source(r *http.Request) string {
	addr := peer(r.RemoteAddr)
	if !addr.IsValid() {
		return "unknown"
	}
	list := proxies()
	if !trusted(addr, list) {
		return addr.String()
	}
	raw := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if raw == "" || len(raw) > 4096 {
		return addr.String()
	}
	hops := strings.Split(raw, ",")
	if len(hops) > 32 {
		return addr.String()
	}
	socket := addr
	for i := len(hops) - 1; i >= 0 && trusted(addr, list); i-- {
		next, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil || next.Zone() != "" {
			return socket.String()
		}
		addr = next.Unmap()
	}
	return addr.String()
}

func HTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	values := r.Header.Values("X-Forwarded-Proto")
	// A trusted edge must overwrite the inbound proto header with one value.
	return len(values) == 1 && strings.EqualFold(strings.TrimSpace(values[0]), "https") && trusted(peer(r.RemoteAddr), proxies())
}

// UntrustedForwarder reports the socket peer when a request carries
// reverse-proxy headers (X-Forwarded-Proto/For) but the peer is not listed in
// PMAIL_TRUSTED_PROXIES, so those headers are ignored.
func UntrustedForwarder(r *http.Request) (netip.Addr, bool) {
	if r.Header.Get("X-Forwarded-Proto") == "" && r.Header.Get("X-Forwarded-For") == "" {
		return netip.Addr{}, false
	}
	addr := peer(r.RemoteAddr)
	if !addr.IsValid() || trusted(addr, proxies()) {
		return netip.Addr{}, false
	}
	return addr, true
}
