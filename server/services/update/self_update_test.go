package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/internal/releasebundle"
)

const testOld = "0.0.3"
const testNew = "v0.0.4"

const testExecutable = "#!/bin/sh\nprintf 'staged version=%s root=%s arg=%s\\n' \"$_PMAIL_EXEC_STAGED_VERSION\" \"$PMail_ROOT\" \"$1\"\n"

func makeBundle(t *testing.T, name, body string, kind byte) []byte {
	t.Helper()
	var dst bytes.Buffer
	gz := gzip.NewWriter(&dst)
	tr := tar.NewWriter(gz)
	for _, item := range []struct {
		name, data string
		kind       byte
	}{
		{"pmail", testExecutable, tar.TypeReg}, {"web/index.html", "<h1>PMail</h1>", tar.TypeReg}, {name, body, kind},
	} {
		if item.name == "" {
			continue
		}
		h := &tar.Header{Name: item.name, Mode: 0777, Typeflag: item.kind, Size: int64(len(item.data))}
		if item.kind == tar.TypeSymlink {
			h.Linkname = "../pmail"
		}
		if err := tr.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if item.data != "" {
			if _, err := tr.Write([]byte(item.data)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return dst.Bytes()
}

func signedFixture(t *testing.T, tag string, archive []byte) (ed25519.PublicKey, []byte, []byte) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body, err := releasebundle.Manifest(tag, sha256.Sum256(archive), sha256.Sum256(archive))
	if err != nil {
		t.Fatal(err)
	}
	return pub, body, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, body)) + "\n")
}

func makeInstaller(t *testing.T, key ed25519.PublicKey, restart func()) (installer, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	return installer{store: storeAt(root), key: key, arch: "amd64", supported: true, restart: restart}, root
}

func newTLSRelease(t *testing.T, in installer, body, sig, archive []byte, wait func()) *Checker {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			assets := []map[string]string{}
			for _, name := range []string{releasebundle.Checksums, releasebundle.Signature, releasebundle.Asset(in.arch)} {
				assets = append(assets, map[string]string{"name": name, "browser_download_url": srv.URL + "/release/" + name})
			}
			if err := json.NewEncoder(w).Encode([]map[string]any{{"tag_name": testNew, "assets": assets, "body": "Notes"}}); err != nil {
				t.Error(err)
			}
		case "/release/" + releasebundle.Checksums:
			_, _ = w.Write(body)
		case "/release/" + releasebundle.Signature:
			_, _ = w.Write(sig)
		case "/release/" + releasebundle.Asset(in.arch):
			if wait != nil {
				wait()
			}
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := newChecker(testOld, srv.Client().Transport, srv.URL+"/releases")
	c.installer = in
	return c
}

