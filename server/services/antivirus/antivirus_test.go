package antivirus

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "pmail-av-") // short enough for macOS and Linux sockaddr_un
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func readStream(conn net.Conn, raw []byte) error {
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	r := bufio.NewReader(conn)
	if command, err := r.ReadString(0); err != nil || command != "zINSTREAM\x00" {
		return fmt.Errorf("INSTREAM header %q: %w", command, err)
	}
	var got bytes.Buffer
	for {
		var n uint32
		if err := binary.Read(r, binary.BigEndian, &n); err != nil {
			return err
		}
		if n == 0 {
			break
		}
		if n > 32<<10 {
			return fmt.Errorf("frame too large: %d", n)
		}
		if _, err := io.CopyN(&got, r, int64(n)); err != nil {
			return err
		}
	}
	if !bytes.Equal(got.Bytes(), raw) {
		return errors.New("raw MIME was changed")
	}
	return nil
}

func TestFakeUnixINSTREAM(t *testing.T) {
	raw := bytes.Repeat([]byte("Content-Type: text/plain\r\n\r\nTest\r\n"), 2500)
	for _, tc := range []struct{ name, wire, state, reason, virus string }{
		{"clean", "stream: OK\x00", "clean", "", ""},
		{"found", "stream: Test.Fixture FOUND\x00", "infected", "", "Test.Fixture"},
		{"unusual_signature", "stream: Test (fixture) FOUND\x00", "infected", "", "Test_fixture_"},
		{"limits", "stream: Heuristics.Limits.Exceeded.MaxFileSize FOUND\x00", "not_scanned", "scan_limit", ""},
		{"malformed", "unexpected payload\x00", "not_scanned", "scanner_unavailable", ""},
		{"timeout", "", "not_scanned", "scan_timeout", ""},
		{"connection_failure", "", "not_scanned", "scanner_unavailable", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newManager(shortDir(t), true, nil)
			m.state.State = "running"
			m.scanTimeout = 2 * time.Second
			if tc.name == "timeout" {
				m.scanTimeout = 150 * time.Millisecond
			}
			if tc.name != "connection_failure" {
				ln, err := net.Listen("unix", m.socket)
				if err != nil {
					t.Fatal(err)
				}
				defer ln.Close()
				checked := make(chan error, 1)
				go func() {
					conn, err := ln.Accept()
					if err != nil {
						checked <- err
						return
					}
					defer conn.Close()
					if err := readStream(conn, raw); err != nil {
						checked <- err
						return
					}
					if tc.wire != "" {
						_, err = io.WriteString(conn, tc.wire)
					} else {
						_, _ = conn.Read(make([]byte, 1))
					}
					checked <- err
				}()
				defer func() {
					_ = ln.Close()
					if err := <-checked; err != nil {
						t.Error(err)
					}
				}()
			}
			if result := m.Scan(t.Context(), raw); result == nil || *result != (Result{Status: tc.state, Reason: tc.reason, Virus: tc.virus}) {
				t.Fatalf("unexpected result: %+v", result)
			}
			s := m.Snapshot()
			if (s.Scanned == 1) != (tc.state != "not_scanned") || (s.Blocked == 1) != (tc.state == "infected") {
				t.Fatalf("unexpected counts: %+v", s)
			}
		})
	}
}

func eventCount(dir, name string) int {
	b, _ := os.ReadFile(filepath.Join(dir, name))
	return bytes.Count(b, []byte("\n"))
}

func appendEvent(dir, name string) error {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	_, err = io.WriteString(f, "event\n")
	return errors.Join(err, f.Close())
}

