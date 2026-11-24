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

	"github.com/Jinnrry/pmail/utils/safenet"
)

const responseLimit = 2 << 20

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type dialFunc func(context.Context, string, string) (net.Conn, error)

func addressAllowed(ip netip.Addr, private bool) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	return private || safenet.PublicAddress(ip)
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
	contentType := ""
	if body != nil {
		p, err := json.Marshal(body)
		if err != nil {
			return nil, 0, ErrFormat
		}
		reader = bytes.NewReader(p)
		contentType = "application/json"
	}
	return exchangeResponse(ctx, c, method, endpoint, reader, contentType, headers, false)
}

// A provider whose JSON error codes matter may inspect a small error payload.
// The caller must map it to fixed errors and never expose it to clients.
func exchangeResponse(ctx context.Context, c *http.Client, method, endpoint string, reader io.Reader, contentType string, headers map[string]string, readError bool) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, ErrInvalid
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, 0, transportError(err)
	}
	defer res.Body.Close()
	status := res.StatusCode
	var providerErr error
	if status < 200 || status >= 300 {
		switch status {
		case 401, 403, 407, 456:
			providerErr = ErrAuth
		case 429:
			providerErr = ErrUpstreamRate
		default:
			providerErr = ErrProvider
		}
		if !readError {
			return nil, status, providerErr
		}
	}
	budget := responseLimit
	if providerErr != nil {
		budget = 64 << 10
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, int64(budget+1)))
	if err != nil || len(data) > budget {
		if providerErr != nil {
			return nil, status, providerErr
		}
		if err != nil {
			return nil, status, transportError(err)
		}
		return nil, status, ErrFormat
	}
	return data, status, providerErr
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