func TestHTTPSReleaseCheckStageAndBoot(t *testing.T) {
	t.Setenv("PMAIL_UPDATE_CHECK", "1")
	t.Setenv("PMAIL_DISABLE_STAGED_UPDATE", "")
	archive := makeBundle(t, "web/assets/app.js", "export {}", tar.TypeReg)
	key, manifest, sig := signedFixture(t, testNew, archive)
	restarting := make(chan struct{}, 1)
	in, root := makeInstaller(t, key, func() { restarting <- struct{}{} })
	entered, proceed := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-proceed:
		default:
			close(proceed)
		}
	}()
	c := newTLSRelease(t, in, manifest, sig, archive, func() { close(entered); <-proceed })
	if err := c.Refresh(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	info := c.Info()
	if !info.HasUpdate || !info.Updater.Enabled || info.Latest == nil || *info.Latest != testNew {
		t.Fatalf("checked release: %+v", info)
	}
	if _, err := c.Apply(t.Context(), "v0.0.5"); !errors.Is(err, ErrStale) {
		t.Fatalf("did not reject stale confirmation: %v", err)
	}
	ref, err := c.Apply(t.Context(), testNew)
	if err != nil {
		t.Fatal(err)
	}
	await(t, entered)
	if job, err := c.Status(ref.JobID); err != nil || job.State != jobDownloading {
		t.Fatalf("stage progress: %+v (%v)", job, err)
	}
	if _, err := c.Apply(t.Context(), testNew); !errors.Is(err, ErrBusy) {
		t.Fatalf("concurrent job: %v", err)
	}
	close(proceed)
	await(t, restarting)
	if got, err := in.store.current(); err != nil || got != testNew {
		t.Fatalf("current = %q, %v", got, err)
	}
	if job := c.Info().Job; job == nil || job.JobID != ref.JobID || job.State != jobRestarting {
		t.Fatalf("persistent progress: %+v", job)
	}
	launch := func(binary string, args, env []string) error {
		if binary != filepath.Join(in.store.version(testNew), "pmail") || args[0] != binary || args[1] != "argument" {
			t.Fatalf("boot selection: %q %q", binary, args)
		}
		if !slices.Contains(env, execMarker+"="+testNew) || !slices.Contains(env, "PMail_ROOT="+root) || slices.Contains(env, execMarker+"=obsolete") {
			t.Fatalf("exec environment: %v", env)
		}
		if err := in.validateHandoff("0.0.4", testNew, binary); err != nil {
			t.Fatal(err)
		}
		if err := in.validateHandoff(testOld, testNew, binary); err == nil {
			t.Fatal("wrong child version accepted")
		}
		if err := in.validateHandoff("0.0.4", testNew, filepath.Join(root, "pmail")); err == nil {
			t.Fatal("image path accepted as staged handoff")
		}
		return nil
	}
	if err := in.bootImage(testOld, root, []string{"pmail", "argument"}, []string{execMarker + "=obsolete"}, launch); err != nil {
		t.Fatal(err)
	}
	if got, err := in.store.attempts(testNew); err != nil || got != 1 {
		t.Fatalf("attempt count: %d, %v", got, err)
	}
	if err := in.store.markHealthy(testNew); err != nil {
		t.Fatal(err)
	}
	if got, err := in.store.attempts(testNew); err != nil || got != 0 {
		t.Fatalf("attempt count after health: %d, %v", got, err)
	}
	if j, err := c.Status(ref.JobID); err != nil || j.State != jobDone {
		t.Fatalf("post-boot job: %+v, %v", j, err)
	}
	if err := in.bootImage("0.0.5", root, []string{"pmail"}, nil, forbidExec(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(in.store.version(testNew)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old stage survived image upgrade: %v", err)
	}
	if _, err := in.store.current(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("obsolete pointer survived: %v", err)
	}
}

func TestRejectTamperedRelease(t *testing.T) {
	t.Setenv("PMAIL_UPDATE_CHECK", "1")
	t.Setenv("PMAIL_DISABLE_STAGED_UPDATE", "")
	original := makeBundle(t, "", "", 0)
	key, manifest, sig := signedFixture(t, testNew, original)
	for _, tc := range []struct {
		name               string
		archive, body, sig []byte
		code               string
	}{
		{"archive", append(slices.Clone(original), 'x'), manifest, sig, "update_checksum_invalid"},
		{"manifest", original, bytes.Replace(manifest, []byte(testNew), []byte("v0.0.5"), 1), sig, "update_signature_invalid"},
		{"signature", original, manifest, []byte(base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))), "update_signature_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restart := make(chan struct{}, 1)
			in, _ := makeInstaller(t, key, func() { restart <- struct{}{} })
			c := newTLSRelease(t, in, tc.body, tc.sig, tc.archive, nil)
			if err := c.Refresh(t.Context(), false); err != nil {
				t.Fatal(err)
			}
			ref, err := c.Apply(t.Context(), testNew)
			if err != nil {
				t.Fatal(err)
			}
			waitFor(t, func() bool {
				j, err := c.Status(ref.JobID)
				if err != nil {
					t.Fatal(err)
				}
				if j.State != jobFailed {
					return false
				}
				if j.Message == nil || *j.Message != tc.code {
					t.Fatalf("rejection: %+v", j)
				}
				return true
			})
			select {
			case <-restart:
				t.Fatal("unverified release requested restart")
			default:
			}
			if _, err := in.store.current(); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unverified pointer: %v", err)
			}
		})
	}
	// A valid signature still cannot nominate a different target version.
	if _, err := releasebundle.Verify(key, "v0.0.5", "amd64", manifest, sig); !errors.Is(err, releasebundle.ErrSignature) {
		t.Fatalf("version change verified: %v", err)
	}
}

func TestArchiveLimitsAndRestrictedPaths(t *testing.T) {
	for _, tc := range []struct {
		name, entry, data string
		kind              byte
		limit             int64
	}{
		{"traversal", "web/../../outside", "x", tar.TypeReg, maxExtracted},
		{"absolute", "/outside", "x", tar.TypeReg, maxExtracted},
		{"symbolic-link", "web/link", "", tar.TypeSymlink, maxExtracted},
		{"duplicate", "web/index.html", "x", tar.TypeReg, maxExtracted},
		{"expanded-size", "web/large", strings.Repeat("x", 1024), tar.TypeReg, 512},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := unpackBundle(t.Context(), root, bytes.NewReader(makeBundle(t, tc.entry, tc.data, tc.kind)), tc.limit); err == nil {
				t.Fatal("accepted disallowed archive")
			}
		})
	}
}

