package list

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/utils/context"
)

type MailCursor struct {
	ID, Link int
	Date     string
}

func DecodeCursor(value string) (MailCursor, error) {
	if value == "" {
		return MailCursor{}, nil
	}
	var c MailCursor
	if len(value) > 256 {
		return c, errors.New("invalid cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || json.Unmarshal(raw, &c) != nil || c.ID <= 0 || c.Link <= 0 || len(c.Date) > 48 {
		return c, errors.New("invalid cursor")
	}
	return c, nil
}

func EncodeCursor(row *response.EmailResponseData) string {
	raw, _ := json.Marshal(MailCursor{row.Id, row.UeId, row.CursorDate})
	return base64.RawURLEncoding.EncodeToString(raw)
}

const webFields = `e.id,e.type,e.status,e.send_date,e.create_time,e.update_time,e.cron_send_time,
 e.from_address,e.from_name,e.reply_to,e.sender,e.subject,e.%s,e.cc,e.bcc,e.error,e.auth_results,e.inbound_analysis,e.auth_unchecked,e.spf_check,e.dkim_check,e.projection_ready,
 CASE WHEN e.projection_ready THEN e.list_snippet ELSE SUBSTR(e.text,1,1024) END AS list_snippet,
 ue.is_read,ue.is_starred,ue.inbound_spam,ue.status AS mailbox_status,ue.id AS ue_id,ue.catch_all_addresses,ue.deleted_at`

func webPageSQL(ctx *context.Context, tag dto.SearchTag, keyword string, cursor MailCursor, offset, limit int, filters ...mailaddress.Filter) (string, []any, string, []any, error) {
	// Reuse the legacy mailbox semantics, including IMAP copies and drafts.
	countSQL, args := genSQL(ctx, true, tag, keyword, false, 0, 0, filters...)
	from := strings.TrimPrefix(countSQL, "select count(1)")
	query := "SELECT ue.id,ue.email_id" + from
	pageArgs := append([]any(nil), args...)
	draft := tag.Status == consts.EmailStatusDrafts && !tag.Starred
	if cursor.ID > 0 {
		if draft {
			if cursor.Date == "" {
				return "", nil, "", nil, errors.New("invalid draft cursor")
			}
			query += " AND (e.send_date < ? OR (e.send_date = ? AND (ue.email_id < ? OR (ue.email_id = ? AND ue.id < ?))))"
			pageArgs = append(pageArgs, cursor.Date, cursor.Date, cursor.ID, cursor.ID, cursor.Link)
		} else {
			query += " AND (ue.email_id < ? OR (ue.email_id = ? AND ue.id < ?))"
			pageArgs = append(pageArgs, cursor.ID, cursor.ID, cursor.Link)
		}
	}
	query += " ORDER BY "
	if draft {
		query += "e.send_date DESC,"
	}
	query += "ue.email_id DESC,ue.id DESC"
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	// Resolve the page before projecting contents. In particular SQLite must not
	// decode wide rows while sorting the candidate mailbox.
	cols := fmt.Sprintf(webFields, db.Instance.Quote("to"))
	if draft {
		castType := "TEXT"
		if db.Instance.DriverName() == "mysql" {
			castType = "CHAR"
		}
		cols += ",CAST(e.send_date AS " + castType + ") AS cursor_date"
	}
	outer := "SELECT " + cols + " FROM (" + query + ") p JOIN user_email ue ON ue.id=p.id JOIN email e ON e.id=p.email_id ORDER BY "
	if draft {
		outer += "e.send_date DESC,"
	}
	outer += "ue.email_id DESC,ue.id DESC"
	return outer, pageArgs, countSQL, args, nil
}

func WebPage(ctx *context.Context, tag dto.SearchTag, keyword string, cursor MailCursor, offset, limit int, count bool, filters ...mailaddress.Filter) ([]*response.EmailResponseData, int64, error) {
	outer, pageArgs, countSQL, args, err := webPageSQL(ctx, tag, keyword, cursor, offset, limit, filters...)
	if err != nil {
		return nil, 0, err
	}

	var rows []*response.EmailResponseData
	if err := db.Instance.SQL(outer, pageArgs...).Find(&rows); err != nil {
		return nil, 0, err
	}
	var total int64
	if count {
		if _, err := db.Instance.SQL(countSQL, args...).Get(&total); err != nil {
			return nil, 0, err
		}
	}
	var legacy []int
	mail := make([]*models.Email, 0, len(rows))
	for _, row := range rows {
		mail = append(mail, &row.Email)
		if !row.ProjectionReady {
			legacy = append(legacy, row.Id)
		}
	}
	if err := mailstore.LoadMetadata(mail); err != nil {
		return nil, 0, err
	}
	// Legacy rows are progressively backfilled. Keep only the compatibility
	// attachment payload reads for the current page, not the full list query.
	if len(legacy) > 0 {
		var stored []models.Email
		if err := db.Instance.Cols("id", "attachments").In("id", legacy).Find(&stored); err != nil {
			return nil, 0, err
		}
		byID := map[int]string{}
		for _, row := range stored {
			byID[row.Id] = row.Attachments
		}
		for _, row := range rows {
			row.Attachments = byID[row.Id]
		}
	}
	return rows, total, nil
}

// FolderCount uses the same predicates as WebPage, without reading MIME/body data.
func FolderCount(ctx *context.Context, tag dto.SearchTag, filter mailaddress.Filter) (total, unread int, err error) {
	q, args := genSQL(ctx, true, tag, "", false, 0, 0, filter)
	q = "SELECT COUNT(1) AS total,COALESCE(SUM(CASE WHEN ue.is_read=0 THEN 1 ELSE 0 END),0) AS unread" + strings.TrimPrefix(q, "select count(1)")
	var row struct{ Total, Unread int }
	_, err = db.Instance.SQL(q, args...).Get(&row)
	return row.Total, row.Unread, err
}