// A real subprocess fixture exercises Start, Wait, cancellation, and reaping,
// with a synthetic socket and signature headers but no actual scanner binaries.
func TestAntivirusProcessFixture(t *testing.T) {
	if os.Getenv("PMAIL_AV_PROCESS_FIXTURE") != "1" {
		return
	}
	kind, dir := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	if kind == "freshclam" {
		if err := appendEvent(dir, "updates"); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"main", "daily"} {
			head := fmt.Sprintf("ClamAV-VDB:fixture:%d:rest", eventCount(dir, "updates"))
			if err := os.WriteFile(filepath.Join(dir, name+".cvd"), []byte(head+strings.Repeat(" ", 512-len(head))), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	if err := appendEvent(dir, "starts"); err != nil {
		t.Fatal(err)
	}
	if eventCount(dir, "starts") == 1 {
		os.Exit(7)
	}
	if err := os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "clamd.sock")
	_ = os.Remove(socket)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	for {
		c, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(time.Second))
		request, _ := bufio.NewReader(c).ReadString(0)
		switch request {
		case "zPING\x00":
			_, _ = io.WriteString(c, "PONG\x00")
		case "zRELOAD\x00":
			if err := appendEvent(dir, "reloads"); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(c, "RELOADING\x00")
		}
		_ = c.Close()
	}
}

func waitFor(t *testing.T, label string, predicate func() bool) {
	t.Helper()
	until := time.Now().Add(7 * time.Second)
	for !predicate() && time.Now().Before(until) {
		time.Sleep(15 * time.Millisecond)
	}
	if !predicate() {
		t.Fatalf("timed out waiting for %s", label)
	}
}

func TestSupervisorDownloadsRestartsReloadsAndStops(t *testing.T) {
	dir := shortDir(t)
	factory := func(ctx context.Context, name string, _ ...string) *exec.Cmd {
		cmd := command(ctx, os.Args[0], "-test.run=^TestAntivirusProcessFixture$", "--", filepath.Base(name), dir)
		cmd.Env = append(os.Environ(), "PMAIL_AV_PROCESS_FIXTURE=1", "GORACE=atexit_sleep_ms=0")
		cmd.Stdout, cmd.Stderr = nil, nil
		return cmd
	}
	m := newManager(dir, false, factory)
	m.probeInterval, m.retryMin = 30*time.Millisecond, 150*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); m.run(ctx) }()
	defer func() { cancel(); <-done }()
	if err := m.RequestUpdate(); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled update: %v", err)
	}
	m.SetEnabled(true)
	waitFor(t, "daemon after synthetic crash", func() bool { return m.Snapshot().State == "running" && eventCount(dir, "starts") == 2 })
	if eventCount(dir, "updates") != 1 {
		t.Fatal("expected one bootstrap download")
	}
	conf, err := os.ReadFile(filepath.Join(dir, "clamd.conf"))
	if err != nil || strings.Contains(string(conf), "TCPSocket") || !strings.Contains(string(conf), "StreamMaxLength 33554432") {
		t.Fatalf("socket config: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("directory mode: %v", err)
	}
	if err := m.RequestUpdate(); err != nil {
		t.Fatal(err)
	}
	if err := m.RequestUpdate(); !errors.Is(err, ErrUpdateBusy) {
		t.Fatalf("concurrent update: %v", err)
	}
	waitFor(t, "one reload for a new daily file", func() bool { return !m.Snapshot().Updating && eventCount(dir, "reloads") == 1 })
	s := m.Snapshot()
	if s.Database == nil || s.Database.Version != 2 || s.LastUpdate == nil || s.LastUpdate.Status != "success" {
		t.Fatalf("update status: %+v", s)
	}
	if persisted := newManager(dir, false, factory).Snapshot().LastUpdate; persisted == nil || persisted.Status != "success" {
		t.Fatal("freshclam result did not persist across supervisor generations")
	}

	m.SetEnabled(false)
	waitFor(t, "foreground child exit", func() bool { return m.Snapshot().State == "disabled" })
	if readDatabase(dir, "main") == nil || readDatabase(dir, "daily") == nil {
		t.Fatal("disabled scanner removed databases")
	}
	m.SetEnabled(true)
	waitFor(t, "re-enabled scanner using cached signatures", func() bool { return m.Snapshot().State == "running" && eventCount(dir, "starts") == 3 })
	if eventCount(dir, "updates") != 2 {
		t.Fatal("re-enable downloaded cached signatures again")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not reap children")
	}
}
