package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/Jinnrry/pmail/controllers/email"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/detail"
	"github.com/Jinnrry/pmail/services/imageproxy"
	"github.com/Jinnrry/pmail/services/telegram"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

// Telegram Mini App endpoints. They do not use the login session: the mail
// request is authenticated by Telegram's signed launch data, and the file and
// image requests by the short-lived ticket returned with the mail.

var webAppAttachmentURL = regexp.MustCompile(`/api/attachments/(\d+)\.(\d+)`)

type webAppAttachment struct {
	Index       int    `json:"index"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	URL         string `json:"url"`
}

type webAppMail struct {
	ID          int                `json:"id"`
	Subject     string             `json:"subject"`
	FromName    string             `json:"from_name"`
	FromAddress string             `json:"from_address"`
	To          string             `json:"to"`
	Date        time.Time          `json:"date"`
	HTML        string             `json:"html"`
	Text        string             `json:"text"`
	Attachments []webAppAttachment `json:"attachments"`
	Ticket      string             `json:"ticket"`
}

func webAppError(ctx *context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, telegram.ErrWebAppAuth):
		InboundError(ctx, w, http.StatusUnauthorized, response.NeedLogin, "telegram_webapp_auth")
	case errors.Is(err, telegram.ErrWebAppTicket):
		InboundError(ctx, w, http.StatusForbidden, response.NoAccessPrivileges, "telegram_webapp_ticket")
	default:
		log.WithContext(ctx).Warnf("Telegram Mini App: %v", err)
		InboundError(ctx, w, http.StatusInternalServerError, response.ServerError, "inbound_internal_error")
	}
}

// TelegramWebAppMail returns one mail for the Mini App page.
func TelegramWebAppMail(w http.ResponseWriter, r *http.Request) {
	ctx := i18n.ForRequest(r)
	var in struct {
		InitData string `json:"init_data"`
		ID       int    `json:"id"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	if err != nil || json.Unmarshal(body, &in) != nil || in.ID <= 0 {
		InboundError(ctx, w, http.StatusBadRequest, response.ParamsError, "inbound_invalid_parameters")
		return
	}
	uid, lang, err := telegram.VerifyWebApp(r.Context(), in.InitData, in.ID)
	if err != nil {
		webAppError(ctx, w, err)
		return
	}
	ctx.UserID = uid
	if l, ok := i18n.Preference(lang); ok && l != "" {
		ctx.Lang = l
	}
	mail, err := detail.WebDetail(ctx, in.ID, true)
	if err != nil {
		webAppError(ctx, w, telegram.ErrWebAppAuth)
		return
	}
	ticket := telegram.Ticket(uid, in.ID, time.Now())
	k := url.QueryEscape(ticket)
	out := webAppMail{
		ID: in.ID, Subject: mail.Subject, FromName: mail.FromName, FromAddress: mail.FromAddress, To: mail.To,
		Date: mail.SendDate, Text: mail.Text.String, Ticket: ticket, Attachments: []webAppAttachment{},
	}
	if out.Date.IsZero() {
		out.Date = mail.CreateTime
	}
	if html := email.ViewHTML(ctx, in.ID); html != "" {
		out.HTML = webAppAttachmentURL.ReplaceAllString(html, "/api/telegram/webapp/file/$1.$2?k="+k)
	}
	file := func(i int) string {
		return "/api/telegram/webapp/file/" + strconv.Itoa(in.ID) + "." + strconv.Itoa(i) + "?k=" + k
	}
	if mail.AttachmentInfo != nil {
		for _, a := range mail.AttachmentInfo {
			if !a.Inline {
				out.Attachments = append(out.Attachments, webAppAttachment{a.Index, a.Filename, a.ContentType, a.Size, file(a.Index)})
			}
		}
	} else {
		var parts []struct {
			Filename    string
			ContentType string
			ContentID   string
			Size        int
		}
		if json.Unmarshal([]byte(mail.Attachments), &parts) == nil {
			for i, a := range parts {
				if a.ContentID == "" {
					out.Attachments = append(out.Attachments, webAppAttachment{i, a.Filename, a.ContentType, a.Size, file(i)})
				}
			}
		}
	}
	inboundOK(w, out)
}

func webAppTicketContext(w http.ResponseWriter, r *http.Request, mailID int) (*context.Context, bool) {
	ctx := i18n.ForRequest(r)
	uid, err := telegram.CheckTicket(r.Context(), r.URL.Query().Get("k"), mailID, time.Now())
	if err != nil {
		webAppError(ctx, w, err)
		return nil, false
	}
	ctx.UserID = uid
	return ctx, true
}

// TelegramWebAppFile serves an attachment or inline image of the ticket's mail.
func TelegramWebAppFile(w http.ResponseWriter, r *http.Request) {
	mail, _, _ := cutDot(r.PathValue("id"))
	id, err := strconv.Atoi(mail)
	if err != nil || id <= 0 {
		InboundError(i18n.ForRequest(r), w, http.StatusBadRequest, response.ParamsError, "inbound_invalid_parameters")
		return
	}
	if ctx, ok := webAppTicketContext(w, r, id); ok {
		GetAttachments(ctx, w, r)
	}
}

// TelegramWebAppImage loads an external image of the ticket's mail through
// the image privacy proxy.
func TelegramWebAppImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		InboundError(i18n.ForRequest(r), w, http.StatusBadRequest, response.ParamsError, "inbound_invalid_parameters")
		return
	}
	ctx, ok := webAppTicketContext(w, r, id)
	if !ok {
		return
	}
	q := r.URL.Query()
	q.Del("k")
	q.Set("t", imageproxy.Token(ctx.UserID, id))
	r.URL.RawQuery = q.Encode()
	email.RemoteImage(ctx, w, r)
}

func cutDot(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}
