package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/internal/releasebundle"
	log "github.com/sirupsen/logrus"
)

const execMarker = "_PMAIL_EXEC_STAGED_VERSION"
const maxBootAttempts = 3
const healthyFor = time.Minute
const bootDeadline = 3 * time.Minute

var runningStage string // set once, before config.Init or any listener goroutine
var healthConfirmed = make(chan struct{})
var confirmOnce sync.Once

// Boot runs only on normal program startup, not on maintenance subcommands.
func Boot(version string) error {
	in := runtimeInstaller()
	marker := os.Getenv(execMarker)
	if marker != "" {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		if err := in.validateHandoff(version, marker, executable); err != nil {
			return err
		}
		if err := config.UseStagedWebRoot(marker); err != nil {
			return err
		}
		runningStage = marker
		go bootWatchdog(healthConfirmed, bootDeadline, func() {
			log.Errorf("staged PMail %s did not become healthy before the startup deadline", marker)
			os.Exit(1)
		})
		return nil
	}
	if !in.supported || rank(canonicalVersion(version)) < 0 {
		return nil
	}
	if err := in.bootImage(version, config.ROOT_PATH, os.Args, os.Environ(), execUpdate); err != nil {
		// An invalid stage should not prevent the immutable image from starting.
		log.Warnf("self-update startup: %v; using image binary", err)
	}
	return nil
}

func (in installer) validateHandoff(version, marker, executable string) error {
	if !in.enabled(version) || rank(marker) < 0 || marker != canonicalVersion(version) ||
		filepath.Clean(executable) != filepath.Join(in.store.version(marker), "pmail") {
		return errors.New("invalid staged exec handoff")
	}
	return nil
}

type execFunc func(string, []string, []string) error

func (in installer) bootImage(version, configRoot string, args, env []string, launch execFunc) error {
	s := in.store
	if err := s.verifyDirs(); errors.Is(err, os.ErrNotExist) {
		return s.finishInterrupted("")
	} else if err != nil {
		return err
	}
	current, err := s.current()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			if removeErr := s.clearPointer(); removeErr != nil {
				return removeErr
			}
		}
		return errors.Join(s.finishInterrupted(""), s.cleanStages("", true))
	}
	image := canonicalVersion(version)
	if rank(current) <= rank(image) {
		if err := s.clearPointer(); err != nil {
			return err
		}
		if err := s.finishBoot(current, jobDone, ""); err != nil {
			return err
		}
		return errors.Join(s.finishInterrupted(""), s.cleanStages("", true))
	}
	if err := s.finishInterrupted(current); err != nil {
		return err
	}
	if err := s.cleanStages(current, true); err != nil {
		return err
	}
	if config.Flag("PMAIL_DISABLE_STAGED_UPDATE") {
		j, err := s.readJob()
		if err != nil {
			return err
		}
		if j.active() && j.Version == current {
			return s.saveJob(changedJob(j, jobFailed, "update_install_disabled"))
		}
		return nil
	}
	if !in.enabled(version) {
		return nil
	}
	if err := privateDirectory(s.version(current)); err != nil {
		return s.abandon(current, "update_bundle_invalid", err)
	}
	attempts, err := s.attempts(current)
	if err != nil {
		return s.abandon(current, "update_rollback", err)
	}
	if attempts >= maxBootAttempts {
		return s.abandon(current, "update_rollback", fmt.Errorf("PMail %s did not stay healthy after %d starts", current, attempts))
	}
	if err := in.verifyStage(current); err != nil {
		return s.abandon(current, "update_bundle_invalid", errors.New("staged bundle failed startup verification"))
	}
	if err := s.writeAttempts(current, attempts+1); err != nil {
		return err
	}
	if err := s.finishBoot(current, jobRestarting, ""); err != nil {
		return err
	}
	executable := filepath.Join(s.version(current), "pmail")
	args = append([]string{executable}, args[1:]...)
	log.Infof("starting verified staged PMail %s (attempt %d/%d)", current, attempts+1, maxBootAttempts)
	if err := launch(executable, args, stagedEnvironment(env, configRoot, current)); err != nil {
		return s.abandon(current, "update_rollback", fmt.Errorf("could not exec PMail %s: %w", current, err))
	}
	return nil // An actual successful exec never returns.
}

