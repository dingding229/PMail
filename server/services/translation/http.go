package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

const responseLimit = 2 << 20

var carrierNAT = netip.MustParsePrefix("100.64.0.0/10")
var ipv4ThisNetwork = netip.MustParsePrefix("0.0.0.0/8")
var ipv4Reserved = netip.MustParsePrefix("240.0.0.0/4")
var nat64WKP = netip.MustParsePrefix("64:ff9b::/96")
var nat64Local = netip.MustParsePrefix("64:ff9b:1::/48")
var sixToFour = netip.MustParsePrefix("2002::/16")
var teredo = netip.MustParsePrefix("2001::/32")

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type dialFunc func(context.Context, string, string) (net.Conn, error)

func addressAllowed(ip netip.Addr, private bool) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	if private {
		return true
	}
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !carrierNAT.Contains(ip) && !ipv4ThisNetwork.Contains(ip) && !ipv4Reserved.Contains(ip) && !nat64WKP.Contains(ip) && !nat64Local.Contains(ip) && !sixToFour.Contains(ip) && !teredo.Contains(ip)
}

// Validate the complete DNS answer before any connection. Dial only a checked
// numeric address, so the transport never re-resolves an approved hostname.
// With a proxy net/http calls this dialer for the proxy endpoint.
func checkedDial(resolve resolver, dial dialFunc, allowPrivate bool) dialFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrConnect
		}
		var ips []netip.Addr
		if literal, e := netip.ParseAddr(host); e == nil {
			ips = []netip.Addr{literal}
		} else {
			ips, err = resolve.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
		}
		if len(ips) == 0 {
			return nil, ErrConnect
		}
		for _, ip := range ips {
			if !addressAllowed(ip, allowPrivate) {
				return nil, ErrNetwork
			}
		}
		for _, ip := range ips {
			c, e := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if e == nil {
				return c, nil
			}
			err = e
			if ctx.Err() != nil {
				break
			}
		}
		return nil, err
	}
}

func httpClient(timeout int, proxy *url.URL, dial dialFunc) *http.Client {
	transport := &http.Transport{DialContext: dial, DisableKeepAlives: true, TLSHandshakeTimeout: 5 * time.Second, MaxResponseHeaderBytes: 64 << 10}
	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Neither transport errors (which can include URLs) nor upstream responses
// are ever returned verbatim. Keep status only for controlled fallback logic.
func exchange(ctx context.Context, c *http.Client, method, endpoint string, body any, headers map[string]string) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		p, err := json.Marshal(body)
		if err != nil {
			return nil, 0, ErrFormat
		}
		reader = bytes.NewReader(p)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, ErrInvalid
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, 0, transportError(err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		switch res.StatusCode {
		case 401, 403, 407, 456:
			return nil, res.StatusCode, ErrAuth
		case 429:
			return nil, res.StatusCode, ErrUpstreamRate
		default:
			return nil, res.StatusCode, ErrProvider
		}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, responseLimit+1))
	if err != nil {
		return nil, res.StatusCode, transportError(err)
	}
	if len(data) > responseLimit {
		return nil, res.StatusCode, ErrFormat
	}
	return data, res.StatusCode, nil
}
func transportError(err error) error {
	var ne net.Error
	switch {
	case errors.Is(err, ErrNetwork):
		return ErrNetwork
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return ErrTimeout
	default:
		return ErrConnect
	}
}
