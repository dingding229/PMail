package controllers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/sitebackup"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestAdminBackupPermissionsAndTraversal(t *testing.T) {
	root := t.TempDir()
	old := config.ROOT_PATH
	config.ROOT_PATH = root
	t.Cleanup(func() { config.ROOT_PATH = old })
	for _, handler := range []HandlerFunc{BackupIndex("1.0.0"), BackupRestore("1.0.0"), BackupDownload, BackupDelete, BackupUpload, BackupRestoreStatus} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/backups", nil)
		handler(&context.Context{UserID: 1}, w, r)
		if w.Code != 403 {
			t.Fatalf("ordinary user has admin backup access: %d", w.Code)
		}
	}
	dir, err := sitebackup.DataDir(root, "backups")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "outside-secret")
	if err = os.WriteFile(file, []byte("must not be served"), 0600); err != nil {
		t.Fatal(err)
	}
	name := sitebackup.Name("1.0.0", false)
	if err = os.Symlink(file, filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside-secret", "..%2Foutside-secret", name, "sample.pmailbak"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/backups/download", nil)
		r.SetPathValue("name", path)
		BackupDownload(&context.Context{UserID: 1, IsAdmin: true}, w, r)
		if w.Code != 404 || strings.Contains(w.Body.String(), "must not be served") {
			t.Fatalf("path %q wasn't rejected: %d", path, w.Code)
		}
	}
	name = sitebackup.Name("1.0.0", false)
	if err = os.WriteFile(filepath.Join(dir, name), []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/admin/backups/download", nil)
	r.SetPathValue("name", name)
	r.Header.Set("Range", "bytes=3-5")
	BackupDownload(&context.Context{UserID: 1, IsAdmin: true}, w, r)
	if w.Code != 206 || w.Body.String() != "345" {
		t.Fatalf("Range unsupported: %d", w.Code)
	}
}
