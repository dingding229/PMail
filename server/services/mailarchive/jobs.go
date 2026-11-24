package mailarchive

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/maintenance"
	"github.com/Jinnrry/pmail/services/sitebackup"
)

const DefaultUpload int64 = 50 << 20
const MaxUpload int64 = 512 << 20
const life = 24 * time.Hour
const maxWorkers = 4

var idFormat = regexp.MustCompile(`^[a-f0-9]{32}$`)

var Jobs Manager

type Job struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	State     string    `json:"state"`
	Created   time.Time `json:"created_at"`
	Expires   time.Time `json:"expires_at"`
	Total     int64     `json:"total"`
	Succeeded int64     `json:"imported"`
	Skipped   int64     `json:"skipped"`
	Failed    int64     `json:"failed"`
	Error     string    `json:"error_code,omitempty"`
	Expected  int64     `json:"expected,omitempty"`
	Size      int64     `json:"size,omitempty"`
	owner     int
}
type Manager struct {
	mu     sync.Mutex
	jobs   map[string]*Job
	active map[int]bool
}

func ErrorCode(err error) string {
	for _, e := range []error{ErrInvalid, ErrTarget, ErrBusy, ErrNotFound, ErrTooLarge, ErrQuota, maintenance.ErrBusy} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	return "mail_archive_failed"
}

func UploadLimit(ctx context.Context) (int64, error) {
	var row models.MailArchiveSettings
	found, err := db.Instance.Context(ctx).ID(1).Get(&row)
	if err != nil {
		return 0, err
	}
	if !found {
		return DefaultUpload, nil
	}
	if row.MaxBytes < 1<<20 || row.MaxBytes > MaxUpload {
		return DefaultUpload, nil
	}
	return row.MaxBytes, nil
}
func SetUploadLimit(ctx context.Context, n int64) error {
	if n < 1<<20 || n > MaxUpload {
		return ErrInvalid
	}
	q := "INSERT INTO mail_archive_settings (id,max_upload_bytes) VALUES (1,?) ON CONFLICT (id) DO UPDATE SET max_upload_bytes=excluded.max_upload_bytes"
	if db.Instance.DriverName() == "mysql" {
		q = "INSERT INTO mail_archive_settings (id,max_upload_bytes) VALUES (1,?) ON DUPLICATE KEY UPDATE max_upload_bytes=VALUES(max_upload_bytes)"
	}
	_, err := db.Instance.Context(ctx).Exec(q, n)
	return err
}

func UserDir(root string, owner int) (string, error) {
	if owner <= 0 {
		return "", ErrNotFound
	}
	dir, err := sitebackup.DataDir(root, "exports")
	if err != nil {
		return "", err
	}
	return privateDir(filepath.Join(dir, strconv.Itoa(owner)))
}
func privateDir(p string) (string, error) {
	if err := os.Mkdir(p, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if info, err := os.Lstat(p); err != nil || !info.IsDir() {
		return "", ErrInvalid
	}
	return p, os.Chmod(p, 0700) // #nosec G302 -- directory: owner needs execute permission.
}

func (m *Manager) begin(owner int, kind string) (Job, error) {
	if owner <= 0 {
		return Job{}, ErrNotFound
	}
	var i [16]byte
	if _, err := rand.Read(i[:]); err != nil {
		return Job{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobs == nil {
		m.jobs = map[string]*Job{}
		m.active = map[int]bool{}
	}
	if m.active[owner] || len(m.active) >= maxWorkers || len(m.jobs) >= 4096 {
		return Job{}, ErrBusy
	}
	now := time.Now().UTC()
	j := Job{ID: hex.EncodeToString(i[:]), Kind: kind, State: "running", Created: now, Expires: now.Add(life), owner: owner}
	result := j
	m.jobs[j.ID] = &j
	m.active[owner] = true
	return result, nil
}
func (m *Manager) abandon(j Job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.jobs, j.ID)
	delete(m.active, j.owner)
}
func (m *Manager) mutate(id string, fn func(*Job)) { m.mu.Lock(); defer m.mu.Unlock(); fn(m.jobs[id]) }
func (m *Manager) run(j Job, fn func(context.Context) error, cleanup ...func()) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
		defer cancel()
		for _, end := range cleanup {
			defer end()
		}
		var err error
		if !maintenance.Work(ctx, func(ctx context.Context) { err = fn(ctx) }) {
			err = maintenance.ErrBusy
		}
		m.mu.Lock()
		p := m.jobs[j.ID]
		p.Expires = time.Now().UTC().Add(life)
		if err != nil {
			p.State = "failed"
			p.Error = ErrorCode(err)
			if errors.Is(err, ErrBusy) {
				p.Error = j.Kind + "_busy"
			}
		} else {
			p.State = "done"
		}
		delete(m.active, j.owner)
		m.mu.Unlock()
	}()
}
func (m *Manager) counter(id string, result string) {
	m.mutate(id, func(j *Job) {
		j.Total++
		switch result {
		case "success":
			j.Succeeded++
		case "skipped":
			j.Skipped++
		default:
			j.Failed++
		}
	})
}
func metaPath(dir, id string) string { return filepath.Join(dir, "export-"+id+".json") }
func zipName(id string) string       { return "export-" + id + ".zip" }

