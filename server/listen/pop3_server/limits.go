package pop3_server

import (
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"
)

const idleTimeout = 10 * time.Minute

var admission connectionBudget

type connectionBudget struct {
	mu      sync.Mutex
	total   int
	sources map[string]int
}

func envBound(name string, fallback, maximum int) int {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil || n < 1 || n > maximum {
		return fallback
	}
	return n
}

func (b *connectionBudget) reserve(source string, totalLimit, sourceLimit int) (func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.total >= totalLimit || b.sources[source] >= sourceLimit {
		return nil, false
	}
	if b.sources == nil {
		b.sources = make(map[string]int)
	}
	b.total++
	b.sources[source]++
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.total--
		b.sources[source]--
		if b.sources[source] == 0 {
			delete(b.sources, source)
		}
	}, true
}

func socketSource(a net.Addr) string {
	if a == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return "unknown"
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Unmap().WithZone("").String()
	}
	return "unknown"
}

func popIdleTimeout() time.Duration {
	return time.Duration(envBound("PMAIL_POP3_IDLE_SECONDS", int(idleTimeout/time.Second), 3600)) * time.Second
}
