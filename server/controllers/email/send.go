package email

import (
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	svccompose "github.com/Jinnrry/pmail/services/compose"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/mailevents"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	"github.com/Jinnrry/pmail/utils/send"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"net/http"
	"time"
)

type sendRequest struct {
	ReplyToEmailID int              `json:"reply_to_email_id"`
	DraftID        int              `json:"draft_id"`
	ReplyTo        []user           `json:"reply_to"`
	From           user             `json:"from"`
	FromAddress    string           `json:"from_address"`
	To             []user           `json:"to"`
	Bcc            []user           `json:"bcc"`
	Cc             []user           `json:"cc"`
	Subject        string           `json:"subject"`
	Text           string           `json:"text"`   // Plaintext message (optional)
	HTML           string           `json:"html"`   // Html message (optional)
	Sender         user             `json:"sender"` // override From as SMTP envelope sender (optional)
	ReadReceipt    []string         `json:"read_receipt"`
	Attachments    []attachment     `json:"attrs"`
	InlineImages   []svccompose.Ref `json:"inline_images"`
}

type user struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type attachment struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func Send(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var reqData sendRequest
	if !readComposeJSON(ctx, w, req, &reqData) {
		return
	}
	log.WithContext(ctx).Infof("发送邮件")
	if len(reqData.To)+len(reqData.Cc)+len(reqData.Bcc) > mailbudget.Recipients {
		mailTooLarge(ctx, w)
		return
	}

	var savedDraft models.Email
	if reqData.DraftID != 0 {
		tx := db.Instance.NewSession()
		var err error
		savedDraft, err = findDraft(tx, ctx.UserID, reqData.DraftID)
		tx.Close()
		if err != nil {
			draftError(ctx, w, err)
			return
		}
	}

	if reqData.FromAddress == "" && reqData.From.Email == "" && reqData.ReplyToEmailID > 0 {
		reqData.FromAddress = preferredFrom(ctx.UserID, reqData.ReplyToEmailID)
	}

	if reqData.FromAddress != "" {
		resolved, err := address.SendAddress(ctx.UserID, reqData.FromAddress)
		if err != nil && !ctx.IsAdmin {
			response.Localized(ctx, response.NoAccessPrivileges, "mail_sender_forbidden").FPrintStatus(w, http.StatusForbidden)
			return
		}
		if err == nil {
			reqData.From.Email = resolved
		}
	}
	if reqData.From.Email != "" && !ctx.IsAdmin {
		resolved, err := address.SendAddress(ctx.UserID, reqData.From.Email)
		if err != nil {
			response.Localized(ctx, response.NoAccessPrivileges, "mail_sender_forbidden").FPrintStatus(w, http.StatusForbidden)
			return
		}
		reqData.From.Email = resolved
	}

	if reqData.From.Email == "" {
		reqData.From.Email, _ = address.SendAddress(ctx.UserID, "")
	}

	if reqData.From.Email == "" {
		response.Localized(ctx, response.ParamsError, "mail_sender_required").FPrint(w)
		return
	}

	if reqData.From.Name == "" {
		reqData.From.Name = ctx.UserName
	}

	if reqData.Subject == "" {
		response.Localized(ctx, response.ParamsError, "mail_subject_required").FPrint(w)
		return
	}

	if len(reqData.To) <= 0 {
		response.Localized(ctx, response.ParamsError, "mail_recipient_required").FPrint(w)
		return
	}

	e := &parsemail.Email{}

	for _, to := range reqData.To {
		e.To = append(e.To, &parsemail.User{
			Name:         to.Name,
			EmailAddress: to.Email,
		})
	}

	for _, bcc := range reqData.Bcc {
		e.Bcc = append(e.Bcc, &parsemail.User{
			Name:         bcc.Name,
			EmailAddress: bcc.Email,
		})
	}

	for _, cc := range reqData.Cc {
		e.Cc = append(e.Cc, &parsemail.User{
			Name:         cc.Name,
			EmailAddress: cc.Email,
		})
	}

	e.From = &parsemail.User{
		Name:         reqData.From.Name,
		EmailAddress: reqData.From.Email,
	}
	if reqData.Sender.Email != "" {
		e.Sender = &parsemail.User{
			Name:         reqData.Sender.Name,
			EmailAddress: reqData.Sender.Email,
		}
	} else {
		e.Sender = &parsemail.User{
			Name:         reqData.From.Name,
			EmailAddress: reqData.From.Email,
		}
	}

	e.Text = []byte(reqData.Text)
	e.HTML = []byte(parsemail.SanitizeHTML(reqData.HTML))
	e.Subject = reqData.Subject
	if reqData.DraftID != 0 && reqData.Attachments == nil {
		if savedDraft.Attachments != "" {
			if err := json.Unmarshal([]byte(savedDraft.Attachments), &e.Attachments); err != nil {
				draftError(ctx, w, err)
				return
			}
		}
	} else {
		atts, err := decodeComposeAttachments(reqData.Attachments)
		if err != nil {
			attachmentError(ctx, w, err)
			return
		}
		e.Attachments = atts
	}

	if err := svccompose.Apply(req.Context(), ctx.UserID, e, reqData.InlineImages, nil); err != nil {
		if !inlineError(ctx, w, err) {
			if stderrors.Is(err, mailbudget.ErrLimit) {
				mailTooLarge(ctx, w)
			} else {
				response.Localized(ctx, response.ServerError, "internal_error").FPrintStatus(w, 500)
			}
		}
		return
	}

	// Assign the message ID before measuring the final signed bytes so the
	// initial sent record and local deliveries use the same actual size.
	if e.MsgID == "" {
		e.MsgID = parsemail.GenerateMsgID(domain.DefaultName())
	}
	raw, buildErr := e.BuildBytes(ctx, true)
	if buildErr != nil {
		if stderrors.Is(buildErr, mailbudget.ErrLimit) {
			mailTooLarge(ctx, w)
			return
		}
		response.Localized(ctx, response.ParamsError, "mail_invalid_header").FPrintStatus(w, http.StatusBadRequest)
		return
	}
	e.Size = len(raw)

	modelEmail := models.Email{
		Type:         1,
		Subject:      e.Subject,
		ReplyTo:      json2string(e.ReplyTo),
		FromName:     e.From.Name,
		FromAddress:  e.From.EmailAddress,
		To:           json2string(e.To),
		Bcc:          json2string(e.Bcc),
		Cc:           json2string(e.Cc),
		Text:         sql.NullString{String: string(e.Text), Valid: true},
		Html:         sql.NullString{String: string(e.HTML), Valid: true},
		Sender:       json2string(e.Sender),
		Attachments:  json2string(e.Attachments),
		Size:         e.Size,
		SPFCheck:     1,
		DKIMCheck:    1,
		SendUserID:   ctx.UserID,
		SendDate:     time.Now(),
		CronSendTime: time.Now(),
		Status:       consts.EmailStatusDeliveryPending,
		CreateTime:   time.Now(),
		MsgID:        e.MsgID,
	}

	err := persistOutboundParent(&modelEmail, ctx.UserID)
	if err != nil || modelEmail.Id <= 0 {
		log.Printf("insert outbound parent error: %v", err)
		response.Localized(ctx, response.ServerError, "mail_persist_failed").FPrint(w)
		return
	}

	defer mailevents.Publish(ctx.UserID)
	e.MessageId = cast.ToInt64(modelEmail.Id)
	e.MsgID = modelEmail.MsgID

	deliveryResults, deliveryErr := send.Send(ctx, e)
	recipients := append(append(append([]*parsemail.User{}, e.To...), e.Cc...), e.Bcc...)
	result := summarizeDelivery(ctx, recipients, deliveryResults, deliveryErr)
	errorValue := errorString(result.firstFailure)
	if errorValue != "" {
		if _, err := db.Instance.Table(&models.Email{}).ID(modelEmail.Id).Cols("error").Update(map[string]any{"error": errorValue}); err != nil {
			log.WithContext(ctx).Errorf("sql Error :%+v", err)
		}
	}
	if err := queue.ReconcileParent(modelEmail.Id, ctx.UserID); err != nil {
		log.WithContext(ctx).Errorf("reconcile parent failed: %v", err)
	}
	if result.Status == "failed" {
		response.NewErrorResponse(response.ServerError, i18n.T(ctx, "send_fail"), result).FPrint(w)
		return
	}
	if reqData.DraftID != 0 {
		// Keep a draft on total delivery failure. A durable queued recipient
		// counts as acceptance, even if its delivery has not completed yet.
		if err := removeDrafts(ctx.UserID, []int{reqData.DraftID}); err != nil {
			// Do not tell the client to re-send an accepted message.
			log.WithContext(ctx).Errorf("Accepted send could not delete draft %d: %v", reqData.DraftID, err)
		}
	}
	response.NewSuccessResponse(result).FPrint(w)
}

func json2string(d any) string {
	by, _ := json.Marshal(d)
	return string(by)
}
