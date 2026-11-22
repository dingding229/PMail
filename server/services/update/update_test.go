package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestReleaseAllowlistNumericETagAndManualRate(t *testing.T) {
	t.Setenv("PMAIL_UPDATE_CHECK", "1")
	count := 0
	c := New("0.0.1", transport(func(req *http.Request) (*http.Response, error) {
		count++
		if req.URL.String() != releasesURL {
			t.Fatalf("wrong URL %s", req.URL)
		}
		if _, ok := req.Context().Deadline(); !ok {
			t.Fatal("no deadline")
		}
		code := 200
		body := `[{"tag_name":"v0.2.1","body":"notes","published_at":"2026-11-01T00:00:00Z"},{"tag_name":"v0.10.0"},{"tag_name":"v1.0.0"},{"tag_name":"v0.20.0","prerelease":true},{"tag_name":"v0.50.0","draft":true}]`
		if count == 2 {
			if req.Header.Get("If-None-Match") != `"abc"` {
				t.Fatal("etag missing")
			}
			code = 304
			body = ""
		}
		return &http.Response{StatusCode: code, Header: http.Header{"Etag": {`"abc"`}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	}))
	ctx := context.Background()
	if err := c.Refresh(ctx, false); err != nil {
		t.Fatal(err)
	}
	if v := c.Info(); v.Latest == nil || *v.Latest != "v0.10.0" || !v.HasUpdate || len(v.Releases) != 3 || !v.Releases[2].Current || v.Releases[2].Notes == "" {
		t.Fatalf("release contract %+v", v)
	}
	if err := c.Refresh(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(ctx, true); !errors.Is(err, ErrRate) {
		t.Fatal(err)
	}
	if v := c.Info(); v.Error != nil || v.CheckedAt == nil || !v.HasUpdate || count != 2 {
		t.Fatalf("304 result %+v", v)
	}
}
func TestReleaseNoRedirectOversizeAndDisabled(t *testing.T) {
	t.Setenv("PMAIL_UPDATE_CHECK", "1")
	calls := 0
	c := New("0.0.1", transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://different.invalid/path"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		if r.URL.String() != releasesURL {
			t.Fatal("redirect followed")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxReleaseBytes+1))), Request: r}, nil
	}))
	_ = c.Refresh(context.Background(), false)
	if calls != 1 || c.Info().Error == nil {
		t.Fatal("not rejected")
	}
	_ = c.Refresh(context.Background(), false)
	if p := c.Info().Error; p == nil || *p != "update_check_invalid_response" {
		t.Fatal("oversize accepted")
	}
	t.Setenv("PMAIL_UPDATE_CHECK", "0")
	_ = c.Refresh(context.Background(), true)
	if calls != 2 {
		t.Fatal("disabled checker contacted upstream")
	}
}
