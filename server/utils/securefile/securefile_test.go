//go:build unix

package securefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestC7PrivateFileAtomicAndRestrictive(t *testing.T) {
	f := filepath.Join(t.TempDir(), "key")
	if err := Write(f, []byte("fixture"), true); err != nil {
		t.Fatal(err)
	}
	if err := Write(f, []byte("replacement"), true); !os.IsExist(err) {
		t.Fatalf("exclusive create: %v", err)
	}
	if err := os.Chmod(f, 0644); err != nil {
		t.Fatal(err)
	}
	data, err := Read(f)
	if err != nil || string(data) != "fixture" {
		t.Fatalf("read: %v", err)
	}
	info, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	link := f + "-link"
	if err := os.Symlink(f, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(link); err == nil {
		t.Fatal("private file symlink accepted")
	}
}
