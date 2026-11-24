package sitebackup

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/internal/backupcatalog"
)

// Inspect consumes and authenticates the complete stream before any mutation.
func (s Store) Inspect(ctx context.Context, in io.Reader, pass string) error {
	v, err := walk(ctx, in, pass, s.Version, nil)
	if err != nil {
		return err
	}
	return checkSiteKey(v.SourceKey)
}

func checkSiteKey(key string) error {
	if key == "" {
		return nil
	}
	if env := os.Getenv("PMAIL_SECRET_KEY"); env != "" {
		if !sameSecret(env, key) {
			return ErrSecret
		}
		return nil
	}
	if strings.TrimSpace(key) != key {
		return ErrSecret
	}
	return nil
}

// Restore verifies before taking the database transaction. beforeWrite is
// invoked only after verification (online callers drain traffic and make the
// safety backup here); it can return a release function for every exit path.
func (s Store) Restore(ctx context.Context, in io.ReadSeeker, pass string, beforeWrite func() (func(bool), error), progress Progress) error {
	return s.restore(ctx, in, pass, beforeWrite, progress, nil)
}

func (s Store) restore(ctx context.Context, in io.ReadSeeker, pass string, beforeWrite func() (func(bool), error), progress Progress, phase func(string)) error {
	v, err := walk(ctx, in, pass, s.Version, nil)
	if err != nil {
		return err
	}
	if err = checkSiteKey(v.SourceKey); err != nil {
		return err
	}
	committed := false
	if beforeWrite != nil {
		release, e := beforeWrite()
		if release != nil {
			defer func() { release(committed) }()
		}
		if e != nil {
			return e
		}
	}
	if _, err = in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	staged, err := s.stageFiles(v)
	if err != nil {
		return err
	}
	defer staged.discard()
	tx, err := s.Engine.DB().DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	dialect := s.Engine.DriverName()
	names := append(slices.Clone(backupcatalog.Tables), backupcatalog.Excluded...)
	if dialect == "postgres" {
		ordered := make([]string, len(names))
		for i, n := range names {
			ordered[i] = s.table(n)
		}
		if _, err := tx.ExecContext(ctx, "LOCK TABLE "+strings.Join(ordered, ", ")+" IN ACCESS EXCLUSIVE MODE"); err != nil {
			return err
		}
	}
	if dialect == "mysql" {
		// A nontransactional legacy table would violate the rollback guarantee.
		for _, n := range names {
			var engine string
			if err := tx.QueryRowContext(ctx, "SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", n).Scan(&engine); err != nil {
				return err
			}
			if !strings.EqualFold(engine, "InnoDB") {
				return ErrInvalid
			}
		}
	}
	for _, n := range slices.Backward(names) {
		if n == "sessions" {
			continue
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+s.table(n)); err != nil { // #nosec G202 -- table name comes from backupcatalog and is quoted by s.table.
			return fmt.Errorf("clear %s: %w", n, err)
		}
	}
	r := tableRestore{s: s, tx: tx, ctx: ctx, progress: progress}
	defer r.close()
	again, err := walk(ctx, in, pass, s.Version, r.insert)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(v.Manifest, again.Manifest) {
		return ErrInvalid
	}
	r.finish()
	if _, err := tx.ExecContext(ctx, "UPDATE "+s.table("email")+" SET projection_ready=false, pop_octets=0"); err != nil { // #nosec G202 -- fixed table, quoted identifier.
		return err
	}
	for _, n := range names {
		if !backupcatalog.HasIDSequence(n) || n == "sessions" || n == "mail_attachment_chunk" || n == "registration_guard" {
			continue
		}
		if err := s.sequence(ctx, tx, n); err != nil {
			return fmt.Errorf("sequence %s: %w", n, err)
		}
	}
	// Old ZIPs were issued to the previous installation's user IDs. Do
	// not let an unrelated restored owner inherit their exports.
	oldExports, err := s.quarantineExports()
	if err != nil {
		return err
	}
	recoverExports := func() error {
		if oldExports == "" {
			return nil
		}
		dir := filepath.Join(s.Root, "config", "exports")
		// Only the empty placeholder created by quarantineExports is removed.
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return os.Rename(oldExports, dir)
	}
	if phase != nil {
		phase("restoring_files")
	}
	if err := staged.apply(); err != nil {
		return errors.Join(err, staged.rollback(), recoverExports())
	}
	if phase != nil {
		phase("clearing_sessions")
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+s.table("sessions")); err != nil { // #nosec G202 -- fixed table, quoted identifier.
		return errors.Join(err, staged.rollback(), recoverExports())
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(err, staged.rollback(), recoverExports())
	}
	if oldExports != "" {
		_ = os.RemoveAll(oldExports)
	}
	committed = true
	return nil
}

