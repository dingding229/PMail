package sitebackup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"io"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const Magic = "PMAILBAK1"
const chunkSize = 64 << 10
const kdfMemory = 64 << 10 // KiB
const kdfPasses = 3
const kdfLanes = 2
const headerSize = len(Magic) + 4 + 4 + 1 + 16 + 4

var (
	ErrPassphrase    = errors.New("backup_passphrase_invalid")
	ErrBadPassphrase = errors.New("backup_bad_passphrase")
	ErrInvalid       = errors.New("backup_corrupt")
	ErrNewer         = errors.New("backup_version_newer")
	ErrBusy          = errors.New("backup_busy")
	ErrNotFound      = errors.New("backup_not_found")
	ErrTooLarge      = errors.New("backup_too_large")
	ErrSecret        = errors.New("backup_secret_mismatch")
	ErrConfirm       = errors.New("backup_confirmation_required")
)

func ValidatePassphrase(s string) error {
	if len(s) > 1024 || !utf8.ValidString(s) || utf8.RuneCountInString(s) < 12 {
		return ErrPassphrase
	}
	return nil
}

func validHeader(h []byte) bool {
	p := len(Magic)
	return len(h) == headerSize && string(h[:p]) == Magic && binary.BigEndian.Uint32(h[p:]) == kdfMemory && binary.BigEndian.Uint32(h[p+4:]) == kdfPasses && h[p+8] == kdfLanes
}

func keyFor(h []byte, pass string) (cipher.AEAD, error) {
	key := argon2.IDKey([]byte(pass), h[len(Magic)+9:headerSize-4], kdfPasses, kdfMemory, kdfLanes, 32)
	defer clear(key)
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

type framing struct {
	aead   cipher.AEAD
	header []byte
	seq    uint64
}

func (f *framing) frame(n int, final bool) (nonce [12]byte, size [5]byte, aad []byte) {
	copy(nonce[:4], f.header[headerSize-4:])
	binary.BigEndian.PutUint64(nonce[4:], f.seq)
	binary.BigEndian.PutUint32(size[:4], uint32(n)) // #nosec G115 -- n <= chunkSize (64 KiB).
	if final {
		size[4] = 1
	}
	aad = make([]byte, 0, headerSize+13)
	aad = append(aad, f.header...)
	aad = append(aad, nonce[4:]...)
	aad = append(aad, size[:]...)
	return
}

type encryptWriter struct {
	framing
	out     io.Writer
	pending []byte
	closed  bool
}

func encrypt(out io.Writer, pass string) (*encryptWriter, error) {
	if err := ValidatePassphrase(pass); err != nil {
		return nil, err
	}
	h := make([]byte, headerSize)
	p := copy(h, Magic)
	binary.BigEndian.PutUint32(h[p:], kdfMemory)
	binary.BigEndian.PutUint32(h[p+4:], kdfPasses)
	h[p+8] = kdfLanes
	if _, err := rand.Read(h[p+9:]); err != nil {
		return nil, err
	}
	aead, err := keyFor(h, pass)
	if err != nil {
		return nil, err
	}
	if n, err := out.Write(h); err != nil {
		return nil, err
	} else if n != len(h) {
		return nil, io.ErrShortWrite
	}
	return &encryptWriter{framing: framing{aead: aead, header: h}, out: out, pending: make([]byte, 0, chunkSize)}, nil
}

func (w *encryptWriter) flush(final bool) error {
	if w.seq == ^uint64(0) {
		return ErrTooLarge
	}
	nonce, size, aad := w.frame(len(w.pending), final)
	block := w.aead.Seal(size[:], nonce[:], w.pending, aad)
	if n, err := w.out.Write(block); err != nil {
		return err
	} else if n != len(block) {
		return io.ErrShortWrite
	}
	w.seq++
	w.pending = w.pending[:0]
	return nil
}

func (w *encryptWriter) Write(b []byte) (int, error) {
	if w.closed {
		return 0, io.ErrClosedPipe
	}
	i := 0
	for len(b) > 0 {
		n := min(len(b), chunkSize-len(w.pending))
		w.pending = append(w.pending, b[:n]...)
		b = b[n:]
		i += n
		if len(w.pending) == chunkSize {
			if err := w.flush(false); err != nil {
				return i, err
			}
		}
	}
	return i, nil
}

func (w *encryptWriter) Close() error {
	if w.closed {
		return io.ErrClosedPipe
	}
	w.closed = true
	if len(w.pending) != 0 {
		if err := w.flush(false); err != nil {
			return err
		}
	}
	return w.flush(true)
}

type decryptReader struct {
	framing
	in      io.Reader
	pending []byte
	last    bool
	short   bool
}

func decrypt(in io.Reader, pass string) (*decryptReader, error) {
	if err := ValidatePassphrase(pass); err != nil {
		return nil, err
	}
	h := make([]byte, headerSize)
	if _, err := io.ReadFull(in, h); err != nil || !validHeader(h) {
		return nil, ErrInvalid
	}
	aead, err := keyFor(h, pass)
	if err != nil {
		return nil, err
	}
	return &decryptReader{framing: framing{aead: aead, header: h}, in: in}, nil
}

func (r *decryptReader) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if len(r.pending) != 0 {
		n := copy(b, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}
	if r.last {
		return 0, io.EOF
	}
	var control [5]byte
	if _, err := io.ReadFull(r.in, control[:]); err != nil || r.seq == ^uint64(0) {
		return 0, ErrInvalid
	}
	u := binary.BigEndian.Uint32(control[:4])
	final := control[4] == 1
	if u > chunkSize || control[4] > 1 || (u == 0) != final || (r.short && !final) {
		return 0, ErrInvalid
	}
	nonce, _, aad := r.frame(int(u), final)
	block := make([]byte, int(u)+r.aead.Overhead())
	if _, err := io.ReadFull(r.in, block); err != nil {
		return 0, ErrInvalid
	}
	plain, err := r.aead.Open(block[:0], nonce[:], block, aad)
	if err != nil {
		// An incorrect key and damage to the first authentication tag are indistinguishable.
		if r.seq == 0 {
			return 0, ErrBadPassphrase
		}
		return 0, ErrInvalid
	}
	r.seq++
	if final {
		if n, err := io.ReadFull(r.in, block[:1]); n != 0 || !errors.Is(err, io.EOF) {
			return 0, ErrInvalid
		}
		r.last = true
		return 0, io.EOF
	}
	r.short = len(plain) < chunkSize
	r.pending = plain
	return r.Read(b)
}

func sameSecret(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
