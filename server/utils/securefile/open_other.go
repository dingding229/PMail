//go:build !unix && !windows

package securefile

import (
	"errors"
	"os"
)

// Refuse to claim that Unix owner/mode checks validated an ACL on this OS.
func openPrivate(path string) (*os.File, error) {
	if _, err := os.Lstat(path); err != nil {
		return nil, err
	}
	return nil, errors.New("private file ownership verification is unsupported on this platform")
}
func restrictPrivate(*os.File, string) error {
	return errors.New("cannot verify private file ownership")
}
func restrictNew(*os.File) error { return errors.New("cannot protect private file") }
