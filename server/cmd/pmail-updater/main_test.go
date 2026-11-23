package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOnlyFixedPMailCommandsAndDurableJob(t *testing.T) {
	token := "fixture-token-not-used-outside-tests"
	calls := make(chan []string, 2)
	s := &service{stateDir: t.TempDir(), project: "fixture", token: key(token), run: func(_ context.Context, args ...string) error { calls <- args; return nil }}
	m := s.handler()
	call := func(method, path, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		m.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		method, path, bearer string
		want                 int
	}{
		{http.MethodPost, "/jobs", "", http.StatusUnauthorized},
		{http.MethodPost, "/jobs", "wrong-fixture-token", http.StatusUnauthorized},
		{http.MethodGet, "/jobs", token, http.StatusMethodNotAllowed},
		{http.MethodPost, "/jobs/11111111111111111111111111111111", token, http.StatusMethodNotAllowed},
	} {
		if w := call(tc.method, tc.path, tc.bearer); w.Code != tc.want {
			t.Fatalf("%s %s: got %d, want %d: %s", tc.method, tc.path, w.Code, tc.want, w.Body)
		}
	}
	w := call(http.MethodPost, "/jobs", token)
	var body map[string]string
	if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &body) != nil || !idRE.MatchString(body["job_id"]) {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	for _, expected := range [][]string{{"pull", "--policy", "always", "pmail"}, {"up", "-d", "--no-deps", "--no-build", "--wait", "--wait-timeout", "120", "pmail"}} {
		select {
		case got := <-calls:
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("command %v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("no command")
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		w := call(http.MethodGet, "/jobs/"+body["job_id"], token)
		var j job
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &j) != nil {
			t.Fatalf("get job: %d %s", w.Code, w.Body)
		}
		if j.State == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish: %s", w.Body)
		}
		time.Sleep(time.Millisecond)
	}
	id := "11111111111111111111111111111111"
	if err := s.save(id, "restarting", ""); err != nil {
		t.Fatal(err)
	}
	s.recover()
	if j, _ := s.read(id); j.State != "failed" {
		t.Fatalf("interrupted job=%+v", j)
	}
}

func TestComposeConfigArgsPublicDeployment(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "docker-compose.yml")
	envFile := filepath.Join(root, ".env")
	alias := filepath.Join(root, "docker-compose.override.yml")
	named := filepath.Join(root, "docker-compose.public.yml")
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(base, "services: {}\n")
	write(envFile, "PMAIL_DOMAIN=example.test\n")
	want := []string{"compose", "--project-name", "fixture", "--project-directory", root, "-f", base, "--env-file", envFile}
	check := func(public bool, expected []string) {
		t.Helper()
		got, err := composeConfigArgs(root, "fixture", public)
		if err != nil || !reflect.DeepEqual(got, expected) {
			t.Fatalf("public=%t args=%v want %v err=%v", public, got, expected, err)
		}
	}
	check(false, want)
	if _, err := composeConfigArgs(root, "fixture", true); err == nil {
		t.Fatal("must not fall back to proxy ports")
	}
	write(named, "# public\n")
	check(false, want)
	check(true, append(append([]string(nil), want...), "-f", named))
	write(alias, "# public\n")
	check(true, append(append([]string(nil), want...), "-f", alias))
	write(alias, "# different\n")
	if _, err := composeConfigArgs(root, "fixture", true); err == nil {
		t.Fatal("ambiguous explicit/default public override should fail closed")
	}
	if err := os.Remove(named); err != nil {
		t.Fatal(err)
	}
	check(true, append(append([]string(nil), want...), "-f", alias))
}
