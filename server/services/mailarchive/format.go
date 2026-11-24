// Package mailarchive implements mailbox-only interchange. It never invokes
// SMTP delivery, the rules engine, Telegram, or other inbound side effects.
package mailarchive

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

const MessageLimit = 30 << 20 // same as the SMTP DATA limit
const maxZipEntries = 2000

var (
	ErrInvalid  = errors.New("mail_archive_invalid")
	ErrTarget   = errors.New("mail_archive_invalid_folder")
	ErrBusy     = errors.New("mail_archive_busy")
	ErrNotFound = errors.New("mail_archive_not_found")
	ErrTooLarge = errors.New("mail_archive_too_large")
	ErrQuota    = errors.New("quota_exceeded")
)

func rd(line string) bool { return strings.HasPrefix(strings.TrimLeft(line, ">"), "From ") }

func writeMbox(out io.Writer, wire string, read, starred bool, when time.Time) error {
	if !strings.Contains(wire, "\r\n\r\n") {
		return ErrInvalid
	}
	if when.IsZero() {
		when = time.Now()
	}
	if _, err := fmt.Fprintf(out, "From MAILER-DAEMON %s\n", when.UTC().Format(time.ANSIC)); err != nil {
		return err
	}
	inHeader := true
	skip := false
	for line := range strings.SplitSeq(strings.TrimSuffix(wire, "\r\n"), "\r\n") {
		if inHeader {
			if line == "" {
				flags := "O"
				if read {
					flags = "RO"
				}
				star := ""
				if starred {
					star = "F"
				}
				if _, err := fmt.Fprintf(out, "Status: %s\nX-Status: %s\n\n", flags, star); err != nil {
					return err
				}
				inHeader = false
				continue
			}
			if line[0] != ' ' && line[0] != '\t' {
				key, _, _ := strings.Cut(line, ":")
				skip = strings.EqualFold(key, "Status") || strings.EqualFold(key, "X-Status")
			}
			if skip {
				continue
			}
		} else if rd(line) {
			if _, err := io.WriteString(out, ">"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(out, line+"\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(out, "\n")
	return err
}

// An oversize mbox message is reported as a failed item (nil), then scanning
// resumes at the next separator. The line scanner is itself size bounded.
func readMbox(ctx context.Context, r io.Reader, fn func([]byte) error) error {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 32<<10), MessageLimit+2)
	var msg bytes.Buffer
	started, body, over := false, false, false
	flush := func() error {
		if !started {
			return nil
		}
		if over || msg.Len() == 0 {
			return fn(nil)
		}
		return fn(bytes.TrimSuffix(msg.Bytes(), []byte("\n")))
	}
	for s.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := s.Bytes()
		if bytes.HasPrefix(line, []byte("From ")) {
			if err := flush(); err != nil {
				return err
			}
			msg.Reset()
			started, body, over = true, false, false
			continue
		}
		if !started {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			return ErrInvalid
		}
		if over {
			continue
		}
		if body && len(line) > 0 && line[0] == '>' && bytes.HasPrefix(bytes.TrimLeft(line, ">"), []byte("From ")) {
			line = line[1:]
		}
		if len(line) == 0 {
			body = true
		}
		if msg.Len()+len(line)+1 > MessageLimit {
			over = true
			continue
		}
		msg.Write(line)
		msg.WriteByte('\n')
	}
	if s.Err() != nil {
		return ErrTooLarge
	}
	if !started {
		return nil
	}
	return flush()
}

func safeZipName(n string, dir bool) bool {
	if n == "" || len(n) > 512 || strings.ContainsAny(n, "\\\x00:") || strings.HasPrefix(n, "/") {
		return false
	}
	if dir {
		n = strings.TrimSuffix(n, "/")
	}
	if path.Clean(n) != n {
		return false
	}
	for _, c := range strings.Split(n, "/") {
		if c == "" || c == "." || c == ".." {
			return false
		}
	}
	return true
}

// Validate all entries and actual decompressed bytes before importing any mail.
func validZip(ctx context.Context, z []*zip.File, budget int64) error {
	if len(z) > maxZipEntries {
		return ErrTooLarge
	}
	var total int64
	seen := map[string]bool{}
	for _, f := range z {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := f.FileInfo().IsDir()
		if !safeZipName(f.Name, dir) || seen[f.Name] || !dir && !f.Mode().IsRegular() {
			return ErrInvalid
		}
		seen[f.Name] = true
		if f.UncompressedSize64 > uint64(budget-total) { // #nosec G115 -- total never exceeds budget, so budget-total >= 0.
			return ErrTooLarge
		}
		if dir {
			if f.UncompressedSize64 != 0 {
				return ErrInvalid
			}
			continue
		}
		switch strings.ToLower(path.Ext(f.Name)) {
		case ".eml", ".mbox":
		default:
			return ErrInvalid
		}
		r, err := f.Open()
		if err != nil {
			return ErrInvalid
		}
		n, err := io.Copy(io.Discard, io.LimitReader(contextReader{ctx, r}, budget-total+1))
		r.Close()
		if n > budget-total {
			return ErrTooLarge
		}
		if err != nil || uint64(n) != f.UncompressedSize64 { // #nosec G115 -- io.Copy returns a non-negative count.
			return ErrInvalid
		}
		total += n
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

func visitInput(ctx context.Context, files []*zip.File, fn func(ext string, r io.Reader) error) error {
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return ErrInvalid
		}
		err = fn(strings.ToLower(path.Ext(f.Name)), contextReader{ctx, r})
		err = errors.Join(err, r.Close())
		if err != nil {
			return err
		}
	}
	return nil
}

// Bound central-directory allocation before handing an uploaded file to
// archive/zip, which builds a slice for every entry it encounters.
func boundZipDirectory(r io.ReaderAt, size int64) error {
	const maxDirectory = 2 << 20
	const endSize = 22
	if size < endSize {
		return ErrInvalid
	}
	b := make([]byte, min(size, 65535+endSize))
	if _, err := r.ReadAt(b, size-int64(len(b))); err != nil {
		return ErrInvalid
	}
	for i := len(b) - endSize; i >= 0; i-- {
		e := b[i:]
		if binary.LittleEndian.Uint32(e) != 0x06054b50 || int(binary.LittleEndian.Uint16(e[20:])) != len(e)-endSize {
			continue
		}
		if binary.LittleEndian.Uint16(e[4:]) != 0 || binary.LittleEndian.Uint16(e[6:]) != 0 || binary.LittleEndian.Uint16(e[8:]) != binary.LittleEndian.Uint16(e[10:]) {
			return ErrInvalid
		}
		if binary.LittleEndian.Uint16(e[10:]) > maxZipEntries || binary.LittleEndian.Uint32(e[12:]) > maxDirectory {
			return ErrTooLarge
		}
		offset := int64(binary.LittleEndian.Uint32(e[16:]))
		directory := int64(binary.LittleEndian.Uint32(e[12:]))
		end := size - int64(len(b)) + int64(i)
		// Reject archive prefixes and ZIP64/multidisk directory metadata. Files
		// that need ZIP64 sizes may still have a normal central directory.
		if offset+directory != end {
			return ErrInvalid
		}
		return nil
	}
	return ErrInvalid
}
