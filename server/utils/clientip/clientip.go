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
	values, _ := config.CIDRs(os.Getenv("PMAIL_TRUSTED_PROXIES"))
	return values // configuration is also validated at startup
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
