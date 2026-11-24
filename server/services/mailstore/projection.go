package mailstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	pcontext "github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

const ChunkSize = 32 << 10
const SnippetSize = 1024

func Snippet(s string) string {
	if len(s) <= SnippetSize {
		return s
	}
	n := SnippetSize
	for !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// Prepare must run before insert/update; WriteParts runs in the same transaction.
func Prepare(row *models.Email) {
	row.ListSnippet = Snippet(row.Text.String)
	row.ProjectionReady = true
}

func Metadata(atts []parsemail.Attachment) []models.MailAttachmentInfo {
	rows := make([]models.MailAttachmentInfo, 0, len(atts))
	for i, a := range atts {
		_, inline := InlineContentType(a.ContentType)
		rows = append(rows, models.MailAttachmentInfo{Index: i, Filename: a.Filename, ContentType: a.ContentType, ContentID: a.ContentID, Charset: a.Charset, Size: len(a.Content), Inline: inline, Previewable: parsemail.Previewable(&a), UTF8: utf8.Valid(a.Content)})
	}
	return rows
}

var errMalformedAttachment = errors.New("invalid legacy attachment JSON")

func WriteParts(tx *xorm.Session, row *models.Email, replace bool) error {
	var parts []parsemail.Attachment
	if row.Attachments != "" {
		if err := json.Unmarshal([]byte(row.Attachments), &parts); err != nil {
			return errMalformedAttachment
		}
	}
	if replace {
		if err := DeleteParts(tx, row.Id); err != nil {
			return err
		}
	}
	for _, m := range Metadata(parts) {
		m.EmailID = row.Id
		if _, err := tx.Insert(&m); err != nil {
			return err
		}
		d := parts[m.Index].Content
		for offset := 0; offset < len(d); offset += ChunkSize {
			c := models.MailAttachmentChunk{EmailID: row.Id, Part: m.ID, Chunk: offset / ChunkSize, Data: d[offset:min(len(d), offset+ChunkSize)]}
			if _, err := tx.Insert(&c); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeleteDerived is called only after deletion of the last message reference.
func DeleteDerived(tx *xorm.Session, id int) error {
	if err := DeleteParts(tx, id); err != nil {
		return err
	}
	for _, row := range []any{&models.EmailTranslation{}, &models.EmailAIResult{}} {
		if _, err := tx.Where("email_id=?", id).Delete(row); err != nil {
			return err
		}
	}
	// Statistics outlive the original message. Clear its link so an ID
	// reused after a backup restore cannot point at an unrelated message.
	for _, model := range []any{&models.DMARCReport{}, &models.TLSReport{}} {
		if _, err := tx.Table(model).Where("email_id=?", id).Update(map[string]any{"email_id": 0}); err != nil {
			return err
		}
	}

	return nil
}

func DeleteParts(tx *xorm.Session, id int) error {
	if _, err := tx.Where("email_id=?", id).Delete(&models.MailAttachmentChunk{}); err != nil {
		return err
	}
	_, err := tx.Where("email_id=?", id).Delete(&models.MailAttachmentInfo{})
	return err
}

var crlf = strings.NewReplacer("\r\n", "\r\n", "\n", "\r\n", "\r", "\r\n")

func POPMessage(ctx *pcontext.Context, row models.Email) (string, error) {
	raw, err := parsemail.NewEmailFromModel(row).BuildBytes(ctx, false)
	if err != nil {
		return "", err
	}
	// The wire builder can emit bare CR/LF in body data. The identical helper is
	// used by RETR and while caching the unescaped octet count.
	out := crlf.Replace(string(raw))
	if !strings.HasSuffix(out, "\r\n") {
		out += "\r\n"
	}
	return out, nil
}

func SetPOPSize(ctx *pcontext.Context, row *models.Email) {
	if row.Type != 0 || row.MsgID == "" || row.SendDate.IsZero() {
		return
	}
	if raw, err := POPMessage(ctx, *row); err == nil {
		row.POPOctets = int64(len(raw))
	}
}

// Backfill processes bounded ID batches and commits one message at a time.
// New writes publish metadata atomically; a restart resumes at the first unset row.
func Backfill(ctx context.Context, e *xorm.Engine) error {
	cursor := 0
	for ctx.Err() == nil {
		var ids []models.Email
		if err := e.Context(ctx).Cols("id").Where("id>? AND projection_ready=?", cursor, false).Asc("id").Limit(64).Find(&ids); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		for _, key := range ids {
			cursor = key.Id
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := db.Transaction(e, func(tx *xorm.Session) error {
				query := tx.Context(ctx).ID(key.Id).Where("projection_ready=?", false)
				if e.DriverName() != config.DBTypeSQLite {
					query.ForUpdate()
				}
				var row models.Email
				ok, err := query.Get(&row)
				if err != nil || !ok {
					return err
				}
				Prepare(&row)
				SetPOPSize(&pcontext.Context{Context: ctx}, &row)
				if err = WriteParts(tx, &row, false); err != nil {
					// Malformed historical JSON remains on the compatibility path.
					if errors.Is(err, errMalformedAttachment) {
						log.Warnf("mail projection skipped malformed attachment data: id=%d", row.Id)
						return nil
					}
					return err
				}
				_, err = tx.NoAutoTime().ID(row.Id).Cols("projection_ready", "list_snippet", "pop_octets").Update(&row)
				return err
			}); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return ctx.Err()
}

// LoadMetadata fetches all parts for the bounded result page in one query.
func LoadMetadata(rows []*models.Email) error {
	ids := make([]int, 0, len(rows))
	byID := map[int][]models.MailAttachmentInfo{}
	for _, row := range rows {
		if row.ProjectionReady {
			ids = append(ids, row.Id)
			byID[row.Id] = []models.MailAttachmentInfo{}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var parts []models.MailAttachmentInfo
	if err := db.Instance.In("email_id", ids).Asc("email_id", "part").Find(&parts); err != nil {
		return err
	}
	for _, part := range parts {
		byID[part.EmailID] = append(byID[part.EmailID], part)
	}
	for _, row := range rows {
		if row.ProjectionReady {
			row.AttachmentInfo = byID[row.Id]
		}
	}
	return nil
}
