package sitebackup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/internal/backupcatalog"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/services/maintenance"
	"xorm.io/xorm"
)

const testPass = "this-is-only-a-backup-test"

func sqliteStore(t *testing.T) Store {
	t.Helper()
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	e.SetMaxIdleConns(1)
	t.Cleanup(func() { e.Close() })
	if err = db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	return Store{e, t.TempDir(), "1.0.0", ""}
}
func withFixture(t *testing.T, s Store) {
	t.Helper()
	t.Setenv("PMAIL_SECRET_KEY", "")
	at := time.Date(2024, 4, 2, 12, 0, 0, 0, time.FixedZone("+8", 8*3600))
	mail := models.Email{Id: 7, MsgID: "backup-fixture@example.test", Subject: "portable mail", Text: sql.NullString{String: "hello", Valid: true}, CreateTime: at, SendDate: at, ProjectionReady: true, POPOctets: 100}
	parts, _ := json.Marshal([]parsemail.Attachment{{Filename: "test.txt", ContentType: "text/plain", Content: []byte("attachment bytes")}})
	mail.Attachments = string(parts)
	for _, r := range []any{
		&models.User{ID: 5, Account: "member", Name: "Member", Password: "test-hash", TOTPEnabled: true},
		&models.TOTPConfig{UserID: 5, Ciphertext: "only-fixture-ciphertext", LastStep: 21},
		&models.RecoveryCode{UserID: 5, Digest: strings.Repeat("a", 64)},
		&models.Domain{ID: 2, Name: "example.test", DkimPrivateKey: "test-key", IsDefault: true},
		&models.MailboxAddress{ID: 9, UserID: 5, DomainID: 2, LocalPart: "alias", Status: models.AddressActive, CanSend: true},
		&models.Group{ID: 8, Name: "folder", UserId: 5}, &mail,
		&models.UserEmail{ID: 13, UserID: 5, EmailID: 7, GroupId: 8, IsRead: 1},
		&models.UserEmailAddress{UserEmailID: 13, AddressID: 9},
		&models.Rule{Id: 3, UserId: 5, Name: "fixture", Value: `[]`},
		&models.TelegramConfig{ID: 1, Token: "fixture-sealed", Username: "testbot"},
		&models.ReportMailPreference{UserID: 5, Enabled: true, GroupID: 8, MarkRead: true},
		&models.DMARCReport{ID: 4, Domain: "example.test", OrgName: "Report Fixture", ReportID: "first", EmailID: 7},
		&models.DMARCRecord{ID: 5, ReportID: 4, SourceIP: "192.0.2.4", Count: 12},
		&models.TLSReport{ID: 6, Domain: "example.test", OrgName: "Report Fixture", ReportID: "tls-first", EmailID: 7},
		&models.OutboundPayload{ID: 17, Digest: strings.Repeat("e", 64), Data: []byte{0, 0x80, 0xff}},
		&models.Sessions{Token: "not-exported", Data: []byte("test")},
	} {
		if _, err := s.Engine.Insert(r); err != nil {
			t.Fatalf("fixture %T: %v", r, err)
		}
	}
	if err := db.Transaction(s.Engine, func(tx *xorm.Session) error { return mailstore.WriteParts(tx, &mail, false) }); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"config/dkim/dkim.priv", "config/ssl/server.pem", "config/.env", "config/config.json"} {
		path := filepath.Join(s.Root, p)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(p), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func archive(t *testing.T, s Store, tls bool) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := s.Export(t.Context(), &b, testPass, tls, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSchemaCatalogCoverage(t *testing.T) {
	s := sqliteStore(t)
	var actual []string
	if err := s.Engine.SQL(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Find(&actual); err != nil {
		t.Fatal(err)
	}
	want := append(slices.Clone(backupcatalog.Tables), backupcatalog.Excluded...)
	slices.Sort(want)
	slices.Sort(actual)
	if !slices.Equal(want, actual) {
		t.Fatalf("every SyncSchema table must be included or deliberately excluded\nwant %v\ngot %v", want, actual)
	}
}

func TestSQLiteRoundTripAndProjection(t *testing.T) {
	s, d := sqliteStore(t), sqliteStore(t)
	s.SecretKey = "fixture-source-site-key"
	d.SecretKey = "fixture-previous-key"
	withFixture(t, s)
	if err := os.MkdirAll(filepath.Join(d.Root, "config", "ssl"), 0700); err != nil {
		t.Fatal(err)
	}
	oldTLS := filepath.Join(d.Root, "config", "ssl", "server.pem")
	if err := os.WriteFile(oldTLS, []byte("prior cert"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d.Root, "config", "secret.key"), []byte(d.SecretKey), 0600); err != nil {
		t.Fatal(err)
	}
	dir, err := DataDir(d.Root, "exports")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "old-user-export"), []byte("prior installation"), 0600); err != nil {
		t.Fatal(err)
	}
	buf := archive(t, s, true)
	if bytes.Contains(buf, []byte("portable mail")) || !bytes.HasPrefix(buf, []byte(Magic)) {
		t.Fatal("archive is not encrypted or missing magic")
	}
	if err = d.Restore(t.Context(), bytes.NewReader(buf), testPass, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range backupcatalog.Tables {
		a, err := s.Engine.Table(name).Count()
		if err != nil {
			t.Fatal(err)
		}
		b, err := d.Engine.Table(name).Count()
		if err != nil || b != a {
			t.Fatalf("%s row counts %d/%d (%v)", name, a, b, err)
		}
	}
	var e models.Email
	var u models.User
	var totp models.TOTPConfig
	var p models.OutboundPayload
	for _, item := range []struct {
		id   int
		bean any
	}{{7, &e}, {5, &u}, {5, &totp}, {17, &p}} {
		if ok, err := d.Engine.ID(item.id).Get(item.bean); err != nil || !ok {
			t.Fatalf("missing %T: %v", item.bean, err)
		}
	}
	if e.Subject != "portable mail" || e.ProjectionReady || e.POPOctets != 0 || !u.TOTPEnabled || totp.LastStep != 21 || !bytes.Equal(p.Data, []byte{0, 0x80, 0xff}) || e.SendDate.UTC().Format(time.RFC3339) != "2024-04-02T04:00:00Z" {
		t.Fatal("critical fields changed")
	}
	for _, table := range []string{"sessions", "mail_attachment_info", "mail_attachment_chunk"} {
		if n, err := d.Engine.Table(table).Count(); err != nil || n != 0 {
			t.Fatalf("derived table %s was copied: %d %v", table, n, err)
		}
	}
	if e := mailstore.Backfill(t.Context(), d.Engine); e != nil {
		t.Fatal(e)
	}
	var chunk models.MailAttachmentChunk
	if ok, err := d.Engine.Where("email_id=?", 7).Get(&chunk); err != nil || !ok || string(chunk.Data) != "attachment bytes" {
		t.Fatalf("attachment rebuild failed: %v", err)
	}
	for path, want := range map[string]string{"config/dkim/dkim.priv": "config/dkim/dkim.priv", "config/ssl/server.pem": "config/ssl/server.pem", "config/secret.key": s.SecretKey + "\n"} {
		if b, err := os.ReadFile(filepath.Join(d.Root, path)); err != nil || string(b) != want {
			t.Fatalf("missing restored key material %q", path)
		}
	}
	for _, path := range []string{"config/.env", "config/config.json", "config/exports/old-user-export"} {
		if _, err := os.Stat(filepath.Join(d.Root, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected surviving file %s", path)
		}
	}
	// The restarted process validates the exports path and its root alias.
	if info, err := os.Stat(filepath.Join(d.Root, "config", "exports")); err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatalf("empty exports directory not left in place: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(d.Root, "config", "exports")); len(entries) != 0 {
		t.Fatalf("old exports survived: %v", entries)
	}
	if _, err := filepath.EvalSymlinks(filepath.Join(d.Root, "exports")); err != nil {
		t.Fatalf("exports alias is dangling: %v", err)
	}
	files, _ := filepath.Glob(oldTLS + ".bak-*")
	if len(files) != 1 {
		t.Fatal("original TLS not backed up")
	}
	if _, err := d.Engine.Insert(&models.Email{Subject: "new"}); err != nil {
		t.Fatal(err)
	}
	var max int
	if _, err := d.Engine.SQL("SELECT MAX(id) FROM email").Get(&max); err != nil || max != 8 {
		t.Fatalf("sequence: %d %v", max, err)
	}
}

func TestArchiveAuthentication(t *testing.T) {
	payload := bytes.Repeat([]byte("test data different frames "), 6000)
	var b bytes.Buffer
	w, err := encrypt(&b, testPass)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	verify := func(b []byte, key string) error {
		r, err := decrypt(bytes.NewReader(b), key)
		if err != nil {
			return err
		}
		p, err := io.ReadAll(r)
		if err == nil && !bytes.Equal(p, payload) {
			return ErrInvalid
		}
		return err
	}
	if e := verify(b.Bytes(), testPass); e != nil {
		t.Fatal(e)
	}
	if e := verify(b.Bytes(), "another-test-password"); !errors.Is(e, ErrBadPassphrase) {
		t.Fatalf("wrong passphrase: %v", e)
	}
	for _, i := range []int{0, headerSize - 3, headerSize + 1, headerSize + 10, headerSize + chunkSize + 23, b.Len() - 1} {
		x := bytes.Clone(b.Bytes())
		x[i] ^= 1
		if e := verify(x, testPass); e == nil {
			t.Fatalf("accepted tampering at %d", i)
		}
	}
	if e := verify(b.Bytes()[:b.Len()-1], testPass); e == nil {
		t.Fatal("truncation accepted")
	}
	if e := verify(append(bytes.Clone(b.Bytes()), 0), testPass); e == nil {
		t.Fatal("trailing data accepted")
	}
	x := bytes.Clone(b.Bytes())
	frameLen := chunkSize + 5 + 16
	copy(x[headerSize:], b.Bytes()[headerSize+frameLen:headerSize+2*frameLen])
	if e := verify(x, testPass); e == nil {
		t.Fatal("misordered frame accepted")
	}
}

func TestNewerAndRollbackDuringWrite(t *testing.T) {
	s, d := sqliteStore(t), sqliteStore(t)
	withFixture(t, s)
	s.Version = "1.1.0"
	err := d.Restore(t.Context(), bytes.NewReader(archive(t, s, false)), testPass, func() (func(bool), error) { t.Fatal("entered maintenance for newer backup"); return nil, nil }, nil)
	if !errors.Is(err, ErrNewer) {
		t.Fatalf("newer version: %v", err)
	}
	s.Version = "1.0.0"
	if _, err = d.Engine.Insert(&models.User{ID: 1, Account: "keep", Name: "Keep", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Engine.Exec(`CREATE TRIGGER injection BEFORE INSERT ON email BEGIN SELECT RAISE(ABORT, 'injected restore failure'); END`); err != nil {
		t.Fatal(err)
	}
	var g maintenance.Gate
	called := false
	err = d.Restore(t.Context(), bytes.NewReader(archive(t, s, false)), testPass, func() (func(bool), error) {
		resume, err := g.Begin(context.Background())
		if err != nil {
			return nil, err
		}
		return func(committed bool) {
			called = true
			if committed {
				t.Error("committed injected failure")
			}
			resume()
		}, nil
	}, nil)
	if err == nil || !called || g.Active() {
		t.Fatalf("failed restore did not leave maintenance: %v", err)
	}
	var kept models.User
	if ok, err := d.Engine.ID(1).Get(&kept); err != nil || !ok || kept.Account != "keep" {
		t.Fatalf("lost target data: %v", err)
	}
	if _, err = os.Stat(filepath.Join(d.Root, "config", "dkim", "dkim.priv")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("restored key written even though database failed")
	}
}

func TestPersistentDataDirectoryAndInstanceLock(t *testing.T) {
	s := sqliteStore(t)
	for _, name := range []string{"backups", "exports"} {
		p, err := DataDir(s.Root, name)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(p) != filepath.Join(s.Root, "config") {
			t.Fatal("archive was not stored on the config volume")
		}
		throughRoot, err := filepath.EvalSymlinks(filepath.Join(s.Root, name))
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil || throughRoot != resolved {
			t.Fatal("root alias did not resolve to the volume")
		}
		i, e := os.Stat(p)
		if e != nil || i.Mode().Perm() != 0700 {
			t.Fatal("unsafe archive directory permissions")
		}
	}
	guard, err := Acquire(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if another, e := Acquire(s.Root); !errors.Is(e, ErrBusy) {
		if another != nil {
			another.Close()
		}
		t.Fatalf("offline operation could run while server was active: %v", e)
	}
	guard.Close()
	another, err := Acquire(s.Root)
	if err != nil {
		t.Fatalf("stale lockfile prevented restart: %v", err)
	}
	another.Close()
}

func TestBackupJobBusyAndWrongKeyProgress(t *testing.T) {
	s := sqliteStore(t)
	dir, err := DataDir(s.Root, "backups")
	if err != nil {
		t.Fatal(err)
	}
	name := Name(s.Version, false)
	if err = Publish(dir, name, func(f *os.File) error { return s.Export(t.Context(), f, testPass, false, nil) }); err != nil {
		t.Fatal(err)
	}
	var m Manager
	ready := make(chan struct{})
	j, err := m.StartRestore(s, name, "invalid-key-test-password", ready)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.StartExport(s, testPass, false); !errors.Is(err, ErrBusy) {
		t.Fatalf("overlapping job accepted: %v", err)
	}
	if m.RestoreStatus(j.ID+"a") != nil || m.RestoreStatus(j.ID) == nil {
		t.Fatal("progress capability was not confined to the originating job")
	}
	close(ready)
	deadline := time.After(5 * time.Second)
	for {
		s := m.RestoreStatus(j.ID)
		if s.State == "failed" {
			if s.Error != "backup_bad_passphrase" || maintenance.Default.Active() {
				t.Fatal("failed validation entered maintenance or was misclassified")
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("backup job did not finish")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestFileReplacementIsUndoneOnLateTransactionFailure(t *testing.T) {
	s, d := sqliteStore(t), sqliteStore(t)
	s.SecretKey, d.SecretKey = "fixture-source-key", "fixture-existing-key"
	withFixture(t, s)
	dest := filepath.Join(d.Root, "config", "dkim", "dkim.priv")
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("previous-key-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	oldExports, err := DataDir(d.Root, "exports")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(oldExports, "previous.zip"), []byte("previous mail export"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Engine.Insert(&models.Sessions{Token: "old-session", Data: []byte("fixture")}); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Engine.Exec(`CREATE TRIGGER injection_late BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'late failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = d.Restore(t.Context(), bytes.NewReader(archive(t, s, true)), testPass, nil, nil); err == nil {
		t.Fatal("restore should have failed after staging files")
	}
	if b, err := os.ReadFile(dest); err != nil || string(b) != "previous-key-bytes" {
		t.Fatalf("original key was not restored after rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldExports, "previous.zip")); err != nil {
		t.Fatalf("old export was not restored after rollback: %v", err)
	}
	if n, err := d.Engine.Count(&models.Sessions{}); err != nil || n != 1 {
		t.Fatalf("original session was lost: %d %v", n, err)
	}
	if n, err := d.Engine.Count(&models.Email{}); err != nil || n != 0 {
		t.Fatalf("DB writes survived rollback: %d %v", n, err)
	}
}