func seedStage(t *testing.T) (installer, string, Job) {
	t.Helper()
	archive := makeBundle(t, "", "", 0)
	key, manifest, sig := signedFixture(t, testNew, archive)
	in, root := makeInstaller(t, key, nil)
	s := in.store
	if err := s.init(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.version(testNew), 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.OpenRoot(s.version(testNew))
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err := unpackBundle(t.Context(), dir, bytes.NewReader(archive), maxExtracted); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{archiveName: archive, releasebundle.Checksums: manifest, releasebundle.Signature: sig} {
		if err := dir.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := atomicState(s.pointer(), []byte(testNew)); err != nil {
		t.Fatal(err)
	}
	j := Job{JobRef: JobRef{strings.Repeat("a", 32)}, Version: testNew, State: jobRestarting}
	if err := s.saveJob(j); err != nil {
		t.Fatal(err)
	}
	return in, root, j
}

func TestBootRollsBackAfterThreeUnhealthyAttempts(t *testing.T) {
	t.Setenv("PMAIL_DISABLE_STAGED_UPDATE", "")
	in, root, _ := seedStage(t)
	var launched int
	for range maxBootAttempts {
		if err := in.bootImage(testOld, root, []string{"pmail"}, nil, func(string, []string, []string) error { launched++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if err := in.bootImage(testOld, root, []string{"pmail"}, nil, forbidExec(t)); err == nil {
		t.Fatal("rollback not reported")
	}
	if j, err := in.store.readJob(); err != nil || j.State != jobFailed || j.Message == nil || *j.Message != "update_rollback" {
		t.Fatalf("rollback record: %+v %v", j, err)
	}
	if _, err := in.store.current(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rolled back pointer remains: %v", err)
	}
	if launched != 3 {
		t.Fatalf("launched %d times", launched)
	}
}

func TestBootRejectsModifiedFilesAndHonorsDisableSwitch(t *testing.T) {
	t.Setenv("PMAIL_DISABLE_STAGED_UPDATE", "1")
	in, root, _ := seedStage(t)
	if err := in.bootImage(testOld, root, []string{"pmail"}, nil, forbidExec(t)); err != nil {
		t.Fatal(err)
	}
	if n, err := in.store.attempts(testNew); n != 0 || err != nil {
		t.Fatalf("disabled update used a boot attempt: %d %v", n, err)
	}
	t.Setenv("PMAIL_DISABLE_STAGED_UPDATE", "0")
	if err := os.WriteFile(filepath.Join(in.store.version(testNew), "web", "unlisted.txt"), []byte("extra"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := in.bootImage(testOld, root, []string{"pmail"}, nil, forbidExec(t)); err == nil {
		t.Fatal("modified stage accepted")
	}
	if job, err := in.store.readJob(); err != nil || job.State != jobFailed || job.Message == nil || *job.Message != "update_bundle_invalid" {
		t.Fatalf("invalid stage outcome: %+v %v", job, err)
	}
}

func TestDownloadRequiresHTTPSAndTrustedRedirect(t *testing.T) {
	in, _ := makeInstaller(t, nil, nil)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Host+"/insecure", http.StatusFound)
	}))
	defer srv.Close()
	c := newChecker(testOld, srv.Client().Transport, srv.URL+"/releases")
	c.installer = in
	if _, err := downloadSmall(t.Context(), c.downloadClient(), srv.URL, 256); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("HTTPS downgrade: %v", err)
	}
	if _, err := downloadSmall(t.Context(), c.downloadClient(), "http://example.test", 256); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("plain HTTP download: %v", err)
	}
}

func TestStagedStartupWatchdog(t *testing.T) {
	ch := make(chan struct{})
	close(ch)
	bootWatchdog(ch, time.Second, func() { t.Error("healthy program was stopped") })
	called := false
	bootWatchdog(make(chan struct{}), time.Millisecond, func() { called = true })
	if !called {
		t.Fatal("unhealthy program was not stopped")
	}
}

func TestHealthWindowDoesNotCountUnhealthyTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var goodSince time.Time
	var probes int
	waitHealthy(ctx, time.Millisecond, 8*time.Millisecond, func(context.Context) bool {
		probes++
		if probes < 5 {
			return false
		}
		if goodSince.IsZero() {
			goodSince = time.Now()
		}
		return true
	}, func() bool {
		if time.Since(goodSince) < 8*time.Millisecond {
			t.Error("unhealthy time was included in confirmation window")
		}
		return true
	})
	if probes < 5 || ctx.Err() != nil {
		t.Fatal("did not confirm healthy startup")
	}
}

func forbidExec(t *testing.T) execFunc {
	t.Helper()
	return func(path string, _ []string, _ []string) error {
		t.Errorf("unexpected exec of %q", path)
		return fmt.Errorf("unexpected exec on %s", runtime.GOOS)
	}
}

func await(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for update phase")
	}
}
func waitFor(t *testing.T, check func() bool) {
	t.Helper()
	for expire := time.Now().Add(5 * time.Second); time.Now().Before(expire); time.Sleep(10 * time.Millisecond) {
		if check() {
			return
		}
	}
	t.Fatal("timed out waiting for update result")
}