type tableRestore struct {
	s        Store
	tx       *sql.Tx
	ctx      context.Context
	name     string
	cols     []string
	kinds    map[string]string
	stmt     *sql.Stmt
	progress Progress
	tables   int
	rows     int64
}

func (r *tableRestore) close() {
	if r.stmt != nil {
		_ = r.stmt.Close()
		r.stmt = nil
	}
}
func (r *tableRestore) finish() {
	if r.name != "" {
		if r.progress != nil {
			r.progress(r.tables, r.rows)
		}
		r.close()
	}
}
func (r *tableRestore) start(table string, row map[string]any) error {
	r.finish()
	r.name = table
	r.tables = slices.Index(backupcatalog.Tables, table) + 1
	r.cols = nil
	r.kinds = map[string]string{}
	res, err := r.tx.QueryContext(r.ctx, "SELECT * FROM "+r.s.table(table)+" WHERE 1=0") // #nosec G202 -- catalog table, quoted identifier.
	if err != nil {
		return err
	}
	columns, err := res.ColumnTypes()
	res.Close()
	if err != nil {
		return err
	}
	for _, c := range columns {
		k := kind(c.DatabaseTypeName())
		if backupcatalog.Binary(table, c.Name()) {
			k = "binary"
		}
		r.kinds[c.Name()] = k
	}
	for c := range row {
		if _, ok := r.kinds[c]; !ok {
			return ErrInvalid
		}
		r.cols = append(r.cols, c)
	}
	sort.Strings(r.cols)
	for _, key := range keyColumns(table) {
		if !slices.Contains(r.cols, key) || row[key] == nil {
			return ErrInvalid
		}
	}
	cols := make([]string, len(r.cols))
	args := make([]string, len(cols))
	for i, c := range r.cols {
		cols[i] = r.s.quote(c)
		args[i] = bind(r.s.Engine.DriverName(), i+1)
	}
	r.stmt, err = r.tx.PrepareContext(r.ctx, "INSERT INTO "+r.s.table(table)+" ("+strings.Join(cols, ",")+") VALUES ("+strings.Join(args, ",")+")") // #nosec G202 -- quoted catalog identifiers and generated placeholders; values are bound.
	return err
}
func (r *tableRestore) insert(table string, row map[string]any) error {
	if r.name != table {
		if err := r.start(table, row); err != nil {
			return err
		}
	}
	if len(row) != len(r.cols) {
		return ErrInvalid
	}
	params := make([]any, len(r.cols))
	for i, c := range r.cols {
		v, ok := row[c]
		if !ok {
			return ErrInvalid
		}
		x, err := driverValue(v, r.kinds[c])
		if err != nil {
			return err
		}
		params[i] = x
	}
	if _, err := r.stmt.ExecContext(r.ctx, params...); err != nil {
		return fmt.Errorf("insert into %s: %w", table, err)
	}
	r.rows++
	if r.rows%256 == 0 && r.progress != nil {
		r.progress(r.tables-1, r.rows)
	}
	return nil
}

func driverValue(v any, k string) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch x := v.(type) {
	case string:
		switch k {
		case "binary":
			return base64.StdEncoding.Strict().DecodeString(x)
		case "time":
			return parseDate(x)
		case "text":
			return x, nil
		}
	case json.Number:
		if k == "bool" {
			switch x.String() {
			case "0":
				return false, nil
			case "1":
				return true, nil
			default:
				return nil, ErrInvalid
			}
		}
		if i, err := x.Int64(); err == nil {
			return i, nil
		}
		if k == "number" {
			return x.Float64()
		}
	case bool:
		if k == "bool" {
			return x, nil
		}
		if k == "integer" {
			if x {
				return int64(1), nil
			}
			return int64(0), nil
		}
	}
	return nil, ErrInvalid
}