func (in installer) verifyStage(tag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root, err := os.OpenRoot(in.store.version(tag))
	if err != nil {
		return err
	}
	defer root.Close()
	manifest, err := readFixed(root, releasebundle.Checksums, releasebundle.MaxManifest)
	if err != nil {
		return err
	}
	sig, err := readFixed(root, releasebundle.Signature, 256)
	if err != nil {
		return err
	}
	hash, err := releasebundle.Verify(in.key, tag, in.arch, manifest, sig)
	if err != nil {
		return err
	}
	archive, err := openRegular(root, archiveName)
	if err != nil {
		return err
	}
	defer archive.Close()
	if err := verifyArchive(archive, hash); err != nil {
		return err
	}
	return verifyInstalled(ctx, root, archive)
}

func stagedEnvironment(env []string, root, tag string) []string {
	result := make([]string, 0, len(env)+2)
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if name != execMarker && name != "PMail_ROOT" {
			result = append(result, entry)
		}
	}
	return append(result, "PMail_ROOT="+root, execMarker+"="+tag)
}

func (s store) finishInterrupted(keep string) error {
	j, err := s.readJob()
	if err != nil || !j.active() || j.Version == keep {
		return err
	}
	return s.saveJob(changedJob(j, jobFailed, "update_job_interrupted"))
}

func (s store) finishBoot(tag, state, message string) error {
	j, err := s.readJob()
	if err != nil || j.JobID == "" || j.Version != tag {
		return err
	}
	return s.saveJob(changedJob(j, state, message))
}

func (s store) clearPointer() error {
	if err := os.Remove(s.pointer()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(s.staged())
}

func (s store) abandon(tag, reason string, cause error) error {
	// Persist the reason before dropping the pointer, so a crash between
	// writes can still surface the fallback to the administrator.
	persist := s.finishBoot(tag, jobFailed, reason)
	if err := s.clearPointer(); err != nil {
		return errors.Join(cause, persist, err)
	}
	root, err := os.OpenRoot(s.staged())
	if err != nil {
		return errors.Join(cause, persist, err)
	}
	defer root.Close()
	return errors.Join(cause, persist, root.RemoveAll(tag))
}

func (s store) cleanStages(keep string, removeIncoming bool) error {
	root, err := os.OpenRoot(s.staged())
	if err != nil {
		return err
	}
	defer root.Close()
	dirs, err := os.ReadDir(s.staged())
	if err != nil {
		return err
	}
	for _, entry := range dirs {
		name := entry.Name()
		if (removeIncoming && strings.HasPrefix(name, ".incoming-")) || (rank(name) >= 0 && name != keep) {
			if err := root.RemoveAll(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// StartHealth runs only after normal listeners have bound, not while the
// domain setup listener is serving ping. Brief listener reloads reset the window.
func StartHealth(ctx context.Context, port int) <-chan struct{} {
	done := make(chan struct{})
	if runningStage == "" {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		in := runtimeInstaller()
		waitHealthy(ctx, 5*time.Second, healthyFor, func(ctx context.Context) bool { return Probe(ctx, port) == nil }, func() bool {
			if err := in.store.markHealthy(runningStage); err != nil {
				log.Warnf("could not record healthy staged update: %v", err)
				return false
			}
			confirmOnce.Do(func() { close(healthConfirmed) })
			return true
		})
	}()
	return done
}

// Docker does not restart merely because its probe is unhealthy. A staged
// process that never becomes ready must exit to consume another attempt.
func bootWatchdog(healthy <-chan struct{}, deadline time.Duration, expire func()) {
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case <-healthy:
	case <-timer.C:
		select {
		case <-healthy:
		default:
			expire()
		}
	}
}

func waitHealthy(ctx context.Context, interval, duration time.Duration, probe func(context.Context) bool, mark func() bool) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	var since time.Time
	for ctx.Err() == nil {
		if !probe(ctx) {
			since = time.Time{}
		} else if since.IsZero() {
			since = time.Now()
		}
		if !since.IsZero() && time.Since(since) >= duration && ctx.Err() == nil && mark() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s store) markHealthy(tag string) error {
	jobMu.Lock()
	defer jobMu.Unlock()
	current, err := s.current()
	if err != nil || current != tag {
		return err
	}
	if err := s.writeAttempts(tag, 0); err != nil {
		return err
	}
	if err := s.finishBoot(tag, jobDone, ""); err != nil {
		return err
	}
	return s.cleanStages(tag, false)
}

func Probe(ctx context.Context, port int) error {
	if port == 0 {
		port = 80
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/ping", port), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned HTTP %d", resp.StatusCode)
	}
	return nil
}
