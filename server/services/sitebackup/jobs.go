package sitebackup

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/internal/backupcatalog"
	"github.com/Jinnrry/pmail/services/maintenance"
	"github.com/Jinnrry/pmail/signal"
	log "github.com/sirupsen/logrus"
)

const UploadLimit int64 = 50 << 30

type Job struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Phase       string `json:"stage"`
	Name        string `json:"file_name,omitempty"`
	Tables      int    `json:"tables_done"`
	TotalTables int    `json:"tables_total"`
	Rows        int64  `json:"rows_done"`
	Error       string `json:"error,omitempty"`
}

type Saved struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Created    time.Time `json:"created_at"`
	Version    string    `json:"pmail_version"`
	PreRestore bool      `json:"is_pre_restore,omitempty"`
}

type Manager struct {
	mu   sync.Mutex
	busy bool
	job  *Job
}

var Jobs Manager

func ErrorCode(err error) string {
	for _, known := range []error{ErrPassphrase, ErrBadPassphrase, ErrInvalid, ErrNewer, ErrBusy, ErrNotFound, ErrTooLarge, ErrSecret, ErrConfirm, maintenance.ErrBusy} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return "backup_failed"
}

func (m *Manager) reserve() (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return nil, ErrBusy
	}
	m.busy = true
	return func() { m.mu.Lock(); m.busy = false; m.mu.Unlock() }, nil
}
func (m *Manager) Current() *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job == nil {
		return nil
	}
	snapshot := *m.job
	return &snapshot
}
func (m *Manager) RestoreStatus(id string) *Job {
	j := m.Current()
	if j == nil || j.Kind != "restore" || subtle.ConstantTimeCompare([]byte(id), []byte(j.ID)) != 1 {
		return nil
	}
	return j
}
func (m *Manager) edit(fn func(*Job)) { m.mu.Lock(); defer m.mu.Unlock(); fn(m.job) }
func (m *Manager) phase(name string) {
	m.edit(func(j *Job) { j.Phase = name; j.Tables = 0; j.Rows = 0 })
}
func (m *Manager) progress(tables int, rows int64) {
	m.edit(func(j *Job) { j.Tables = tables; j.Rows = rows })
}

func (m *Manager) start(kind, name string, run func() error) (Job, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Job{}, err
	}
	release, err := m.reserve()
	if err != nil {
		return Job{}, err
	}
	phase := "exporting"
	if kind == "restore" {
		phase = "verifying"
	}
	j := Job{ID: hex.EncodeToString(nonce[:]), Kind: kind, State: "running", Phase: phase, Name: name, TotalTables: len(backupcatalog.Tables)}
	initial := j
	m.mu.Lock()
	m.job = &j
	m.mu.Unlock()
	go func() {
		err := run()
		m.edit(func(x *Job) {
			if err != nil {
				x.State = "failed"
				x.Error = ErrorCode(err)
			} else {
				x.State = "done"
				x.Phase = "done"
				if kind == "restore" {
					x.Phase = "restarting"
				}
				x.Tables = x.TotalTables
			}
		})
		if err != nil || kind != "restore" {
			release()
		}
		// Log controlled codes only; database records, filenames derived
		// from uploaded content, passphrases and raw errors never go to logs.
		if err != nil {
			log.Warnf("audit_event=backup_%s_failed code=%s", kind, ErrorCode(err))
		} else {
			log.Infof("audit_event=backup_%s_completed", kind)
		}
		if err == nil && kind == "restore" {
			// Allow the UI to see completion before gracefully draining HTTP.
			time.Sleep(4 * time.Second)
			select {
			case signal.BackupExitChan <- struct{}{}:
			default:
			}
		}
	}()
	return initial, nil
}

func (m *Manager) StartExport(s Store, pass string, tls bool) (Job, error) {
	if err := ValidatePassphrase(pass); err != nil {
		return Job{}, err
	}
	dir, err := DataDir(s.Root, "backups")
	if err != nil {
		return Job{}, err
	}
	name := Name(s.Version, false)
	return m.start("export", name, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
		defer cancel()
		return Publish(dir, name, func(f *os.File) error { return s.Export(ctx, f, pass, tls, m.progress) })
	})
}

