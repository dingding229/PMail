package ip

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

var publicCache = struct {
	sync.Mutex
	byFamily map[string]publicResult
}{byFamily: make(map[string]publicResult)}

type publicResult struct {
	addr      netip.Addr
	timestamp time.Time
}

// PublicAddresses uses explicit operator values first. Missing families are
// probed independently, with the actual outbound address (works behind NAT).
func PublicAddresses(ctx context.Context, ipv4, ipv6 string) (v4, v6 netip.Addr) {
	var wg sync.WaitGroup
	wg.Go(func() { v4 = publicAddress(ctx, "tcp4", "https://api.ipify.org", ipv4) })
	wg.Go(func() { v6 = publicAddress(ctx, "tcp6", "https://api6.ipify.org", ipv6) })
	wg.Wait()
	return
}
func publicAddress(ctx context.Context, family, endpoint, override string) netip.Addr {
	parse := func(value string) netip.Addr {
		addr, err := netip.ParseAddr(strings.TrimSpace(value))
		addr = addr.Unmap()
		if err != nil || (family == "tcp4") != addr.Is4() {
			return netip.Addr{}
		}
		return addr
	}
	if strings.TrimSpace(override) != "" {
		return parse(override)
	}
	publicCache.Lock()
	cached, ok := publicCache.byFamily[family]
	publicCache.Unlock()
	if ok && time.Since(cached.timestamp) < time.Hour {
		return cached.addr
	}
	ctx, cancel := context.WithTimeout(ctx, 2250*time.Millisecond)
	defer cancel()
	dialer := net.Dialer{}
	transport := &http.Transport{DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, family, address)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	var address netip.Addr
	if err == nil {
		if response, err := client.Do(req); err == nil {
			defer response.Body.Close()
			if response.StatusCode == http.StatusOK {
				if data, err := io.ReadAll(io.LimitReader(response.Body, 128)); err == nil {
					address = parse(string(data))
					if !address.IsGlobalUnicast() || address.IsPrivate() {
						address = netip.Addr{}
					}
				}
			}
		}
	}
	if ctx.Err() != context.Canceled {
		publicCache.Lock()
		publicCache.byFamily[family] = publicResult{address, time.Now()}
		publicCache.Unlock()
	}
	return address
}
