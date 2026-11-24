package sitebackup

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Runtime data is stored inside the Compose config volume. The root-level
// names are convenience symlinks, recreated when the container is replaced.
func DataDir(root, name string) (string, error) {
	if name != "backups" && name != "exports" {
		return "", ErrInvalid
	}
	parent := filepath.Join(root, "config")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	if i, err := os.Lstat(parent); err != nil || !i.IsDir() {
		return "", ErrInvalid
	}
	d := filepath.Join(parent, name)
	if err := os.Mkdir(d, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if i, err := os.Lstat(d); err != nil || !i.IsDir() {
		return "", ErrInvalid
	}
	if err := os.Chmod(d, 0700); err != nil { // #nosec G302 -- directory: owner needs execute permission.
		return "", err
	}
	alias := filepath.Join(root, name)
	if _, err := os.Lstat(alias); errors.Is(err, os.ErrNotExist) {
		// The canonical volume path remains usable with a read-only root.
		_ = os.Symlink(filepath.Join("config", name), alias)
	}
	return d, nil
}

func noFollow() int { return unix.O_NOFOLLOW | unix.O_NONBLOCK }

// Acquire prevents offline restore/export while the same data volume is live,
// and prevents a second process from opening it during restore. File lifetime
// is independent of the advisory lock (never unlink the lock file).
func Acquire(root string) (*os.File, error) {
	d := filepath.Join(root, "config")
	if err := os.MkdirAll(d, 0700); err != nil {
		return nil, err
	}
	i, err := os.Lstat(d)
	if err != nil || !i.IsDir() {
		return nil, ErrInvalid
	}
	f, err := os.OpenFile(filepath.Join(d, ".pmail-instance.lock"), os.O_CREATE|os.O_RDWR|noFollow(), 0600) // #nosec G304 -- fixed file name in the config directory.
	if err != nil {
		return nil, err
	}
	if st, e := f.Stat(); e != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, ErrInvalid
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return f, nil
}

var nameFormat = regexp.MustCompile(`^(?:pre-restore-)?pmail-(v?[0-9]{1,9}\.[0-9]{1,9}\.[0-9]{1,9}|dev|unknown)-([0-9]{8}T[0-9]{6}(?:\.[0-9]{9})?Z)\.pmailbak$`)

const nameTime = "20060102T150405.000000000Z"

func Name(version string, pre bool) string {
	p := "pmail-"
	if pre {
		p = "pre-restore-" + p
	}
	if version != "dev" && versionRank(version) < 0 {
		version = "unknown"
	}
	return p + strings.TrimPrefix(version, "v") + "-" + time.Now().UTC().Format(nameTime) + ".pmailbak"
}

func parseName(name string) (string, time.Time, bool) {
	m := nameFormat.FindStringSubmatch(name)
	if len(m) == 0 {
		return "", time.Time{}, false
	}
	stamp, err := time.Parse(nameTime, m[2])
	if err != nil {
		stamp, err = time.Parse("20060102T150405Z", m[2])
	}
	v := strings.TrimPrefix(m[1], "v")
	if v == "unknown" {
		v = ""
	}
	return v, stamp, err == nil
}

func OpenNamed(dir, name string) (*os.File, error) {
	if _, _, ok := parseName(name); !ok {
		return nil, ErrNotFound
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDONLY|noFollow(), 0) // #nosec G304 -- name passed parseName (fixed backup name format), O_NOFOLLOW.
	if err != nil {
		return nil, ErrNotFound
	}
	i, err := f.Stat()
	if err != nil || !i.Mode().IsRegular() {
		f.Close()
		return nil, ErrNotFound
	}
	return f, nil
}

// Publish reserves the final name without overwriting any previous archive.
// The temporary file is visible only after a complete, fsynced write.
func Publish(dir, name string, write func(*os.File) error) error {
	f, err := os.CreateTemp(dir, ".pmailbak-part-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = write(f); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Link(f.Name(), filepath.Join(dir, name))
}