func (m *Manager) StartRestore(s Store, name, pass string, ready <-chan struct{}) (Job, error) {
	if err := ValidatePassphrase(pass); err != nil {
		return Job{}, err
	}
	dir, err := DataDir(s.Root, "backups")
	if err != nil {
		return Job{}, err
	}
	f, err := OpenNamed(dir, name)
	if err != nil {
		return Job{}, err
	}
	j, err := m.start("restore", name, func() error {
		defer f.Close()
		<-ready
		ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
		defer cancel()
		return s.restore(ctx, f, pass, func() (func(bool), error) {
			m.phase("entering_maintenance")
			drain, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()
			resume, err := maintenance.Default.Begin(drain)
			if err != nil {
				return nil, err
			}
			release := func(committed bool) {
				if !committed {
					resume()
				}
			}
			m.phase("pre_backup")
			if err = Publish(dir, Name(s.Version, true), func(out *os.File) error { return s.Export(ctx, out, pass, true, m.progress) }); err != nil {
				return release, err
			}
			m.phase("restoring_db")
			return release, nil
		}, m.progress, func(name string) { m.edit(func(j *Job) { j.Phase = name }) })
	})
	if err != nil {
		f.Close()
	}
	return j, err
}

func List(dir string) ([]Saved, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []Saved{}
	for _, e := range entries {
		v, t, ok := parseName(e.Name())
		if !ok || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Saved{e.Name(), info.Size(), t, v, strings.HasPrefix(e.Name(), "pre-restore-")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

func (m *Manager) Delete(dir, name string) error {
	if _, _, ok := parseName(name); !ok {
		return ErrNotFound
	}
	release, err := m.reserve()
	if err != nil {
		return err
	}
	defer release()
	f, err := OpenNamed(dir, name)
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(filepath.Join(dir, name))
}

// Receive streams the encrypted payload; only the outer header can be
// validated without the passphrase. Full validation is mandatory on restore.
func (m *Manager) Receive(dir, name string, in io.Reader) (string, error) {
	if !strings.HasSuffix(strings.ToLower(name), ".pmailbak") {
		return "", ErrInvalid
	}
	if _, _, ok := parseName(name); !ok {
		name = Name("unknown", false)
	}
	release, err := m.reserve()
	if err != nil {
		return "", err
	}
	defer release()
	if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
		name = Name("unknown", false)
	}
	var h [headerSize]byte
	if _, err := io.ReadFull(in, h[:]); err != nil || !validHeader(h[:]) {
		return "", ErrInvalid
	}
	return name, Publish(dir, name, func(f *os.File) error {
		if _, err := f.Write(h[:]); err != nil {
			return err
		}
		n, err := io.Copy(f, io.LimitReader(in, UploadLimit-int64(headerSize)+1))
		if n > UploadLimit-int64(headerSize) {
			return ErrTooLarge
		}
		if err == nil && n < 21 {
			return ErrInvalid
		}
		return err
	})
}

func (m *Manager) Cleanup(root string, now time.Time) {
	release, err := m.reserve()
	if err != nil {
		return
	}
	defer release()
	dir, err := DataDir(root, "backups")
	if err != nil {
		return
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".pmailbak-part-") || !e.Type().IsRegular() {
			continue
		}
		if i, e2 := e.Info(); e2 == nil && now.Sub(i.ModTime()) > 24*time.Hour {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	// Remove interrupted key-staging files, never the intentional .bak-* keys.
	for _, dir := range []string{filepath.Join(root, "config"), filepath.Join(root, "config", "dkim"), filepath.Join(root, "config", "ssl")} {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		files, _ := os.ReadDir(dir)
		for _, e := range files {
			st, err := e.Info()
			if err != nil || now.Sub(st.ModTime()) < 24*time.Hour {
				continue
			}
			if st.Mode().IsRegular() && strings.HasPrefix(e.Name(), ".pmail-restore-part-") {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}

}
