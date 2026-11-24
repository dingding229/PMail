package sitebackup

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/internal/backupcatalog"
	"github.com/Jinnrry/pmail/internal/releasebundle"
	"xorm.io/xorm"
)

const maxRow = 128 << 20
const maxManifest = 1 << 20
const maxKeyFile = 2 << 20
const maxKeyFiles = 128
const maxKeysTotal = 16 << 20
const maxArchive = int64(1) << 40

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type FileInfo struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type TableInfo struct {
	FileInfo
	Rows int64 `json:"rows"`
}
type Manifest struct {
	Format   int                  `json:"format_version"`
	Version  string               `json:"pmail_version"`
	Created  time.Time            `json:"created_at"`
	Database string               `json:"source_database"`
	Tables   map[string]TableInfo `json:"tables"`
	Files    map[string]FileInfo  `json:"files"`
	// Needed for sealed per-site DB credentials and existing SRS signatures.
	// The manifest is itself encrypted. Never expose this field through HTTP.
	SourceKey string `json:"source_seal_key,omitempty"`
}

type Store struct {
	Engine    *xorm.Engine
	Root      string
	Version   string
	SecretKey string
}

type Progress func(tables int, rows int64)

type countingHash struct {
	hash.Hash
	size int64
}

func (c *countingHash) Write(p []byte) (int, error) {
	n, e := c.Hash.Write(p)
	c.size += int64(n)
	return n, e
}
func (c *countingHash) info() FileInfo {
	return FileInfo{Size: c.size, SHA256: hex.EncodeToString(c.Sum(nil))}
}
func digest(b []byte) FileInfo {
	s := sha256.Sum256(b)
	return FileInfo{int64(len(b)), hex.EncodeToString(s[:])}
}

