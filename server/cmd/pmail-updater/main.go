// pmail-updater exposes only fixed PMail Compose operations over its private API.
package main

import (
	"bytes"
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
	"slices"
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
	if !idRE.MatchString(id) {
		return j, os.ErrNotExist
	}
	root, err := os.OpenRoot(s.stateDir)
	if err != nil {
		return j, err
	}
	defer root.Close()
	b, err := root.ReadFile(id + ".json")
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

// composeConfigArgs reconstructs the two supported public installations: the
// default override filename and the explicit docker-compose.public.yml pair.
// Fail closed rather than restarting a public deployment with proxy ports.
func composeConfigArgs(root, project string, public bool) ([]string, error) {
	base := []string{"compose", "--project-name", project, "--project-directory", root, "-f", filepath.Join(root, "docker-compose.yml")}
	envFile := filepath.Join(root, ".env")
	if _, err := os.Stat(envFile); err == nil {
		base = append(base, "--env-file", envFile)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	override := filepath.Join(root, "docker-compose.override.yml")
	if !public {
		if _, err := os.Stat(override); err == nil {
			base = append(base, "-f", override)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return base, nil
	}

	folder, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer folder.Close()
	var selected string
	var selectedContent []byte
	for _, file := range []string{"docker-compose.override.yml", "docker-compose.public.yml"} {
		content, err := folder.ReadFile(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if selected == "" {
			selected, selectedContent = filepath.Join(root, file), content
		} else if !bytes.Equal(selectedContent, content) {
			return nil, errors.New("conflicting public Compose overrides; keep only the active file")
		}
	}
	if selected == "" {
		return nil, errors.New("public Compose override is missing")
	}
	return append(base, "-f", selected), nil
}

func allowedComposeCommand(args []string) bool {
	return slices.Equal(args, []string{"pull", "--policy", "always", "pmail"}) || slices.Equal(args, []string{"up", "-d", "--no-deps", "--no-build", "--wait", "--wait-timeout", "120", "pmail"})
}

func composeRunner(project string, public bool) func(context.Context, ...string) error {
	return func(ctx context.Context, args ...string) error {
		if !projectRE.MatchString(project) || !allowedComposeCommand(args) {
			return errors.New("invalid updater Compose command")
		}
		const root = "/project"
		base, err := composeConfigArgs(root, project, public)
		if err != nil {
			log.Printf("updater Compose configuration unavailable: %v", err)
			return err
		}
		cmd := exec.CommandContext(ctx, "/usr/local/bin/docker", append(base, args...)...) // #nosec G204,G702 -- absolute Docker CLI, validated project name, fixed mounted config files and exactly two allowlisted argv sequences; no shell or HTTP input.
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
	svc := &service{stateDir: "/state", project: project, token: key(secret), run: composeRunner(project, os.Getenv("PMAIL_UPDATER_PUBLIC") == "1")}
	svc.recover()
	server := &http.Server{Addr: ":8989", Handler: svc.handler(), ReadHeaderTimeout: 4 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 20 * time.Second, MaxHeaderBytes: 8192}
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("updater listener failed")
	}
}
