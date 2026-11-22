package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestOnlyFixedPMailCommandsAndDurableJob(t *testing.T) {
	token := "fixture-token-not-used-outside-tests"
	calls := make(chan []string, 2)
	s := &service{stateDir: t.TempDir(), project: "fixture", token: key(token), run: func(_ context.Context, args ...string) error { calls <- args; return nil }}
	m := s.handler()
	call := func(method, path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if auth {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		m.ServeHTTP(w, r)
		return w
	}
	if w := call(http.MethodPost, "/jobs", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := call(http.MethodPost, "/jobs", true)
	var body map[string]string
	if w.Code != 202 || json.Unmarshal(w.Body.Bytes(), &body) != nil || !idRE.MatchString(body["job_id"]) {
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
	for time.Now().Before(deadline) {
		j, _ := s.read(body["job_id"])
		if j.State == "done" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if j, err := s.read(body["job_id"]); err != nil || j.State != "done" {
		t.Fatalf("state %+v %v", j, err)
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
