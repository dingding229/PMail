package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/Jinnrry/pmail/internal/releasebundle"
	"github.com/Jinnrry/pmail/utils/securefile"
)

var jobID = regexp.MustCompile(`^[a-f0-9]{32}$`)

const (
	jobPending     = "pending"
	jobDownloading = "downloading"
	jobVerifying   = "verifying"
	jobStaging     = "staging"
	jobRestarting  = "restarting"
	jobDone        = "done"
	jobFailed      = "failed"
)

type JobRef struct {
	JobID string `json:"job_id"`
}
type Job struct {
	JobRef
	Version string  `json:"version"`
	State   string  `json:"state"`
	Message *string `json:"message"`
}

func (j Job) active() bool { return j.JobID != "" && j.State != jobDone && j.State != jobFailed }

type store struct{ base string }

var jobMu sync.Mutex // protects lifecycle/state transitions across listener reloads in this process

func storeAt(root string) store           { return store{base: filepath.Join(root, "config", "update")} }
func (s store) staged() string            { return filepath.Join(s.base, "staged") }
func (s store) pointer() string           { return filepath.Join(s.staged(), "current") }
func (s store) version(tag string) string { return filepath.Join(s.staged(), tag) }

func privateDirectory(name string) error {
	info, err := os.Lstat(name) // #nosec G703 -- paths are fixed names under the administrator's private config volume.
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("update directory must be private: %s", name)
	}
	return nil
}

func (s store) verifyDirs() error {
	if err := privateDirectory(s.base); err != nil {
		return err
	}
	return privateDirectory(s.staged())
}

func (s store) init() error {
	for _, dir := range []string{s.base, s.staged()} {
		if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := privateDirectory(dir); err != nil {
			return err
		}
	}
	return nil
}

func (s store) readJob() (Job, error) {
	var result Job
	if err := privateDirectory(s.base); errors.Is(err, os.ErrNotExist) {
		return result, nil
	} else if err != nil {
		return result, err
	}
	data, err := securefile.Read(filepath.Join(s.base, "job.json"))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(data, &result); err != nil || !jobID.MatchString(result.JobID) || releasebundle.Rank(result.Version) < 0 {
		return Job{}, errors.New("invalid update job state")
	}
	return result, nil
}

func (s store) saveJob(j Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return atomicState(filepath.Join(s.base, "job.json"), data)
}

func (s store) current() (string, error) {
	if err := s.verifyDirs(); err != nil {
		return "", err
	}
	data, err := securefile.Read(s.pointer())
	if err != nil {
		return "", err
	}
	tag := strings.TrimSpace(string(data))
	if releasebundle.Rank(tag) < 0 {
		return "", errors.New("invalid staged update version")
	}
	return tag, nil
}

func (s store) attempts(tag string) (int, error) {
	data, err := securefile.Read(filepath.Join(s.version(tag), "attempts"))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(string(data))
	if err != nil || n < 0 {
		return 0, errors.New("invalid update attempts")
	}
	return n, nil
}

func (s store) writeAttempts(tag string, n int) error {
	return atomicState(filepath.Join(s.version(tag), "attempts"), strconv.AppendInt(nil, int64(n), 10))
}

func atomicState(name string, data []byte) error {
	if err := securefile.Write(name, data, false); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(name))
}

func syncDirectory(name string) error {
	d, err := os.Open(name) // #nosec G304 -- call sites use a process-owned update state directory.
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func readFixed(root *os.Root, name string, limit int64) ([]byte, error) {
	f, err := openRegular(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(data)) > limit {
		return nil, errArchive
	}
	return data, err
}

func openRegular(root *os.Root, name string) (*os.File, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errArchive
	}
	return root.Open(name)
}
