// pmail-updater exposes only fixed PMail Compose operations over its private API.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var projectRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var idRE = regexp.MustCompile(`^[a-f0-9]{32}$`)

type job struct {
	State   string  `json:"state"`
	Message *string `json:"message"`
}
type service struct {
	mu                sync.Mutex
	stateDir, project string
	token             [32]byte
	running           bool
	run               func(context.Context, ...string) error
}

func key(s string) [32]byte { return sha256.Sum256([]byte(s)) }
func (s *service) save(id, state, message string) error {
	j := job{State: state}
	if message != "" {
		j.Message = &message
	}
	b, _ := json.Marshal(j)
	name := filepath.Join(s.stateDir, id+".json")
	tmp, err := os.CreateTemp(s.stateDir, "job-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}
func (s *service) read(id string) (job, error) {
	var j job
	b, err := os.ReadFile(filepath.Join(s.stateDir, id+".json"))
	if err != nil {
		return j, err
	}
	err = json.Unmarshal(b, &j)
	return j, err
}
func (s *service) recover() {
	entries, _ := os.ReadDir(s.stateDir)
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		if id == e.Name() || !idRE.MatchString(id) {
			continue
		}
		j, err := s.read(id)
		if err != nil {
			continue
		}
		if j.State == "pending" || j.State == "pulling" || j.State == "restarting" {
			_ = s.save(id, "failed", "update_job_interrupted")
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 7*24*time.Hour {
			_ = os.Remove(filepath.Join(s.stateDir, e.Name()))
		}
	}
}
func (s *service) process(id string) {
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	if err := s.save(id, "pulling", ""); err != nil {
		return
	}
	if err := s.run(ctx, "pull", "--policy", "always", "pmail"); err != nil {
		_ = s.save(id, "failed", "update_pull_failed")
		return
	}
	if err := s.save(id, "restarting", ""); err != nil {
		return
	}
	if err := s.run(ctx, "up", "-d", "--no-deps", "--no-build", "--wait", "--wait-timeout", "120", "pmail"); err != nil {
		_ = s.save(id, "failed", "update_restart_failed")
		return
	}
	_ = s.save(id, "done", "")
}
func (s *service) handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("POST /jobs", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.running {
			w.WriteHeader(409)
			return
		}
		s.recover()
		if r.ContentLength != 0 {
			w.WriteHeader(400)
			return
		}
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			w.WriteHeader(500)
			return
		}
		id := hex.EncodeToString(raw[:])
		if err := s.save(id, "pending", ""); err != nil {
			w.WriteHeader(500)
			return
		}
		s.running = true
		go s.process(id)
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(map[string]string{"job_id": id})
	})
	m.HandleFunc("GET /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !idRE.MatchString(id) {
			w.WriteHeader(404)
			return
		}
		j, err := s.read(id)
		if err != nil {
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(j)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || len(token) > 256 {
			w.WriteHeader(401)
			return
		}
		sum := key(token)
		if subtle.ConstantTimeCompare(sum[:], s.token[:]) != 1 {
			w.WriteHeader(401)
			return
		}
		m.ServeHTTP(w, r)
	})
}
func composeRunner(project string) func(context.Context, ...string) error {
	return func(ctx context.Context, args ...string) error {
		const root = "/project"
		base := []string{"compose", "--project-name", project, "--project-directory", root, "-f", root + "/docker-compose.yml"}
		if _, err := os.Stat(root + "/.env"); err == nil {
			base = append(base, "--env-file", root+"/.env")
		}
		if _, err := os.Stat(root + "/docker-compose.override.yml"); err == nil {
			base = append(base, "-f", root+"/docker-compose.override.yml")
		}
		cmd := exec.CommandContext(ctx, "docker", append(base, args...)...)
		cmd.Dir = root
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		// Use only the local daemon and deployment files. No inherited Compose
		// overrides or remote Docker endpoints can redirect this job.
		cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/root", "DOCKER_HOST=unix:///var/run/docker.sock"}
		return cmd.Run()
	}
}
func main() {
	secret := os.Getenv("PMAIL_UPDATER_TOKEN")
	project := os.Getenv("PMAIL_UPDATER_PROJECT")
	if len(secret) < 32 || len(secret) > 256 || !projectRE.MatchString(project) {
		log.Fatal("updater requires a 32+ character token and a valid Compose project")
	}
	if err := os.MkdirAll("/state", 0700); err != nil {
		log.Fatal("updater state directory unavailable")
	}
	svc := &service{stateDir: "/state", project: project, token: key(secret), run: composeRunner(project)}
	svc.recover()
	server := &http.Server{Addr: ":8989", Handler: svc.handler(), ReadHeaderTimeout: 4 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 20 * time.Second, MaxHeaderBytes: 8192}
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("updater listener failed")
	}
}
