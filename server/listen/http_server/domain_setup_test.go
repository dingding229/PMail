package http_server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
)

var setupTestIPs atomic.Uint32

func setupTestIP() string {
	n := setupTestIPs.Add(1)
	return fmt.Sprintf("198.18.%d.%d", (n>>8)&255, n&255)
}

func TestDomainSetupValidationAndHandoff(t *testing.T) {
	base, key, finished := startTestDomainSetup(t, func(d string) error {
		if d != "example.test" {
			return fmt.Errorf("unexpected domain: %q", d)
		}
		return nil
	})
	resp, err := http.Get(base + "/api/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		NeedDomain  bool   `json:"need_domain"`
		DefaultLang string `json:"default_lang"`
	}
	err = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if err != nil || !body.NeedDomain || body.DefaultLang != "en" {
		t.Fatalf("bootstrap: %+v (%v)", body, err)
	}
	remote := setupTestIP()
	for _, tc := range []struct {
		key, domain, origin string
		status              int
	}{
		{"invalid", "example.test", "", http.StatusForbidden},
		{key, "not a domain", "", http.StatusBadRequest},
		{key, "192.0.2.20", "", http.StatusBadRequest},
		{key, "example.test", "https://elsewhere.test", http.StatusForbidden},
		{key, "Example.Test.", "", http.StatusOK},
	} {
		if code := setupPost(t, base, tc.key, tc.domain, tc.origin, remote); code != tc.status {
			t.Fatalf("setup %q: got HTTP %d, want %d", tc.domain, code, tc.status)
		}
	}
	waitSetup(t, finished)
	if _, err := config.ReadRegistrationKey(); err != nil {
		t.Fatal("the registration key must survive for admin registration")
	}
	socket, err := net.Listen("tcp", base[len("http://"):])
	if err != nil {
		t.Fatalf("normal listener could not bind: %v", err)
	}
	socket.Close()
}

func TestDomainSetupLimitsAttemptsAndDrainsSlowRequests(t *testing.T) {
	base, key, finished := startTestDomainSetup(t, func(string) error { return nil })
	remote := setupTestIP()
	for i := 0; i < 6; i++ {
		want := http.StatusForbidden
		if i == 5 {
			want = http.StatusTooManyRequests
		}
		if code := setupPost(t, base, "invalid", "example.test", "", remote); code != want {
			t.Fatalf("attempt %d: HTTP %d, want %d", i+1, code, want)
		}
	}
	slow, err := net.Dial("tcp", base[len("http://"):])
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	if _, err := fmt.Fprintf(slow, "GET /api/ping HTTP/1.1\r\nHost: localhost\r\n\r\nPOST /api/setup/domain HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: 64\r\n\r\n{"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(slow), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setup ping: %d", resp.StatusCode)
	}
	if code := setupPost(t, base, key, "example.test", "", setupTestIP()); code != http.StatusOK {
		t.Fatal(code)
	}
	waitSetup(t, finished)
	_ = slow.SetDeadline(time.Now().Add(time.Second))
	if n, err := slow.Read(make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("slow request still open: %d, %v", n, err)
	}
}

func TestDomainSetupCommitsOnlyOnce(t *testing.T) {
	key := testDomainSetupKey(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var saves, completions atomic.Int32
	handler := domainSetupHandler("test", func(string) error {
		if saves.Add(1) == 1 {
			close(entered)
		}
		<-release
		return nil
	}, func() { completions.Add(1) })
	post := func(domain string) int {
		payload, _ := json.Marshal(domainSetupBody{Key: key, Domain: domain})
		r := httptest.NewRequest(http.MethodPost, "/api/setup/domain", bytes.NewReader(payload))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = setupTestIP() + ":50000"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	codes := make(chan int, 2)
	go func() { codes <- post("one.test") }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("first submission did not reach persistence")
	}
	go func() { codes <- post("two.test") }()
	close(release)
	a, b := <-codes, <-codes
	if a == http.StatusConflict {
		a, b = b, a
	}
	if saves.Load() != 1 || completions.Load() != 1 || a != http.StatusOK || b != http.StatusConflict {
		t.Fatalf("saves=%d completions=%d HTTP responses=%d and %d", saves.Load(), completions.Load(), a, b)
	}
}

func TestDomainSetupReportsListenerFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
	if err := serveDomainSetup(ln, "test", nil, time.Second); err == nil {
		t.Fatal("listener failure must not hang setup")
	}
}

func startTestDomainSetup(t *testing.T, save SaveDomainFunc) (string, string, <-chan error) {
	t.Helper()
	t.Setenv("PMAIL_DEFAULT_LANG", "en")
	t.Setenv("PMAIL_TRUSTED_PROXIES", "127.0.0.1/32")
	key := testDomainSetupKey(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	finished := make(chan error, 1)
	go func() { finished <- serveDomainSetup(ln, "test", save, 250*time.Millisecond) }()
	return "http://" + ln.Addr().String(), key, finished
}

func testDomainSetupKey(t *testing.T) string {
	t.Helper()
	oldRoot := config.ROOT_PATH
	t.Cleanup(func() { config.ROOT_PATH = oldRoot })
	config.ROOT_PATH = t.TempDir() + string(os.PathSeparator)
	key, err := config.EnsureRegistrationKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func setupPost(t *testing.T, base, key, domain, origin, remote string) int {
	t.Helper()
	payload, _ := json.Marshal(domainSetupBody{Key: key, Domain: domain})
	req, err := http.NewRequest("POST", base+"/api/setup/domain", bytes.NewReader(payload))
	if err != nil {
		t.Error(err)
		return 0
	}
	req.Host = "arbitrary.test"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", remote)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	} // May race with shutdown after another request committed.
	resp.Body.Close()
	return resp.StatusCode
}

func waitSetup(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first-run listener did not shut down")
	}
}