func (s Store) quote(n string) string { return s.Engine.Quote(n) }
func (s Store) table(n string) string {
	if s.Engine.DriverName() == "postgres" && s.Engine.Dialect().URI().Schema != "" {
		return s.quote(s.Engine.Dialect().URI().Schema) + "." + s.quote(n)
	}
	return s.quote(n)
}
func bind(dialect string, n int) string {
	if dialect == "postgres" {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}
func kind(typ string) string {
	typ = strings.ToUpper(typ)
	switch {
	case strings.Contains(typ, "TIMESTAMP") || strings.Contains(typ, "DATETIME") || typ == "DATE":
		return "time"
	case strings.Contains(typ, "BOOL"):
		return "bool"
	case strings.Contains(typ, "INT") || typ == "BIT":
		return "integer"
	case strings.Contains(typ, "DOUBLE") || strings.Contains(typ, "FLOAT") || strings.Contains(typ, "REAL") || strings.Contains(typ, "NUMERIC") || strings.Contains(typ, "DECIMAL"):
		return "number"
	case strings.Contains(typ, "BLOB") || typ == "BYTEA" || strings.Contains(typ, "BINARY"):
		return "binary"
	}
	return "text"
}
func parseDate(v string) (time.Time, error) {
	for _, f := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05.999999999", "2006-01-02"} {
		if t, err := time.Parse(f, v); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, ErrInvalid
}
func portable(v any, column string) (any, error) {
	if v == nil {
		return nil, nil
	}
	if t, ok := v.(time.Time); ok {
		return t.UTC().Format(time.RFC3339Nano), nil
	}
	if column == "binary" {
		if b, ok := v.([]byte); ok {
			return base64.StdEncoding.EncodeToString(b), nil
		}
		if s, ok := v.(string); ok {
			return base64.StdEncoding.EncodeToString([]byte(s)), nil
		}
		return nil, ErrInvalid
	}
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	if text, ok := v.(string); ok {
		switch column {
		case "time":
			if text == "" || strings.HasPrefix(text, "0000-00-00") {
				return nil, nil
			}
			t, err := parseDate(text)
			if err != nil {
				return nil, err
			}
			return t.Format(time.RFC3339Nano), nil
		case "integer":
			return strconv.ParseInt(text, 10, 64)
		case "number":
			return strconv.ParseFloat(text, 64)
		case "bool":
			switch text {
			case "1", "t", "true":
				return true, nil
			case "0", "f", "false":
				return false, nil
			default:
				return nil, ErrInvalid
			}
		}
	}
	return v, nil
}
func keyColumns(name string) []string {
	if c := backupcatalog.KeyColumn(name); c != "" {
		return []string{c}
	}
	switch name {
	case "recovery_code":
		return []string{"user_id", "digest"}
	case "user_email_address":
		return []string{"user_email_id", "address_id"}
	case "imap_mailbox_uid":
		return []string{"box_id", "link_id"}
	case "spam_training":
		return []string{"user_id", "email_id"}
	case "spam_token":
		return []string{"user_id", "token_key"}
	case "compose_image":
		return []string{"cid"}
	case "draft_inline_image":
		return []string{"draft_id", "cid"}
	case "email_translation":
		return []string{"email_id", "user_id", "target_lang", "engine"}
	case "email_ai_result":
		return []string{"user_id", "email_id", "action", "lang"}
	}
	return nil
}

// Read from one snapshot; only a row and its JSON representation are held at
// once. Use keyset batches on single numeric keys, an ordered cursor otherwise.
func (s Store) dumpTable(ctx context.Context, tx *sql.Tx, name string, out io.Writer) (TableInfo, error) {
	stats := &countingHash{Hash: sha256.New()}
	enc := json.NewEncoder(io.MultiWriter(out, stats))
	enc.SetEscapeHTML(false)
	keys := keyColumns(name)
	if len(keys) == 0 {
		return TableInfo{}, ErrInvalid
	}
	order := make([]string, len(keys))
	for i, k := range keys {
		order[i] = s.quote(k)
	}
	q := "SELECT * FROM " + s.table(name)
	var cursor int64
	first := true
	var n int64
	paged := backupcatalog.KeyColumn(name) != ""
	for {
		stmt := q
		var args []any
		if paged && !first {
			stmt += " WHERE " + order[0] + ">" + bind(s.Engine.DriverName(), 1)
			args = []any{cursor}
		}
		stmt += " ORDER BY " + strings.Join(order, ",")
		if paged {
			stmt += " LIMIT 256"
		}
		rows, err := tx.QueryContext(ctx, stmt, args...)
		if err != nil {
			return TableInfo{}, err
		}
		batch, err := func() (int, error) {
			defer rows.Close()
			cols, err := rows.ColumnTypes()
			if err != nil {
				return 0, err
			}
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range cols {
				ptrs[i] = &vals[i]
			}
			count := 0
			for rows.Next() {
				if err := rows.Scan(ptrs...); err != nil {
					return count, err
				}
				row := make(map[string]any, len(cols))
				for i, c := range cols {
					k := kind(c.DatabaseTypeName())
					if backupcatalog.Binary(name, c.Name()) {
						k = "binary"
					}
					val, e := portable(vals[i], k)
					if e != nil {
						return count, e
					}
					row[c.Name()] = val
					if paged && c.Name() == keys[0] {
						if cursor, e = strconv.ParseInt(fmt.Sprint(val), 10, 64); e != nil {
							return count, ErrInvalid
						}
					}
				}
				if err := enc.Encode(row); err != nil {
					return count, err
				}
				count++
			}
			return count, rows.Err()
		}()
		if err != nil {
			return TableInfo{}, err
		}
		n += int64(batch)
		if !paged || batch < 256 {
			return TableInfo{stats.info(), n}, nil
		}
		first = false
	}
}

func allowedFile(n string) bool {
	parts := strings.Split(n, "/")
	if len(parts) != 3 || parts[0] != "files" || (parts[1] != "dkim" && parts[1] != "ssl") {
		return false
	}
	leaf := parts[2]
	if len(leaf) < 1 || len(leaf) > 160 || strings.HasPrefix(leaf, ".") || strings.Contains(leaf, ".bak-") {
		return false
	}
	for _, b := range leaf {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '.') {
			return false
		}
	}
	switch strings.ToLower(filepath.Ext(leaf)) {
	case ".priv", ".pem", ".key", ".crt", ".cer", ".pub":
		return true
	}
	return false
}
func (s Store) keyFiles(includeTLS bool) (map[string][]byte, error) {
	out := map[string][]byte{}
	total := 0
	for _, dir := range []string{"dkim", "ssl"} {
		if dir == "ssl" && !includeTLS {
			continue
		}
		path := filepath.Join(s.Root, "config", dir)
		i, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil || !i.IsDir() {
			return nil, ErrInvalid
		}
		root, err := os.OpenRoot(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		err = func() error {
			defer root.Close()
			fd, err := root.Open(".")
			if err != nil {
				return err
			}
			entries, err := fd.ReadDir(-1)
			fd.Close()
			if err != nil {
				return err
			}
			for _, e := range entries {
				name := "files/" + dir + "/" + e.Name()
				if !allowedFile(name) || !e.Type().IsRegular() {
					continue
				}
				f, err := root.OpenFile(e.Name(), os.O_RDONLY|noFollow(), 0)
				if err != nil {
					return err
				}
				b, err := io.ReadAll(io.LimitReader(f, maxKeyFile+1))
				f.Close()
				if err != nil {
					return err
				}
				total += len(b)
				if len(b) > maxKeyFile || total > maxKeysTotal || len(out) >= maxKeyFiles {
					return ErrTooLarge
				}
				out[name] = b
			}
			return nil
		}()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func entry(t *tar.Writer, n string, size int64) error {
	return t.WriteHeader(&tar.Header{Name: n, Size: size, Typeflag: tar.TypeReg, Mode: 0600})
}

func (s Store) Export(ctx context.Context, out io.Writer, pass string, tls bool, progress Progress) error {
	if err := ValidatePassphrase(pass); err != nil {
		return err
	}
	files, err := s.keyFiles(tls)
	if err != nil {
		return err
	}
	m := Manifest{Format: 1, Version: s.Version, Created: time.Now().UTC(), Database: s.Engine.DriverName(), Tables: map[string]TableInfo{}, Files: map[string]FileInfo{}, SourceKey: s.SecretKey}
	if len(m.SourceKey) > 4096 || strings.ContainsAny(m.SourceKey, "\r\n\x00") {
		return ErrSecret
	}
	enc, err := encrypt(out, pass)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(enc)
	tw := tar.NewWriter(gz)
	opts := &sql.TxOptions{ReadOnly: true}
	if m.Database != "sqlite" {
		opts.Isolation = sql.LevelRepeatableRead
	}
	tx, err := s.Engine.DB().DB.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var all int64
	for i, n := range backupcatalog.Tables {
		// A tar entry needs its byte length up front. Walk the same snapshot
		// twice rather than creating a plaintext scratch file on disk.
		info, err := s.dumpTable(ctx, tx, n, io.Discard)
		if err != nil {
			return fmt.Errorf("read table %s: %w", n, err)
		}
		if err := entry(tw, "tables/"+n+".jsonl", info.Size); err != nil {
			return err
		}
		again, err := s.dumpTable(ctx, tx, n, tw)
		if err != nil {
			return fmt.Errorf("stream table %s: %w", n, err)
		}
		if again != info {
			return ErrInvalid
		}
		m.Tables[n] = info
		all += info.Rows
		if progress != nil {
			progress(i+1, all)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// A certificate should not be exported halfway through a renewal that
	// replaced its key or leaf while the database snapshot was streaming.
	verified, err := s.keyFiles(tls)
	if err != nil {
		return err
	}
	if len(files) != len(verified) {
		return ErrInvalid
	}
	for name, b := range files {
		if b2, ok := verified[name]; !ok || !bytes.Equal(b, b2) {
			return ErrInvalid
		}
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		b := files[n]
		m.Files[n] = digest(b)
		if err := entry(tw, n, int64(len(b))); err != nil {
			return err
		}
		if _, err := tw.Write(b); err != nil {
			return err
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err = entry(tw, "manifest.json", int64(len(b))); err != nil {
		return err
	}
	if _, err = tw.Write(b); err != nil {
		return err
	}
	if err = tw.Close(); err != nil {
		return err
	}
	if err = gz.Close(); err != nil {
		return err
	}
	return enc.Close()
}

type validated struct {
	Manifest
	files map[string][]byte
}
type rowWriter func(table string, row map[string]any) error

func versionRank(v string) int64 {
	if v != "" && !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return releasebundle.Rank(v)
}
func checkVersion(current, saved string) error {
	if saved == "dev" && current == "dev" {
		return nil
	}
	r := versionRank(saved)
	c := versionRank(current)
	if r < 0 || c < 0 {
		return ErrInvalid
	}
	if r > c {
		return ErrNewer
	}
	return nil
}

func walk(ctx context.Context, in io.Reader, pass, version string, write rowWriter) (validated, error) {
	var v validated
	v.files = map[string][]byte{}
	tables := map[string]TableInfo{}
	files := map[string]FileInfo{}
	enc, err := decrypt(in, pass)
	if err != nil {
		return v, err
	}
	buf := bufio.NewReader(enc)
	gz, err := gzip.NewReader(buf)
	if err != nil {
		return v, archiveError(err)
	}
	defer gz.Close()
	gz.Multistream(false)
	tr := tar.NewReader(gz)
	seenManifest := false
	var total int64
	var fileTotal int
	for {
		if err := ctx.Err(); err != nil {
			return v, err
		}
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return v, archiveError(err)
		}
		if seenManifest || h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > maxArchive-total {
			return v, ErrInvalid
		}
		total += h.Size
		switch {
		case strings.HasPrefix(h.Name, "tables/") && strings.HasSuffix(h.Name, ".jsonl"):
			name := strings.TrimSuffix(strings.TrimPrefix(h.Name, "tables/"), ".jsonl")
			if len(tables) >= len(backupcatalog.Tables) || name != backupcatalog.Tables[len(tables)] {
				return v, ErrInvalid
			}
			d := &countingHash{Hash: sha256.New()}
			scanner := bufio.NewScanner(io.TeeReader(tr, d))
			scanner.Buffer(make([]byte, 32<<10), maxRow)
			var rows int64
			for scanner.Scan() {
				if err := ctx.Err(); err != nil {
					return v, err
				}
				row, err := decodeRow(scanner.Bytes())
				if err != nil {
					return v, err
				}
				if write != nil {
					if err := write(name, row); err != nil {
						return v, err
					}
				}
				rows++
			}
			if err = scanner.Err(); err != nil {
				return v, archiveError(err)
			}
			if d.size != h.Size {
				return v, ErrInvalid
			}
			tables[name] = TableInfo{d.info(), rows}
		case allowedFile(h.Name):
			if _, ok := files[h.Name]; ok || len(files) >= maxKeyFiles || h.Size > maxKeyFile || h.Size > int64(maxKeysTotal-fileTotal) {
				return v, ErrInvalid
			}
			b, err := io.ReadAll(tr)
			if err != nil {
				return v, archiveError(err)
			}
			fileTotal += len(b)
			files[h.Name] = digest(b)
			v.files[h.Name] = b
		case h.Name == "manifest.json":
			if h.Size > maxManifest {
				return v, ErrInvalid
			}
			b, err := io.ReadAll(tr)
			if err != nil {
				return v, archiveError(err)
			}
			d := json.NewDecoder(bytes.NewReader(b))
			d.DisallowUnknownFields()
			if d.Decode(&v.Manifest) != nil || d.Decode(new(any)) != io.EOF {
				return v, ErrInvalid
			}
			seenManifest = true
		default:
			return v, ErrInvalid
		}
	}
	// Force gzip checksum, the authenticated end record and underlying EOF.
	var extra [1]byte
	if n, err := io.ReadFull(gz, extra[:]); n != 0 || err != io.EOF {
		return v, archiveError(err)
	}
	if _, err := buf.Peek(1); err != io.EOF {
		return v, archiveError(err)
	}
	if !seenManifest || v.Format != 1 || v.Created.IsZero() || !slices.Contains([]string{"sqlite", "postgres", "mysql"}, v.Database) || len(tables) != len(backupcatalog.Tables) || len(v.Tables) != len(tables) || len(v.Files) != len(files) || len(v.SourceKey) > 4096 || strings.ContainsAny(v.SourceKey, "\r\n\x00") {
		return v, ErrInvalid
	}
	for k, x := range tables {
		if v.Tables[k] != x {
			return v, ErrInvalid
		}
	}
	for k, x := range files {
		if v.Files[k] != x {
			return v, ErrInvalid
		}
	}
	return v, checkVersion(version, v.Version)
}

func decodeRow(b []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	row := map[string]any{}
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return nil, ErrInvalid
	}
	for d.More() {
		t, err := d.Token()
		k, ok := t.(string)
		if err != nil || !ok || !identifier.MatchString(k) || len(row) >= 256 {
			return nil, ErrInvalid
		}
		if _, dup := row[k]; dup {
			return nil, ErrInvalid
		}
		var v any
		if err := d.Decode(&v); err != nil {
			return nil, ErrInvalid
		}
		switch v.(type) {
		case nil, string, json.Number, bool:
		default:
			return nil, ErrInvalid
		}
		row[k] = v
	}
	if _, err := d.Token(); err != nil || d.Decode(new(any)) != io.EOF || len(row) == 0 {
		return nil, ErrInvalid
	}
	return row, nil
}
func archiveError(err error) error {
	if errors.Is(err, ErrBadPassphrase) {
		return ErrBadPassphrase
	}
	return ErrInvalid
}
