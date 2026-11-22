// Package securefile handles private, process-owned key files.
package securefile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxSize = 4 << 20

func Read(path string) ([]byte, error) {
	f, err := openPrivate(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := restrictPrivate(f, path); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err == nil && len(data) > maxSize {
		return nil, fmt.Errorf("private file %q exceeds size limit", path)
	}
	return data, err
}

// Write installs a complete private file without ever exposing partial data.
// createOnly never replaces an existing entry and may return os.ErrExist.
func Write(path string, data []byte, createOnly bool) error {
	if len(data) > maxSize {
		return fmt.Errorf("private data exceeds size limit")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".pmail-private-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := restrictNew(f); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if createOnly {
		return os.Link(f.Name(), path)
	}
	return os.Rename(f.Name(), path)
}
