package email

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

type draftRequest struct {
	ID          int          `json:"id"`
	FromAddress string       `json:"from_address"`
	To          []user       `json:"to"`
	Cc          []user       `json:"cc"`
	Bcc         []user       `json:"bcc"`
	Subject     string       `json:"subject"`
	HTML        string       `json:"html"`
	Text        string       `json:"text"`
	Attachments []attachment `json:"attrs"`
}

var (
	errDraftNotFound  = errors.New("draft not found")
	errAttachmentData = errors.New("invalid attachment data URL")
)

func decodeComposeAttachments(attrs []attachment) ([]*parsemail.Attachment, error) {
	out := make([]*parsemail.Attachment, 0, len(attrs))
	for _, att := range attrs {
		raw, dataURL := strings.CutPrefix(att.Data, "data:")
		contentType, data, encoded := strings.Cut(raw, ";base64,")
		if media, _, err := mime.ParseMediaType(contentType); !dataURL || !encoded || err != nil || !strings.Contains(media, "/") {
			return nil, errAttachmentData
		}
		content, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, err
		}
		out = append(out, &parsemail.Attachment{Filename: att.Name, ContentType: contentType, Content: content})
	}
	return out, nil
}

func composeUsers(input []user) []*parsemail.User {
	output := make([]*parsemail.User, 0, len(input))
	for _, item := range input {
		output = append(output, &parsemail.User{Name: item.Name, EmailAddress: item.Email})
	}
	return output
}

func SaveDraft(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body draftRequest
	if !readMailJSON(ctx, w, r, &body, false) {
		return
	}
	if body.ID < 0 {
		response.Localized(ctx, response.ParamsError, "invalid_id").FPrint(w)
		return
	}
	from, err := address.SendAddress(ctx.UserID, body.FromAddress)
	if err != nil {
		response.Localized(ctx, response.NoAccessPrivileges, "mail_sender_forbidden").FPrintStatus(w, http.StatusForbidden)
		return
	}
	atts, err := decodeComposeAttachments(body.Attachments)
	if err != nil {
		attachmentError(ctx, w, err)
		return
	}
	draft := models.Email{
		Id: body.ID, Type: consts.EmailTypeReceive, Status: consts.EmailStatusDrafts, SendUserID: ctx.UserID,
		FromAddress: from, FromName: ctx.UserName, Subject: body.Subject,
		To: json2string(composeUsers(body.To)), Cc: json2string(composeUsers(body.Cc)), Bcc: json2string(composeUsers(body.Bcc)),
		Text: sql.NullString{String: body.Text, Valid: true}, Html: sql.NullString{String: parsemail.SanitizeHTML(body.HTML), Valid: true},
		Attachments: json2string(atts),
	}
	draft.Sender = json2string(parsemail.User{Name: draft.FromName, EmailAddress: from})
	draft.TextFormat, _ = parsemail.CodeMetadata(draft.Text.String, draft.Html.String, "")
	if err := persistDraft(&draft, body.Attachments == nil); err != nil {
		draftError(ctx, w, err)
		return
	}
	response.NewSuccessResponse(map[string]int{"id": draft.Id}).FPrint(w)
}

func persistDraft(draft *models.Email, keepAttachments bool) error {
	creating := draft.Id == 0
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		now := time.Now()
		draft.SendDate = now
		if creating {
			draft.Id = 0 // SQLite may replay a rolled-back insert.
			draft.MsgID = parsemail.GenerateMsgID(domain.DefaultName())
			draft.CronSendTime = now
			draft.Size = len(draft.Text.String) + len(draft.Html.String) + len(draft.Attachments)
			if _, err := tx.Insert(draft); err != nil {
				return err
			}
			_, err := tx.Insert(&models.UserEmail{UserID: draft.SendUserID, EmailID: draft.Id, Status: consts.EmailStatusDrafts, IsRead: 1})
			return err
		}
		stored, err := findDraft(tx, draft.SendUserID, draft.Id)
		if err != nil {
			return err
		}
		if stored.Status != consts.EmailStatusDrafts || stored.SendUserID != draft.SendUserID {
			return errDraftNotFound
		}
		if keepAttachments {
			draft.Attachments = stored.Attachments
		}
		draft.Size = len(draft.Text.String) + len(draft.Html.String) + len(draft.Attachments)
		updates := map[string]any{"from_address": draft.FromAddress, "from_name": draft.FromName, "sender": draft.Sender, "subject": draft.Subject, "to": draft.To, "cc": draft.Cc, "bcc": draft.Bcc, "text": draft.Text.String, "html": draft.Html.String, "text_format": draft.TextFormat, "attachments": draft.Attachments, "size": draft.Size, "send_date": now, "update_time": now}
		_, err = tx.Table(&models.Email{}).ID(draft.Id).Where("send_user_id=? AND status=?", draft.SendUserID, consts.EmailStatusDrafts).Update(updates)
		return err
	})
}

func findDraft(tx *xorm.Session, owner, id int) (models.Email, error) {
	var row models.Email
	found, err := tx.Where("user_id=? AND email_id=? AND status=?", owner, id, consts.EmailStatusDrafts).Exist(new(models.UserEmail))
	if err != nil {
		return row, err
	}
	if !found {
		return row, errDraftNotFound
	}
	found, err = tx.ID(id).Get(&row)
	if err == nil && !found {
		err = errDraftNotFound
	}
	return row, err
}

func removeDrafts(owner int, ids []int) error {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		for _, id := range ids {
			removed, err := tx.Where("user_id=? AND email_id=? AND status=?", owner, id, consts.EmailStatusDrafts).Delete(new(models.UserEmail))
			if err != nil {
				return err
			}
			if removed == 0 {
				continue
			}
			linked, err := tx.Where("email_id=?", id).Exist(new(models.UserEmail))
			if err != nil {
				return err
			}
			if linked {
				continue
			}
			if _, err := tx.ID(id).Where("send_user_id=? AND status=?", owner, consts.EmailStatusDrafts).Delete(new(models.Email)); err != nil {
				return err
			}
		}
		return nil
	})
}

func DeleteDraft(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID  int   `json:"id"`
		IDs []int `json:"ids"`
	}
	if !readMailJSON(ctx, w, r, &body, false) {
		return
	}
	if len(body.IDs) == 0 && body.ID > 0 {
		body.IDs = []int{body.ID}
	}
	if !validIDs(body.IDs) {
		response.Localized(ctx, response.ParamsError, "mail_ids_required").FPrint(w)
		return
	}
	if err := removeDrafts(ctx.UserID, body.IDs); err != nil {
		draftError(ctx, w, err)
		return
	}
	response.NewSuccessResponse(i18n.T(ctx, "succ")).FPrint(w)
}

func attachmentError(ctx *context.Context, w http.ResponseWriter, err error) {
	if errors.Is(err, errAttachmentData) {
		response.Localized(ctx, response.ParamsError, "attachment_encoding_invalid").FPrint(w)
		return
	}
	response.Localized(ctx, response.ParamsError, "att_err").FPrint(w)
}

func draftError(ctx *context.Context, w http.ResponseWriter, err error) {
	if errors.Is(err, errDraftNotFound) {
		response.Localized(ctx, response.ParamsError, "mail_draft_not_found").FPrintStatus(w, http.StatusNotFound)
		return
	}
	log.WithContext(ctx).Errorf("Draft operation: %v", err)
	response.Localized(ctx, response.ServerError, "internal_error").FPrintStatus(w, http.StatusInternalServerError)
}
