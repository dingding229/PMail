//go:build unix

package securefile

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func openPrivate(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open private file", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

func owned(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Geteuid()
}

func restrictPrivate(f *os.File, path string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !owned(info) {
		return fmt.Errorf("private file %q must be a regular file owned by the service", path)
	}
	if info.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0600 {
		if err := f.Chmod(0600); err != nil {
			return fmt.Errorf("secure permissions of %q: %w", path, err)
		}
		log.Warnf("tightened permissions on private file %q to 0600", path)
	}
	return nil
}
func restrictNew(f *os.File) error { return f.Chmod(0600) }
