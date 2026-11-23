package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

const maxExtracted = 256 << 20
const maxEntries = 8192
const archiveName = "bundle.tar.gz"

var errArchive = errors.New("invalid update archive")

// visitBundle accepts only one gzip stream, regular files and directories
// below web/, and a regular pmail at the root. No archive-supplied permissions.
func visitBundle(ctx context.Context, src io.Reader, maxBytes int64, visit func(string, int64, io.Reader) error) error {
	buffer := bufio.NewReader(src)
	gz, err := gzip.NewReader(buffer)
	if err != nil {
		return err
	}
	defer gz.Close()
	gz.Multistream(false)
	// Bound headers, PAX records and padding as well as the file payloads.
	budget := &io.LimitedReader{R: gz, N: maxBytes + (16 << 20)}
	tr := tar.NewReader(budget)
	seen := make(map[string]bool)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(h.Name, "/")
		isDir := h.Typeflag == tar.TypeDir
		if len(name) > 512 || len(seen) >= maxEntries || !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:\x00") ||
			(name != "pmail" && name != "web" && !strings.HasPrefix(name, "web/")) ||
			isDir && (name == "pmail" || h.Size != 0) || !isDir && (name == "web" || h.Typeflag != tar.TypeReg || h.Name != name) {
			return errArchive
		}
		if _, exists := seen[name]; exists || h.Size < 0 || h.Size > maxBytes-total || (h.Size == 0 && (name == "pmail" || name == "web/index.html")) {
			return errArchive
		}
		for key := range h.PAXRecords {
			if strings.HasPrefix(key, "GNU.sparse.") {
				return errArchive
			}
		}
		seen[name] = !isDir
		if isDir {
			continue
		} // parents are created when their first file is extracted
		total += h.Size
		if err := visit(name, h.Size, tr); err != nil {
			return err
		}
	}
	if !seen["pmail"] || !seen["web/index.html"] {
		return errArchive
	}
	// Go's tar reader has consumed its two zero blocks. Permit only the final
	// zero padding left by tar, and finish reading gzip to validate its CRC.
	var trailing [8192]byte
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := budget.Read(trailing[:])
		for _, b := range trailing[:n] {
			if b != 0 {
				return errArchive
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if budget.N == 0 {
		return errArchive
	}
	if _, err := buffer.Peek(1); err != io.EOF {
		return errArchive
	}
	return nil
}

func unpackBundle(ctx context.Context, root *os.Root, src io.Reader, maxBytes int64) error {
	return visitBundle(ctx, src, maxBytes, func(name string, size int64, data io.Reader) error {
		if err := root.MkdirAll(path.Dir(name), 0700); err != nil {
			return err
		}
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		n, err := io.Copy(f, data)
		if err == nil && n != size {
			err = io.ErrUnexpectedEOF
		}
		if err == nil && name == "pmail" {
			err = f.Chmod(0755)
		} // #nosec G302 -- verified update executable; the enclosing stage is 0700.
		if err == nil {
			err = f.Sync()
		}
		return errors.Join(err, f.Close())
	})
}

// The signed archive is retained so startup can verify the extracted binary
// and web files again. The serving tree must contain no extra files or links.
func verifyInstalled(ctx context.Context, root *os.Root, archive io.Reader) error {
	files := make(map[string]bool)
	err := visitBundle(ctx, archive, maxExtracted, func(name string, size int64, data io.Reader) error {
		expected := sha256.New()
		if n, err := io.Copy(expected, data); err != nil || n != size {
			return errArchive
		}
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() != size || name == "pmail" && info.Mode().Perm() != 0755 {
			return errArchive
		}
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		actual := sha256.New()
		n, err := io.Copy(actual, io.LimitReader(f, size+1))
		f.Close()
		if err != nil || n != size || string(actual.Sum(nil)) != string(expected.Sum(nil)) {
			return errArchive
		}
		files[name] = true
		return nil
	})
	if err != nil {
		return err
	}
	return fs.WalkDir(root.FS(), "web", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (d.Type() != 0 || !files[name]) {
			return errArchive
		}
		return nil
	})
}
