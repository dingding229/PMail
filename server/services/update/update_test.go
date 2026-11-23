package update

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	log "github.com/sirupsen/logrus"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestReleaseAllowlistNumericETagAndManualRate(t *testing.T) {
	t.Setenv("PMAIL_UPDATE_CHECK", "1")
	var count atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.RequestURI() != "/fixture/releases?test=1" {
			t.Errorf("unexpected upstream request: %s %s", req.Method, req.URL)
		}
		switch count.Add(1) {
		case 1:
			if req.Header.Get("If-None-Match") != "" {
				t.Error("first request already has an ETag")
			}
			w.Header().Set("ETag", `"abc"`)
			_, _ = io.WriteString(w, `[
				{"tag_name":"v0.2.1","body":"notes","published_at":"2026-11-01T00:00:00Z"},
				{"tag_name":"v0.10.0"}, {"tag_name":"v1.0.0"}, {"tag_name":"0.80.0"},
				{"tag_name":"v0.90.0-beta.1"}, {"tag_name":"v0.20.0","prerelease":true},
				{"tag_name":"v0.50.0","draft":true}
			]`)
		case 2:
			if req.Header.Get("If-None-Match") != `"abc"` {
				t.Error("ETag missing")
			}
			w.WriteHeader(http.StatusNotModified)
		default:
			t.Error("rate-limited request reached upstream")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer fixture.Close()
	t.Setenv("PMAIL_UPDATE_RELEASES_URL", fixture.URL+"/fixture/releases?test=1")
	c := New("0.0.1", nil)
	ctx := context.Background()
	if err := c.Refresh(ctx, false); err != nil {
		t.Fatal(err)
	}
	if v := c.Info(); v.Latest == nil || *v.Latest != "v0.10.0" || !v.HasUpdate || len(v.Releases) != 3 ||
		v.Releases[1].Version != "v0.2.1" || !v.Releases[2].Current || v.Releases[2].Notes == "" {
		t.Fatalf("release contract %+v", v)
	}
	if err := c.Refresh(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(ctx, true); !errors.Is(err, ErrRate) {
		t.Fatalf("second manual check: %v", err)
	}
	if v := c.Info(); v.Error != nil || v.CheckedAt == nil || !v.HasUpdate || count.Load() != 2 {
		t.Fatalf("304 result %+v, upstream calls=%d", v, count.Load())
	}
}

func TestReleasesEndpointValidation(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
		ok        bool
	}{
		{"", defaultReleasesURL, true},
		{"https://releases.example.test:443/list?fixture=1", "https://releases.example.test:443/list?fixture=1", true},
		{"http://localhost:8080/list", "http://localhost:8080/list", true},
		{"http://127.0.0.1:8080/list", "http://127.0.0.1:8080/list", true},
		{"http://[::1]:8080/list", "http://[::1]:8080/list", true},
		{"https://[2001:db8::1]/list", "https://[2001:db8::1]/list", true},
		{"http://releases.example.test/list", defaultReleasesURL, false},
		{"http://localhost.example.test/list", defaultReleasesURL, false},
		{"http://127.0.0.2/list", defaultReleasesURL, false},
		{"http://[::ffff:127.0.0.1]/list", defaultReleasesURL, false},
		{"http://localhost@remote.example.test/list", defaultReleasesURL, false},
		{"https://user:secret@releases.example.test/list", defaultReleasesURL, false},
		{"https://releases.example.test/list#fragment", defaultReleasesURL, false},
		{"https://releases.example.test:65536/list", defaultReleasesURL, false},
		{"https://releases.example.test:/list", defaultReleasesURL, false},
		{"https://[localhost]/list", defaultReleasesURL, false},
		{"https:///list", defaultReleasesURL, false},
		{"https:releases.example.test/list", defaultReleasesURL, false},
		{"https://releases.example.test/%zz", defaultReleasesURL, false},
		{"//localhost/list", defaultReleasesURL, false},
		{"file:///tmp/list", defaultReleasesURL, false},
	} {
		if got, ok := releasesEndpoint(tc.raw); got != tc.want || ok != tc.ok {
			t.Errorf("releasesEndpoint(%q) = %q, %t; want %q, %t", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNewCheckerWarnsAndFallsBackOnInvalidEndpoint(t *testing.T) {
	logger := log.StandardLogger()
	var warnings bytes.Buffer
	oldOutput, oldLevel := logger.Out, logger.GetLevel()
	logger.SetOutput(&warnings)
	logger.SetLevel(log.WarnLevel)
	t.Cleanup(func() { logger.SetOutput(oldOutput); logger.SetLevel(oldLevel) })
	t.Setenv("PMAIL_UPDATE_RELEASES_URL", "https://invalid-user:fixture-secret@fake.example.test/list")
	c := New("0.0.1", nil)
	if c.endpoint != defaultReleasesURL || !strings.Contains(warnings.String(), "invalid PMAIL_UPDATE_RELEASES_URL") || strings.Contains(warnings.String(), "fixture-secret") {
		t.Fatalf("endpoint = %q, warning = %q", c.endpoint, warnings.String())
	}
}

func TestReleaseNoRedirectOversizeAndDisabled(t *testing.T) {
	t.Setenv("PMAIL_UPDATE_CHECK", "1")
	t.Setenv("PMAIL_UPDATE_RELEASES_URL", "")
	calls := 0
	c := New("0.0.1", transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://different.invalid/path"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		if r.URL.String() != defaultReleasesURL {
			t.Fatal("unexpected default URL or redirect followed")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("no release request deadline")
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