func (m *Manager) StartExport(root string, owner int, folders []Folder) (Job, error) {
	dir, err := UserDir(root, owner)
	if err != nil {
		return Job{}, err
	}
	j, err := m.begin(owner, "export")
	if err != nil {
		return j, err
	}
	m.run(j, func(ctx context.Context) error {
		total, err := countExport(ctx, owner, folders)
		if err != nil {
			return err
		}
		m.mutate(j.ID, func(j *Job) { j.Expected = total })
		if err := sitebackup.Publish(dir, zipName(j.ID), func(f *os.File) error { return Export(ctx, owner, folders, f, func() { m.counter(j.ID, "success") }) }); err != nil {
			return err
		}
		m.mu.Lock()
		meta := *m.jobs[j.ID]
		m.mu.Unlock()
		meta.State = "done"
		info, err := os.Stat(filepath.Join(dir, zipName(j.ID)))
		if err != nil {
			return err
		}
		meta.Size = info.Size()
		m.mutate(j.ID, func(job *Job) { job.Size = meta.Size })
		meta.Expires = time.Now().UTC().Add(life)
		if err := sitebackup.Publish(dir, filepath.Base(metaPath(dir, j.ID)), func(f *os.File) error { return json.NewEncoder(f).Encode(meta) }); err != nil {
			_ = os.Remove(filepath.Join(dir, zipName(j.ID)))
			return err
		}
		return nil
	})
	return j, nil
}

// PrepareImport reserves the owner before reading the multipart body.
// The caller must call Abort or Submit exactly once.
func (m *Manager) PrepareImport(root string, owner int) (*Upload, error) {
	dir, err := UserDir(root, owner)
	if err != nil {
		return nil, err
	}
	j, err := m.begin(owner, "import")
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, ".import-part-")
	if err != nil {
		m.abandon(j)
		return nil, err
	}
	return &Upload{manager: m, job: j, file: f}, nil
}

type Upload struct {
	manager   *Manager
	job       Job
	file      *os.File
	ext       string
	submitted bool
}

func (u *Upload) Copy(name string, from io.Reader, limit int64) error {
	u.ext = extension(name)
	if u.ext == "" {
		return ErrInvalid
	}
	n, err := io.Copy(u.file, io.LimitReader(from, limit+1))
	if n > limit {
		return ErrTooLarge
	}
	if err == nil && n == 0 {
		return ErrInvalid
	}
	return err
}
func (u *Upload) Abort() {
	if u.submitted {
		return
	}
	u.file.Close()
	_ = os.Remove(u.file.Name())
	u.manager.abandon(u.job)
}
func (u *Upload) Submit(f Folder, bytes int64, notify func(int)) (Job, error) {
	if u.ext == "" {
		return Job{}, ErrInvalid
	}
	if err := u.file.Close(); err != nil {
		return Job{}, err
	}
	u.submitted = true
	u.manager.run(u.job, func(ctx context.Context) error {
		var added bool
		err := Import(ctx, u.job.owner, f, u.file.Name(), u.ext, bytes*10, func(state string) { u.manager.counter(u.job.ID, state); added = added || state == "success" })
		if (err == nil || added) && notify != nil {
			notify(u.job.owner)
		}
		return err
	}, func() { _ = os.Remove(u.file.Name()) })
	return u.job, nil
}

func (m *Manager) Status(root string, owner int, id, kind string) (Job, error) {
	if owner <= 0 || !idFormat.MatchString(id) {
		return Job{}, ErrNotFound
	}
	m.mu.Lock()
	p := m.jobs[id]
	var j Job
	if p != nil {
		j = *p
	}
	m.mu.Unlock()
	if p != nil {
		if j.owner != owner || j.Kind != kind || !j.Expires.After(time.Now()) {
			return Job{}, ErrNotFound
		}
		return j, nil
	}
	if kind != "export" {
		return Job{}, ErrNotFound
	}
	dir, err := UserDir(root, owner)
	if err != nil {
		return Job{}, ErrNotFound
	}
	if info, err := os.Lstat(metaPath(dir, id)); err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return Job{}, ErrNotFound
	}
	b, err := os.ReadFile(metaPath(dir, id))
	if err != nil || json.Unmarshal(b, &j) != nil || j.ID != id || j.Kind != kind || j.State != "done" || !j.Expires.After(time.Now()) {
		return Job{}, ErrNotFound
	}
	return j, nil
}
func (m *Manager) OpenExport(root string, owner int, id string) (*os.File, error) {
	j, err := m.Status(root, owner, id, "export")
	if err != nil || j.State != "done" {
		return nil, ErrNotFound
	}
	dir, err := UserDir(root, owner)
	if err != nil {
		return nil, ErrNotFound
	}
	path := filepath.Join(dir, zipName(id))
	if st, err := os.Lstat(path); err != nil || !st.Mode().IsRegular() {
		return nil, ErrNotFound
	}
	return os.Open(path) // #nosec G304 -- id matched idFormat; regular file checked with Lstat.
}

func (m *Manager) Cleanup(root string, now time.Time) {
	dir, err := sitebackup.DataDir(root, "exports")
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, j := range m.jobs {
		if j.State != "running" && !j.Expires.After(now) {
			delete(m.jobs, id)
		}
	}
	users, _ := os.ReadDir(dir)
	for _, user := range users {
		id, err := strconv.Atoi(user.Name())
		if err != nil || id <= 0 || !user.IsDir() || m.active[id] {
			continue
		}
		p := filepath.Join(dir, user.Name())
		entries, _ := os.ReadDir(p)
		for _, e := range entries {
			n := e.Name()
			known := strings.HasPrefix(n, "export-") || strings.HasPrefix(n, ".import-part-") || strings.HasPrefix(n, ".pmailbak-part-")
			if i, err := e.Info(); known && err == nil && i.Mode().IsRegular() && now.Sub(i.ModTime()) >= life {
				_ = os.Remove(filepath.Join(p, n))
			}
		}
	}
}
