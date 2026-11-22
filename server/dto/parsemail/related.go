package parsemail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"

	"github.com/Jinnrry/pmail/utils/wiretext"
	"github.com/emersion/go-message/mail"
	msgproto "github.com/emersion/go-message/textproto"
)

func nested(parent *multipart.Writer, kind string, params map[string]string, write func(*multipart.Writer) error) error {
	// Allocate a random boundary without buffering a nested part.
	boundary := multipart.NewWriter(io.Discard).Boundary()
	params["boundary"] = boundary
	h := make(textproto.MIMEHeader)
	h.Set("Content-Type", mime.FormatMediaType(kind, params))
	part, err := parent.CreatePart(h)
	if err != nil {
		return err
	}
	child := multipart.NewWriter(part)
	if err = child.SetBoundary(boundary); err != nil {
		return err
	}
	if err = write(child); err != nil {
		return err
	}
	return child.Close()
}
func encodedPart(w *multipart.Writer, h textproto.MIMEHeader, data []byte) error {
	h.Set("Content-Transfer-Encoding", "base64")
	p, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	var line [78]byte
	for len(data) > 0 {
		n := min(57, len(data))
		size := base64.StdEncoding.EncodedLen(n)
		base64.StdEncoding.Encode(line[:size], data[:n])
		line[size] = '\r'
		line[size+1] = '\n'
		if _, err = p.Write(line[:size+2]); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}
func bodyPart(p *multipart.Writer, media string, data []byte) error {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Type", media+"; charset=UTF-8")
	return encodedPart(p, h, data)
}
func filePart(p *multipart.Writer, a *Attachment) error {
	if a == nil || !wiretext.HeaderValue(a.Filename) || !wiretext.HeaderValue(a.ContentType) {
		return ErrInvalidMailHeader
	}
	ct := a.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil || !strings.Contains(mt, "/") {
		return ErrInvalidMailHeader
	}
	for k, v := range params {
		if !wiretext.HeaderName(k) || !wiretext.HeaderValue(v) {
			return ErrInvalidMailHeader
		}
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Type", mime.FormatMediaType(mt, params))
	kind := "attachment"
	if a.Inline {
		kind = "inline"
	}
	h.Set("Content-Disposition", mime.FormatMediaType(kind, map[string]string{"filename": a.Filename}))
	if a.ContentID != "" {
		if !wiretext.HeaderValue(a.ContentID) || strings.ContainsAny(a.ContentID, "<> \t") {
			return ErrInvalidMailHeader
		}
		h.Set("Content-ID", "<"+a.ContentID+">")
	}
	return encodedPart(p, h, a.Content)
}

// Preserve the plain text alternative, and associate CIDs with precisely the
// HTML alternative instead of treating them as downloadable mixed siblings.
func relatedBytes(e *Email, h mail.Header) ([]byte, error) {
	var out bytes.Buffer
	root := multipart.NewWriter(&out)
	h.Set("MIME-Version", "1.0")
	h.SetContentType("multipart/mixed", map[string]string{"boundary": root.Boundary()})
	if err := msgproto.WriteHeader(&out, h.Header.Header); err != nil {
		return nil, err
	}
	err := nested(root, "multipart/alternative", map[string]string{}, func(alt *multipart.Writer) error {
		if err := bodyPart(alt, "text/plain", e.Text); err != nil {
			return err
		}
		return nested(alt, "multipart/related", map[string]string{"type": "text/html"}, func(related *multipart.Writer) error {
			if err := bodyPart(related, "text/html", e.HTML); err != nil {
				return err
			}
			for _, a := range e.Attachments {
				if a != nil && a.Inline {
					if err := filePart(related, a); err != nil {
						return err
					}
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	for _, a := range e.Attachments {
		if a == nil || !a.Inline {
			if err = filePart(root, a); err != nil {
				return nil, err
			}
		}
	}
	if err = root.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
