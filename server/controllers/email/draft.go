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
	svccompose "github.com/Jinnrry/pmail/services/compose"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

type draftRequest struct {
	ID           int              `json:"id"`
	FromAddress  string           `json:"from_address"`
	To           []user           `json:"to"`
	Cc           []user           `json:"cc"`
	Bcc          []user           `json:"bcc"`
	Subject      string           `json:"subject"`
	HTML         string           `json:"html"`
	Text         string           `json:"text"`
	Attachments  []attachment     `json:"attrs"`
	InlineImages []svccompose.Ref `json:"inline_images"`
}

var (
	errDraftNotFound  = errors.New("draft not found")
	errAttachmentData = errors.New("invalid attachment data URL")
)

func decodeComposeAttachments(attrs []attachment) ([]*parsemail.Attachment, error) {
	if len(attrs)+4 > mailbudget.Parts {
		return nil, mailbudget.ErrLimit
	}
	out := make([]*parsemail.Attachment, 0, len(attrs))
	total := 0
	for _, att := range attrs {
		raw, dataURL := strings.CutPrefix(att.Data, "data:")
		contentType, data, encoded := strings.Cut(raw, ";base64,")
		if media, _, err := mime.ParseMediaType(contentType); !dataURL || !encoded || err != nil || !strings.Contains(media, "/") {
			return nil, errAttachmentData
		}
		content, err := mailbudget.Read(base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)), int64(min(mailbudget.Part, mailbudget.Attachments-total)))
		if err != nil {
			return nil, err
		}
		total += len(content)
		if len(content) > mailbudget.Part || total > mailbudget.Attachments {
			return nil, mailbudget.ErrLimit
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
	if !readComposeJSON(ctx, w, r, &body) {
		return
	}
	if len(body.To)+len(body.Cc)+len(body.Bcc) > mailbudget.Recipients || len(body.Text) > mailbudget.Part || len(body.HTML) > mailbudget.Part {
		mailTooLarge(ctx, w)
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
	if draft.Text.String == "" && draft.Html.String != "" {
		draft.Text.String = parsemail.HTMLText(draft.Html.String)
	}
	draft.TextFormat, _ = parsemail.CodeMetadata(draft.Text.String, draft.Html.String, "")
	if err := persistDraft(&draft, body.Attachments == nil, body.InlineImages); err != nil {
		if !inlineError(ctx, w, err) {
			draftError(ctx, w, err)
		}
		return
	}
	response.NewSuccessResponse(struct {
		ID int `json:"id"`
		models.EmailISOTimes
	}{draft.Id, draft.ISOTimes()}).FPrint(w)
}

func persistDraft(draft *models.Email, keepAttachments bool, inline ...[]svccompose.Ref) error {
	var refs []svccompose.Ref
	if len(inline) > 0 {
		refs = inline[0]
	}
	creating := draft.Id == 0
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if _, err := quota.Lock(tx, draft.SendUserID); err != nil {
			return err
		}
		now := time.Now()
		draft.SendDate = now
		if creating {
			draft.Id = 0 // SQLite may replay a rolled-back insert.
			draft.MsgID = parsemail.GenerateMsgID(domain.DefaultName())
			draft.CronSendTime = now
			draft.Size = len(draft.Text.String) + len(draft.Html.String) + len(draft.Attachments)
			if draft.Size > mailbudget.Message {
				return mailbudget.ErrLimit
			}
			mailstore.Prepare(draft)
			if _, err := tx.Insert(draft); err != nil {
				return err
			}
			if err := svccompose.SyncDraft(tx, draft.SendUserID, draft.Id, draft.Html.String, refs); err != nil {
				return err
			}
			if err := mailstore.WriteParts(tx, draft, false); err != nil {
				return err
			}
			link := models.UserEmail{UserID: draft.SendUserID, EmailID: draft.Id, Status: consts.EmailStatusDrafts, IsRead: 1, AddressAttributed: true}
			if _, err := tx.Insert(&link); err != nil {
				return err
			}
			if err := mailaddress.Add(tx, []models.UserEmail{link}, []string{draft.FromAddress}); err != nil {
				return err
			}
			if err := quota.Adjust(tx, draft.SendUserID, int64(draft.Size)); err != nil {
				return err
			}
			return readDraftTimes(tx, draft)
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
		if draft.Size > mailbudget.Message {
			return mailbudget.ErrLimit
		}
		mailstore.Prepare(draft)
		updates := map[string]any{"projection_ready": true, "list_snippet": draft.ListSnippet, "from_address": draft.FromAddress, "from_name": draft.FromName, "sender": draft.Sender, "subject": draft.Subject, "to": draft.To, "cc": draft.Cc, "bcc": draft.Bcc, "text": draft.Text.String, "html": draft.Html.String, "text_format": draft.TextFormat, "attachments": draft.Attachments, "size": draft.Size, "send_date": now}
		if _, err := tx.Table(&models.Email{}).ID(draft.Id).Where("send_user_id=? AND status=?", draft.SendUserID, consts.EmailStatusDrafts).Update(updates); err != nil {
			return err
		}
		if err := svccompose.SyncDraft(tx, draft.SendUserID, draft.Id, draft.Html.String, refs); err != nil {
			return err
		}
		if !keepAttachments || !stored.ProjectionReady {
			if err := mailstore.WriteParts(tx, draft, true); err != nil {
				return err
			}
		}
		if draft.FromAddress != stored.FromAddress {
			if err := mailaddress.ReplaceDraft(tx, draft.SendUserID, draft.Id, draft.FromAddress); err != nil {
				return err
			}
		}
		if draft.Size != stored.Size {
			n, err := tx.Where("user_id=? AND email_id=?", draft.SendUserID, draft.Id).Count(&models.UserEmail{})
			if err != nil {
				return err
			}
			if err = quota.Adjust(tx, draft.SendUserID, n*(int64(draft.Size)-int64(stored.Size))); err != nil {
				return err
			}
		}
		return readDraftTimes(tx, draft)
	})
}

