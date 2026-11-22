package mailbudget

import (
	"errors"
	"io"
	"os"
	"strconv"
)

const (
	DefaultJSON = 32 << 20
	Message     = 32 << 20
	Part        = 24 << 20
	Attachments = 24 << 20
	Parts       = 256
	MaxDepth    = 32
	Recipients  = 50
	POPLine     = 64 << 10
	POPListing  = 4 << 20
)

var ErrLimit = errors.New("mail resource limit exceeded")

func JSON() int64 {
	if v, err := strconv.ParseInt(os.Getenv("PMAIL_MAIL_JSON_MAX_BYTES"), 10, 64); err == nil && v >= 1024 && v <= 128<<20 {
		return v
	}
	return DefaultJSON
}

func Read(r io.Reader, limit int64) ([]byte, error) {
	out, err := io.ReadAll(io.LimitReader(r, limit+1))
	if int64(len(out)) > limit {
		return nil, ErrLimit
	}
	return out, err
}
