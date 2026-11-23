// Package antivirus supervises the image's local ClamAV and scans raw inbound mail.
package antivirus

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/utils/mailbudget"
)

const ScanTimeout = 30 * time.Second

var errReply = errors.New("unexpected clamd response")
var errLimits = errors.New("clamd scan limits exceeded")
var signatureUnsafe = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

type Result struct {
	Status string `json:"status"` // clean, infected, or not_scanned
	Reason string `json:"reason,omitempty"`
	Virus  string `json:"virus,omitempty"`
}

func roundTrip(ctx context.Context, socket, command string, write func(io.Writer) error) (string, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return "", err
		}
	}
	if _, err := io.WriteString(conn, "z"+command+"\x00"); err != nil {
		return "", err
	}
	if write != nil {
		if err := write(conn); err != nil {
			return "", err
		}
	}
	// ReadSlice bounds even a malformed response; clamd uses NUL after z-prefixed commands.
	response, err := bufio.NewReaderSize(conn, 512).ReadSlice(0)
	if err != nil {
		return "", err
	}
	return string(response[:len(response)-1]), nil
}

func scanRaw(ctx context.Context, socket string, message []byte) (string, error) {
	if len(message) > mailbudget.Message {
		return "", errLimits
	}
	response, err := roundTrip(ctx, socket, "INSTREAM", func(w io.Writer) error {
		const chunk = 32 << 10
		for len(message) > 0 {
			n := min(chunk, len(message))
			var header [4]byte
			binary.BigEndian.PutUint32(header[:], uint32(n)) // #nosec G115 -- n is clamped to one 32 KiB frame.
			if _, err := w.Write(header[:]); err != nil {
				return err
			}
			if _, err := w.Write(message[:n]); err != nil {
				return err
			}
			message = message[n:]
		}
		_, err := w.Write([]byte{0, 0, 0, 0})
		return err
	})
	if err != nil {
		return "", err
	}
	if response == "stream: OK" {
		return "", nil
	}
	name, ok := strings.CutPrefix(response, "stream: ")
	if !ok {
		return "", errReply
	}
	name, ok = strings.CutSuffix(name, " FOUND")
	if !ok || strings.TrimSpace(name) == "" {
		return "", errReply
	}
	if strings.HasPrefix(name, "Heuristics.Limits.Exceeded") {
		return "", errLimits
	}
	return signatureUnsafe.ReplaceAllString(name[:min(len(name), 180)], "_"), nil
}
