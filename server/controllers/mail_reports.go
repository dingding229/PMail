package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/listen/imap_server"
	"github.com/Jinnrry/pmail/services/reportmail"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func ReportMailAPIPath(path string) bool {
	switch path {
	case "/api/settings/report-mail", "/api/settings/report-mail/apply", "/api/admin/mail-reports/summary", "/api/admin/mail-reports/records", "/api/admin/mail-reports/reports":
		return true
	}
	return false
}

func reportResponse(ctx *context.Context, w http.ResponseWriter, data any, err error) {
	switch {
	case err == nil:
		inboundOK(w, data)
	case errors.Is(err, reportmail.ErrGroup):
		InboundError(ctx, w, http.StatusBadRequest, response.ParamsError, "report_mail_invalid_group")
	case errors.Is(err, reportmail.ErrQuery):
		InboundError(ctx, w, http.StatusBadRequest, response.ParamsError, "mail_reports_invalid")
	default:
		if ctx.Context == nil || ctx.Err() == nil {
			log.WithContext(ctx).Warn("Mail reports API operation failed")
		}
		InboundError(ctx, w, http.StatusInternalServerError, response.ServerError, "inbound_internal_error")
	}
}

func ReportMailPreferences(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	if r.Method == http.MethodGet {
		p, err := reportmail.Preference(r.Context(), ctx.UserID)
		reportResponse(ctx, w, p, err)
		return
	}
	var body struct {
		Enabled  *bool `json:"enabled"`
		GroupID  *int  `json:"group_id"`
		MarkRead *bool `json:"mark_read"`
		Forward  *bool `json:"forward_reports"`
	}
	if !inboundBody(ctx, w, r, &body) {
		return
	}
	p, err := reportmail.UpdatePreference(r.Context(), ctx.UserID, body.Enabled, body.GroupID, body.MarkRead, body.Forward)
	reportResponse(ctx, w, p, err)
}

func ReportMailApply(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	p, err := reportmail.Apply(r.Context(), ctx.UserID)
	if p.Moved > 0 {
		imap_server.MailboxChanged(ctx.UserID)
	}
	reportResponse(ctx, w, p, err)
}

func AdminMailReports(view string) HandlerFunc {
	return func(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
		if !inboundAccess(ctx, w, true) {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		q := r.URL.Query()
		window, err := reportmail.ResolveWindow(r.Context(), q.Get("domain"), q.Get("from"), q.Get("to"), time.Now())
		if err != nil {
			reportResponse(ctx, w, nil, err)
			return
		}
		if view == "summary" {
			s, err := reportmail.Summary(r.Context(), window, nil)
			reportResponse(ctx, w, s, err)
			return
		}
		page := 1
		if q.Has("page") {
			page, err = strconv.Atoi(q.Get("page"))
			if err != nil || page < 1 {
				reportResponse(ctx, w, nil, reportmail.ErrQuery)
				return
			}
		}
		if view == "records" {
			p, err := reportmail.Records(r.Context(), window, ctx.UserID, page, q.Get("result"))
			reportResponse(ctx, w, p, err)
			return
		}
		p, err := reportmail.Reports(r.Context(), window, ctx.UserID, page)
		reportResponse(ctx, w, p, err)
	}
}
