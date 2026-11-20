package email

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/hooks"
	"github.com/Jinnrry/pmail/hooks/framework"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/utils/async"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/send"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"io"
	"net/http"
	"strings"
	"time"
)

type sendRequest struct {
	ReplyTo     []user       `json:"reply_to"`
	From        user         `json:"from"`
	FromAddress string       `json:"from_address"`
	To          []user       `json:"to"`
	Bcc         []user       `json:"bcc"`
	Cc          []user       `json:"cc"`
	Subject     string       `json:"subject"`
	Text        string       `json:"text"`   // Plaintext message (optional)
	HTML        string       `json:"html"`   // Html message (optional)
	Sender      user         `json:"sender"` // override From as SMTP envelope sender (optional)
	ReadReceipt []string     `json:"read_receipt"`
	Attachments []attachment `json:"attrs"`
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
	reqBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.WithContext(ctx).Errorf("%+v", err)
		response.NewErrorResponse(response.ParamsError, "params error", err.Error()).FPrint(w)
		return
	}
	log.WithContext(ctx).Infof("发送邮件")

	var reqData sendRequest
	err = json.Unmarshal(reqBytes, &reqData)
	if err != nil {
		log.WithContext(ctx).Errorf("%+v", err)
		response.NewErrorResponse(response.ParamsError, "params error", err.Error()).FPrint(w)
		return
	}

	if reqData.FromAddress != "" {
		resolved, err := address.SendAddress(ctx.UserID, reqData.FromAddress)
		if err != nil && !ctx.IsAdmin {
			response.NewErrorResponse(response.NoAccessPrivileges, "forbidden sender address", "").FPrintStatus(w, http.StatusForbidden)
			return
		}
		if err == nil {
			reqData.From.Email = resolved
		}
	}
	if reqData.From.Email != "" && !ctx.IsAdmin {
		resolved, err := address.SendAddress(ctx.UserID, reqData.From.Email)
		if err != nil {
			response.NewErrorResponse(response.NoAccessPrivileges, "forbidden sender address", "").FPrintStatus(w, http.StatusForbidden)
			return
		}
		reqData.From.Email = resolved
	}

	if reqData.From.Email == "" {
		reqData.From.Email, _ = address.SendAddress(ctx.UserID, "")
	}

	if reqData.From.Email == "" {
		response.NewErrorResponse(response.ParamsError, "发件人必填", "发件人必填").FPrint(w)
		return
	}

	if reqData.From.Name == "" {
		reqData.From.Name = ctx.UserName
	}

	if reqData.Subject == "" {
		response.NewErrorResponse(response.ParamsError, "邮件标题必填", "邮件标题必填").FPrint(w)
		return
	}

	if len(reqData.To) <= 0 {
		response.NewErrorResponse(response.ParamsError, "收件人必填", "收件人必填").FPrint(w)
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
	e.HTML = []byte(reqData.HTML)
	e.Subject = reqData.Subject
	for _, att := range reqData.Attachments {
		att.Data = strings.TrimPrefix(att.Data, "data:")
		infos := strings.Split(att.Data, ";")
		contentType := infos[0]
		content := strings.TrimPrefix(infos[1], "base64,")
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			log.WithContext(ctx).Errorf("附件解码错误！%v", err)
			response.NewErrorResponse(response.ParamsError, i18n.GetText(ctx.Lang, "att_err"), err.Error()).FPrint(w)
			return
		}
		e.Attachments = append(e.Attachments, &parsemail.Attachment{
			Filename:    att.Name,
			ContentType: contentType,
			Content:     decoded,
		})

	}

	// Build the exact message once so outbound and local-receive records use the actual byte size.
	e.Size = len(e.BuildBytes(ctx, false))

	log.WithContext(ctx).Debugf("插件执行--SendBefore")
	for _, hook := range hooks.AllHooks() {
		if hook == nil {
			continue
		}
		hook.SendBefore(ctx, e)
	}
	log.WithContext(ctx).Debugf("插件执行--SendBefore End")

	// Assign the message ID before measuring the final signed bytes so the
	// initial sent record and local deliveries use the same actual size.
	if e.MsgID == "" {
		e.MsgID = parsemail.GenerateMsgID(domain.DefaultName())
	}
	e.Size = len(e.BuildBytes(ctx, true))

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

	err = persistOutboundParent(&modelEmail, ctx.UserID)
	if err != nil || modelEmail.Id <= 0 {
		log.Printf("insert outbound parent error: %v", err)
		response.NewErrorResponse(response.ServerError, i18n.GetText(ctx.Lang, "send_fail"), errorString(err)).FPrint(w)
		return
	}

	e.MessageId = cast.ToInt64(modelEmail.Id)
	e.MsgID = modelEmail.MsgID

	deliveryErr, deliveryResults := send.Send(ctx, e)
	log.WithContext(ctx).Debugf("插件执行--SendAfter")
	as2 := async.New(ctx)
	for _, hook := range hooks.AllHooks() {
		if hook == nil {
			continue
		}
		as2.WaitProcess(func(hk any) {
			hk.(framework.EmailHook).SendAfter(ctx, e, deliveryResults)
		}, hook)
	}
	as2.Wait()
	log.WithContext(ctx).Debugf("插件执行--SendAfter")

	allRecipients := append(append(append([]*parsemail.User{}, e.To...), e.Cc...), e.Bcc...)
	recipientsResult := make([]map[string]any, 0, len(allRecipients))
	delivered, queued, failed := 0, 0, 0
	var firstFailure error
	for _, recipient := range allRecipients {
		deliveryResult, exists := deliveryResults[recipient.EmailAddress]
		status := "delivered"
		errorText := ""
		switch {
		case exists && stderrors.Is(deliveryResult, queue.ErrQueued):
			status = "queued"
			queued++
		case exists && deliveryResult != nil:
			status = "failed"
			errorText = deliveryResult.Error()
			failed++
			if firstFailure == nil {
				firstFailure = deliveryResult
			}
		default:
			delivered++
		}
		recipientsResult = append(recipientsResult, map[string]any{"address": recipient.EmailAddress, "status": status, "error": errorText})
	}
	if deliveryErr != nil && firstFailure == nil {
		firstFailure = deliveryErr
	}
	if delivered+queued == 0 {
		failed = len(allRecipients)
	}
	statusText := "sent"
	if queued > 0 && delivered+queued > 0 {
		statusText = "queued"
	}
	if delivered+queued == 0 {
		statusText = "failed"
	}
	errorValue := ""
	if firstFailure != nil {
		errorValue = firstFailure.Error()
	}
	if errorValue != "" {
		if _, err := db.Instance.Table(&models.Email{}).ID(modelEmail.Id).Cols("error").Update(map[string]any{"error": errorValue}); err != nil {
			log.WithContext(ctx).Errorf("sql Error :%+v", err)
		}
	}
	if err := queue.ReconcileParent(modelEmail.Id, ctx.UserID); err != nil {
		log.WithContext(ctx).Errorf("reconcile parent failed: %v", err)
	}
	result := map[string]any{"status": statusText, "recipients": recipientsResult}
	if delivered+queued == 0 {
		response.NewErrorResponse(response.ServerError, i18n.GetText(ctx.Lang, "send_fail"), result).FPrint(w)
		return
	}
	response.NewSuccessResponse(result).FPrint(w)
}

func json2string(d any) string {
	by, _ := json.Marshal(d)
	return string(by)
}