func (s Store) sequence(ctx context.Context, tx *sql.Tx, n string) error {
	var last int64
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(id),0) FROM "+s.table(n)).Scan(&last); err != nil {
		return err
	}
	if last < 0 || last == int64(^uint64(0)>>1) {
		return ErrInvalid
	}
	switch s.Engine.DriverName() {
	case "sqlite":
		if _, err := tx.ExecContext(ctx, "DELETE FROM sqlite_sequence WHERE name=?", n); err != nil {
			return err
		}
		if last != 0 {
			_, err := tx.ExecContext(ctx, "INSERT INTO sqlite_sequence(name,seq) VALUES (?,?)", n, last)
			return err
		}
	case "postgres":
		return backupcatalog.ResetPostgresSequence(ctx, tx, s.table(n), last)
	case "mysql":
		// InnoDB advances AUTO_INCREMENT for explicit inserted IDs and may keep
		// a larger old watermark. ALTER TABLE would implicitly commit the entire
		// restore; never trade rollback safety for a lower next auto ID.
	}
	return nil
}

type stagedFile struct {
	temp, dest, old string
	hadOld, put     bool
}
type stagedFiles []stagedFile

func (s Store) stageFiles(v validated) (stagedFiles, error) {
	files := map[string][]byte{}
	for name, b := range v.files {
		files[strings.TrimPrefix(name, "files/")] = b
	}
	if v.SourceKey != "" && os.Getenv("PMAIL_SECRET_KEY") == "" && v.SourceKey != s.SecretKey {
		files["secret.key"] = []byte(v.SourceKey + "\n")
	}
	p := filepath.Join(s.Root, "config")
	if err := os.MkdirAll(p, 0700); err != nil {
		return nil, err
	}
	if i, err := os.Lstat(p); err != nil || !i.IsDir() {
		return nil, ErrInvalid
	}
	var result stagedFiles
	defer func() {
		if files != nil {
			result.discard()
		}
	}()
	stamp := time.Now().UTC().Format(nameTime)
	for n, b := range files {
		dest := filepath.Join(p, filepath.FromSlash(n))
		dir := filepath.Dir(dest)
		if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if i, err := os.Lstat(dir); err != nil || !i.IsDir() {
			return nil, ErrInvalid
		}
		if i, err := os.Lstat(dest); err == nil && !i.Mode().IsRegular() {
			return nil, ErrInvalid
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		f, err := os.CreateTemp(dir, ".pmail-restore-part-")
		if err != nil {
			return nil, err
		}
		result = append(result, stagedFile{temp: f.Name(), dest: dest, old: dest + ".bak-" + stamp})
		_, err = f.Write(b)
		if err == nil {
			err = f.Sync()
		}
		err = errors.Join(err, f.Close())
		if err != nil {
			return nil, err
		}
	}
	files = nil
	return result, nil
}
func (f stagedFiles) discard() {
	for _, s := range f {
		_ = os.Remove(s.temp)
	}
}
func (f stagedFiles) apply() error {
	for i := range f {
		s := &f[i]
		if _, err := os.Lstat(s.dest); err == nil {
			// A hard link never overwrites a previous backup name. Original and
			// replacement stay on the same filesystem.
			if err := os.Link(s.dest, s.old); err != nil {
				return err
			}
			s.hadOld = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(s.temp, s.dest); err != nil {
			return err
		}
		s.put = true
	}
	return nil
}
func (f stagedFiles) rollback() error {
	var all error
	for _, s := range slices.Backward(f) {
		if s.put && s.hadOld {
			all = errors.Join(all, os.Rename(s.old, s.dest))
		} else if s.put {
			all = errors.Join(all, os.Remove(s.dest))
		} else if s.hadOld {
			all = errors.Join(all, os.Remove(s.old))
		}
	}
	return all
}

func (s Store) quarantineExports() (string, error) {
	dir := filepath.Join(s.Root, "config", "exports")
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil || !info.IsDir() {
		return "", ErrInvalid
	}
	target := filepath.Join(s.Root, "config", ".pmail-retired-exports-"+time.Now().UTC().Format(nameTime))
	if err := os.Rename(dir, target); err != nil {
		return "", err
	}
	// Startup validates this path (and its root-level alias), so leave an
	// empty directory behind; otherwise the restarted process exits.
	if err := os.Mkdir(dir, 0700); err != nil {
		return "", errors.Join(err, os.Rename(target, dir))
	}
	return target, nil
}