// Read the stored timestamps after Xorm has handled its created/updated hooks.
func readDraftTimes(tx *xorm.Session, draft *models.Email) error {
	var stored models.Email
	found, err := tx.ID(draft.Id).Cols("send_date", "create_time", "update_time", "cron_send_time").Get(&stored)
	if err != nil {
		return err
	}
	if !found {
		return errDraftNotFound
	}
	draft.SendDate, draft.CreateTime, draft.UpdateTime, draft.CronSendTime = stored.SendDate, stored.CreateTime, stored.UpdateTime, stored.CronSendTime
	return nil
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
		if _, err := quota.Lock(tx, owner); err != nil {
			return err
		}
		for _, id := range ids {
			size, err := quota.MessageSize(tx, id)
			if err != nil {
				return err
			}
			var links []int
			if err := tx.Table(&models.UserEmail{}).Where("user_id=? AND email_id=? AND status=?", owner, id, consts.EmailStatusDrafts).Cols("id").Find(&links); err != nil {
				return err
			}
			removed, err := tx.Where("user_id=? AND email_id=? AND status=?", owner, id, consts.EmailStatusDrafts).Delete(new(models.UserEmail))
			if err != nil {
				return err
			}
			if err := mailaddress.DeleteLinks(tx, links); err != nil {
				return err
			}
			if removed == 0 {
				continue
			}
			if err := quota.Adjust(tx, owner, -removed*size); err != nil {
				return err
			}
			linked, err := tx.Where("email_id=?", id).Exist(new(models.UserEmail))
			if err != nil {
				return err
			}
			if linked {
				continue
			}
			deleted, err := tx.ID(id).Where("send_user_id=? AND status=?", owner, consts.EmailStatusDrafts).Delete(new(models.Email))
			if err != nil {
				return err
			}
			if deleted > 0 {
				if err := svccompose.DeleteDraft(tx, id); err != nil {
					return err
				}
				if err := mailstore.DeleteParts(tx, id); err != nil {
					return err
				}
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
	if errors.Is(err, mailbudget.ErrLimit) {
		mailTooLarge(ctx, w)
		return
	}
	if errors.Is(err, errAttachmentData) {
		response.Localized(ctx, response.ParamsError, "attachment_encoding_invalid").FPrint(w)
		return
	}
	response.Localized(ctx, response.ParamsError, "att_err").FPrint(w)
}

func draftError(ctx *context.Context, w http.ResponseWriter, err error) {
	if errors.Is(err, mailbudget.ErrLimit) {
		mailTooLarge(ctx, w)
		return
	}
	if errors.Is(err, errDraftNotFound) {
		response.Localized(ctx, response.ParamsError, "mail_draft_not_found").FPrintStatus(w, http.StatusNotFound)
		return
	}
	log.WithContext(ctx).Errorf("Draft operation: %v", err)
	response.Localized(ctx, response.ServerError, "internal_error").FPrintStatus(w, http.StatusInternalServerError)
}
