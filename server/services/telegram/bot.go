package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotConfigured = errors.New("telegram_not_configured")
	ErrNotBound      = errors.New("telegram_not_bound")
	ErrBindExpired   = errors.New("telegram_bind_expired")
	ErrRateLimited   = errors.New("telegram_rate_limited")
	ErrToken         = errors.New("telegram_token_invalid")
	ErrProxy         = errors.New("telegram_proxy_invalid")
	ErrInvalid       = errors.New("telegram_invalid_parameters")
	ErrUnavailable   = errors.New("telegram_unavailable")
)
var tokenRE = regexp.MustCompile(`^[0-9]{1,20}:[A-Za-z0-9_-]{15,128}$`)
var botNameRE = regexp.MustCompile(`^[A-Za-z0-9_]{5,32}$`)

type botError struct{ Status, RetryAfter int }

func (*botError) Error() string { return "Telegram Bot API rejected request" }

type WorkerClient struct {
	token string
	base  *url.URL
	http  *http.Client
}
type user struct {
	Username string `json:"username"`
	IsBot    bool   `json:"is_bot"`
}
type chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	First    string `json:"first_name"`
	Last     string `json:"last_name"`
	Username string `json:"username"`
}
type update struct {
	ID      int64 `json:"update_id"`
	Message *struct {
		Chat chat   `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

var envProxy = func() string { return strings.TrimSpace(os.Getenv("PMAIL_TELEGRAM_PROXY")) }

func effectiveProxy(p string) string {
	if q := envProxy(); q != "" {
		return q
	}
	return p
}
func parseProxy(s string) (*url.URL, error) {
	if s == "" {
		return nil, nil
	}
	u, err := url.Parse(s)
	if err != nil || len(s) > 1024 || u.Hostname() == "" || (u.Path != "" && u.Path != "/") || u.Fragment != "" || u.RawQuery != "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
		return nil, ErrProxy
	}
	return u, nil
}
func newClient(token, p string) (*WorkerClient, error) {
	if token == "" {
		return nil, ErrNotConfigured
	}
	if !tokenRE.MatchString(token) {
		return nil, ErrToken
	}
	base := strings.TrimSpace(os.Getenv("PMAIL_TELEGRAM_API_BASE"))
	if base == "" {
		base = "https://api.telegram.org"
	}
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, ErrUnavailable
	}
	host := u.Hostname()
	loop := host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && loop) {
		return nil, ErrUnavailable
	}
	proxy, err := parseProxy(effectiveProxy(p))
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{DialContext: d.DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 32 * time.Second, DisableKeepAlives: true}
	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &WorkerClient{token, u, &http.Client{Transport: transport, Timeout: 36 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *WorkerClient) call(ctx context.Context, method string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return ErrUnavailable
	}
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/bot" + c.token + "/" + method
	u.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer res.Body.Close()
	// Never expose request URLs, Bot API descriptions or raw provider errors.
	data, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return ErrUnavailable
	}
	var wrapper struct {
		OK     bool            `json:"ok"`
		Code   int             `json:"error_code"`
		Result json.RawMessage `json:"result"`
		Params struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if err = json.Unmarshal(data, &wrapper); err != nil {
		if res.StatusCode == 403 || res.StatusCode == 429 {
			return &botError{Status: res.StatusCode}
		}
		return ErrUnavailable
	}
	if res.StatusCode != 200 || !wrapper.OK {
		code := wrapper.Code
		if code == 0 {
			code = res.StatusCode
		}
		return &botError{code, max(0, wrapper.Params.RetryAfter)}
	}
	if out != nil && json.Unmarshal(wrapper.Result, out) != nil {
		return ErrUnavailable
	}
	return nil
}
func (c *WorkerClient) me(ctx context.Context) (string, error) {
	var u user
	err := c.call(ctx, "getMe", struct{}{}, &u)
	if err != nil || !u.IsBot || !botNameRE.MatchString(u.Username) {
		return "", ErrToken
	}
	return u.Username, nil
}
func (c *WorkerClient) send(ctx context.Context, id int64, text string) error {
	return c.call(ctx, "sendMessage", map[string]any{"chat_id": id, "text": text, "parse_mode": "HTML", "link_preview_options": map[string]bool{"is_disabled": true}}, nil)
}
