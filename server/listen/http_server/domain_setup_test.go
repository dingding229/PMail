package http_server

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
)

func TestSetupWebDomain(t *testing.T) {
	for host, want := range map[string]string{
		"mail.xshi.cc":        "",
		"mail.xshi.cc:443":    "",
		"pm.xshi.cc":          "pm.xshi.cc",
		"185.99.135.205:8080": "",
		"localhost:8080":      "",
		"[::1]:80":            "",
	} {
		if got := setupWebDomain(host, "xshi.cc"); got != want {
			t.Errorf("setupWebDomain(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestDomainSetupRequiresKeyThenSaves(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	oldRoot, oldInstance := config.ROOT_PATH, config.Instance
	t.Cleanup(func() { config.ROOT_PATH, config.Instance = oldRoot, oldInstance })
	config.ROOT_PATH = root + string(os.PathSeparator)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	config.Instance = &config.Config{HttpPort: port}

	saved := make(chan [2]string, 1)
	finished := make(chan error, 1)
	go func() {
		finished <- DomainSetup("test", func(d, w string) error { saved <- [2]string{d, w}; return nil })
	}()
	base := "http://127.0.0.1:" + itoa(port)
	var key string
	for i := 0; i < 50; i++ {
		if resp, err := http.Get(base + "/api/bootstrap"); err == nil {
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if body["need_domain"] != true {
				t.Fatalf("bootstrap: %v", body)
			}
			key, err = config.ReadRegistrationKey()
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	post := func(k, d string) int {
		payload, _ := json.Marshal(domainSetupBody{Key: k, Domain: d})
		resp, err := http.Post(base+"/api/setup/domain", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post("AAAA-AAAA-AAAA-AAAA", "xshi.cc"); code != http.StatusForbidden {
		t.Fatalf("wrong key: %d", code)
	}
	if code := post(key, "not a domain"); code != http.StatusBadRequest {
		t.Fatalf("bad domain: %d", code)
	}
	if code := post(key, "XSHI.cc"); code != http.StatusOK {
		t.Fatalf("valid: %d", code)
	}
	if got := <-saved; got[0] != "xshi.cc" || got[1] != "" {
		t.Fatalf("saved %v", got)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("DomainSetup did not return")
	}
	if _, err := config.ReadRegistrationKey(); err != nil {
		t.Fatal("the registration key must survive for the admin registration step")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
