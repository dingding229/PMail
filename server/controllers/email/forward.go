package email

import (
	"database/sql"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/detail"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	"github.com/Jinnrry/pmail/utils/send"
	log "github.com/sirupsen/logrus"
)

type forwardRequest struct {
	EmailID            int      `json:"email_id"`
	To                 []string `json:"to"`
	Cc                 []string `json:"cc"`
	Text               string   `json:"text"`
	HTML               string   `json:"html"`
	FromAddress        string   `json:"from_address"`
	IncludeAttachments *bool    `json:"include_attachments"`
}

func EmailForward(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var body forwardRequest
	if !readComposeJSON(ctx, w, req, &body) {
		return
	}
	if len(body.To)+len(body.Cc) > mailbudget.Recipients {
		mailTooLarge(ctx, w)
		return
	}
	if body.EmailID <= 0 {
		response.Localized(ctx, response.ParamsError, "invalid_json").FPrint(w)
		return
	}
	original, err := detail.GetEmailDetail(ctx, body.EmailID, false)
	if err != nil || original.MailboxStatus == consts.EmailStatusDrafts || original.Status == consts.EmailStatusDrafts {
		response.Localized(ctx, response.NoAccessPrivileges, "mail_not_found").FPrintStatus(w, http.StatusForbidden)
		return
	}
	fromAddress, err := address.SendAddress(ctx.UserID, body.FromAddress)
	if err != nil && !ctx.IsAdmin {
		response.Localized(ctx, response.NoAccessPrivileges, "mail_sender_forbidden").FPrintStatus(w, http.StatusForbidden)
		return
	}
	if fromAddress == "" {
		fromAddress = body.FromAddress
	}
	if fromAddress == "" {
		fromAddress, err = address.SendAddress(ctx.UserID, "")
		if err != nil {
			response.Localized(ctx, response.ParamsError, "mail_sender_required").FPrint(w)
			return
		}
	}
	to, err := forwardUsers(body.To)
	if err != nil || len(to) == 0 {
		response.Localized(ctx, response.ParamsError, "mail_recipient_required").FPrint(w)
		return
	}
	cc, err := forwardUsers(body.Cc)
	if err != nil {
		response.Localized(ctx, response.ParamsError, "mail_cc_invalid").FPrint(w)
		return
	}
	orig := parsemail.NewEmailFromModel(original.Email)
	e := buildForwardEmail(orig, ctx.UserName, fromAddress, to, cc, body.Text, body.HTML, body.IncludeAttachments)
	e.MsgID = parsemail.GenerateMsgID(fromDomain(fromAddress))
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
		Type: consts.EmailTypeSend, Subject: e.Subject, FromName: e.From.Name, FromAddress: e.From.EmailAddress,
		To: json2string(to), Cc: json2string(cc), Text: sql.NullString{String: string(e.Text), Valid: true},
		Html: sql.NullString{String: string(e.HTML), Valid: true}, Attachments: json2string(e.Attachments),
		Size: e.Size, SendUserID: ctx.UserID, SendDate: time.Now(), CronSendTime: time.Now(),
		CreateTime: time.Now(), Status: consts.EmailStatusDeliveryPending, MsgID: e.MsgID,
	}
	if err := persistOutboundParent(&modelEmail, ctx.UserID); err != nil {
		response.Localized(ctx, response.ServerError, "mail_persist_failed").FPrint(w)
		return
	}
	e.MessageId = int64(modelEmail.Id)
	deliveryErr, results := send.Send(ctx, e)
	status := consts.EmailStatusSent
	statusText := "sent"
	out := make([]map[string]any, 0, len(append(append([]*parsemail.User{}, to...), cc...)))
	delivered, queued := 0, 0
	for _, recipient := range append(append([]*parsemail.User{}, to...), cc...) {
		err, ok := results[recipient.EmailAddress]
		item := map[string]any{"address": recipient.EmailAddress, "status": "delivered", "error": ""}
		if ok && stderrors.Is(err, queue.ErrQueued) {
			item["status"] = "queued"
			queued++
		} else if ok && err != nil {
			item["status"] = "failed"
			item["error"] = i18n.Delivery(ctx, "delivery_detail", err.Error())
		} else {
			delivered++
		}
		out = append(out, item)
	}
	if queued > 0 {
		status = consts.EmailStatusDeliveryPending
		statusText = "queued"
	}
	if delivered+queued == 0 || deliveryErr != nil && delivered+queued == 0 {
		status = consts.EmailStatusFail
		statusText = "failed"
	}
	if _, err := db.Instance.Table(&models.Email{}).ID(modelEmail.Id).Cols("status", "error").Update(map[string]any{"status": status, "error": errorString(deliveryErr)}); err != nil {
		log.Errorf("update forwarded email %d failed: %v", modelEmail.Id, err)
	}
	if errorText := errorString(deliveryErr); errorText != "" {
		if _, err := db.Instance.Table(&models.Email{}).ID(modelEmail.Id).Cols("error").Update(map[string]any{"error": errorText}); err != nil {
			log.Errorf("update forwarded email %d error failed: %v", modelEmail.Id, err)
		}
	}
	if err := queue.ReconcileParent(modelEmail.Id, ctx.UserID); err != nil {
		log.Errorf("reconcile forwarded parent %d failed: %v", modelEmail.Id, err)
	}
	result := map[string]any{"status": statusText, "recipients": out}
	if delivered+queued == 0 {
		response.NewErrorResponse(response.ServerError, i18n.T(ctx, "forward_failed"), result).FPrint(w)
		return
	}
	response.NewSuccessResponse(result).FPrint(w)
}

func forwardUsers(values []string) ([]*parsemail.User, error) {
	out := make([]*parsemail.User, 0, len(values))
	for _, value := range values {
		user := parsemail.BuilderUser(strings.TrimSpace(value))
		if user == nil || user.EmailAddress == "" {
			return nil, fmt.Errorf("invalid recipient")
		}
		out = append(out, user)
	}
	return out, nil
}

func buildForwardEmail(original *parsemail.Email, name, from string, to, cc []*parsemail.User, extraText, extraHTML string, include *bool) *parsemail.Email {
	includeAttachments := include == nil || *include
	summary := fmt.Sprintf("---------- Forwarded message ----------\nFrom: %s\nDate: %s\nSubject: %s\nTo: %s\n\n", original.From.Build(), original.Date, original.Subject, original.BuildTo2String())
	text := extraText
	if text != "" {
		text += "\n\n"
	}
	text += summary + string(original.Text)
	html := parsemail.SanitizeHTML(extraHTML)
	if html != "" {
		html += "<br><br>"
	}
	html += fmt.Sprintf("<p>---------- Forwarded message ----------<br>From: %s<br>Date: %s<br>Subject: %s<br>To: %s</p>", original.From.Build(), original.Date, original.Subject, original.BuildTo2String())
	html += parsemail.SanitizeHTML(string(original.HTML))
	ret := &parsemail.Email{From: &parsemail.User{Name: name, EmailAddress: from}, To: to, Cc: cc, Subject: "Fwd: " + original.Subject, Text: []byte(text), HTML: []byte(html)}
	if includeAttachments {
		ret.Attachments = append(ret.Attachments, original.Attachments...)
	}
	return ret
}

func fromDomain(address string) string {
	_, host, _ := strings.Cut(address, "@")
	return host
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
