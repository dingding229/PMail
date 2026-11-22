package attachments

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func InlineContentType(raw string) (string, bool) { return mailstore.InlineContentType(raw) }

type Part struct {
	models.MailAttachmentInfo
	legacy []byte
}

// Find authorizes the email before dereferencing either the new metadata or
// legacy JSON. The returned reader is pinned to one attachment generation.
func Find(ctx *context.Context, emailID, index int, cid string) *Part {
	var mail models.Email
	ok, err := db.Instance.ID(emailID).Cols("id", "status", "projection_ready").Get(&mail)
	if err != nil || !ok || !auth.HasAuth(ctx, &mail) {
		return nil
	}
	if mail.ProjectionReady {
		var info models.MailAttachmentInfo
		q := db.Instance.Where("email_id=?", emailID).Asc("part")
		if index >= 0 {
			q.And("part=?", index)
		} else {
			q.And("content_id=?", cid)
		}
		if ok, err := q.Get(&info); err == nil && ok {
			return &Part{MailAttachmentInfo: info}
		}
		return nil
	}
	ok, err = db.Instance.ID(emailID).Cols("attachments").Get(&mail)
	if err != nil || !ok {
		return nil
	}
	var atts []parsemail.Attachment
	if err := json.Unmarshal([]byte(mail.Attachments), &atts); err != nil {
		return nil
	}
	for i, a := range atts {
		if index == i || (index < 0 && a.ContentID == cid) {
			info := mailstore.Metadata([]parsemail.Attachment{a})[0]
			info.Index = i
			return &Part{MailAttachmentInfo: info, legacy: a.Content}
		}
	}
	return nil
}

func (p *Part) Open(ctx stdctx.Context) io.Reader {
	if p.ID == 0 {
		return bytes.NewReader(p.legacy)
	}
	return &chunkReader{ctx: ctx, part: p.MailAttachmentInfo}
}

type chunkReader struct {
	ctx             stdctx.Context
	part            models.MailAttachmentInfo
	chunk, position int
	current         []byte
}

func (r *chunkReader) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if r.position == r.part.Size {
		return 0, io.EOF
	}
	if len(r.current) == 0 {
		var c models.MailAttachmentChunk
		ok, err := db.Instance.Context(r.ctx).Where("email_id=? AND part_id=? AND chunk_no=?", r.part.EmailID, r.part.ID, r.chunk).Get(&c)
		if err != nil {
			return 0, err
		}
		expected := min(mailstore.ChunkSize, r.part.Size-r.position)
		if !ok || len(c.Data) != expected {
			return 0, io.ErrUnexpectedEOF
		}
		r.current = c.Data
		r.chunk++
	}
	n := copy(b, r.current)
	r.current = r.current[n:]
	r.position += n
	return n, nil
}

func (p *Part) Preview(ctx stdctx.Context) (parsemail.TextPreview, bool) {
	if !p.Previewable {
		return parsemail.TextPreview{}, false
	}
	raw, err := io.ReadAll(io.LimitReader(p.Open(ctx), int64(min(p.Size, parsemail.PreviewLimit+8))))
	if err != nil {
		return parsemail.TextPreview{}, false
	}
	// Older preview validated the entire attachment before slicing. Keep
	// incomplete UTF-8 at the read boundary from changing charset detection.
	if p.UTF8 && len(raw) < p.Size {
		for !utf8.Valid(raw) {
			raw = raw[:len(raw)-1]
		}
	}
	v, ok := parsemail.PreviewText(&parsemail.Attachment{Filename: p.Filename, ContentType: p.ContentType, Charset: p.Charset, Content: raw})
	v.Size = p.Size
	v.Truncated = v.Truncated || len(raw) < p.Size
	return v, ok
}

func LogStreamError(ctx *context.Context, err error) {
	log.WithContext(ctx).Warnf("attachment stream ended early: %v", err)
}
