package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func InboundAPIPath(path string) bool {
	switch path {
	case "/api/email/spam", "/api/spam/rules", "/api/settings/spam", "/api/admin/inbound", "/api/admin/inbound/clamav", "/api/admin/inbound/clamav/update", "/api/admin/trash":
		return true
	}
	return false
}

func InboundError(ctx *context.Context, w http.ResponseWriter, status, number int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	response.NewErrorResponse(number, i18n.T(ctx, code), map[string]string{"code": code}).FPrintStatus(w, status)
}
func inboundOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	success(w, data)
}
func inboundErr(ctx *context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, inbound.ErrInvalid):
		InboundError(ctx, w, 400, response.ParamsError, "inbound_invalid_parameters")
	case errors.Is(err, inbound.ErrNotFound):
		InboundError(ctx, w, 404, response.ParamsError, "inbound_resource_not_found")
	case errors.Is(err, inbound.ErrRuleConflict):
		InboundError(ctx, w, 409, response.ParamsError, "spam_rule_conflict")
	case errors.Is(err, inbound.ErrRuleLimit):
		InboundError(ctx, w, 409, response.ParamsError, "spam_rule_limit")
	default:
		log.WithContext(ctx).Warnf("inbound API failed: %v", err)
		InboundError(ctx, w, 500, response.ServerError, "inbound_internal_error")
	}
}
func inboundBody(ctx *context.Context, w http.ResponseWriter, r *http.Request, into any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		InboundError(ctx, w, 415, response.ParamsError, "json_media_type_required")
		return false
	}
	read := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	read.DisallowUnknownFields()
	if err = read.Decode(into); err == nil {
		err = read.Decode(new(any))
		if errors.Is(err, io.EOF) {
			return true
		}
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		InboundError(ctx, w, 413, response.ParamsError, "inbound_payload_too_large")
	} else {
		inboundErr(ctx, w, inbound.ErrInvalid)
	}
	return false
}
func inboundAccess(ctx *context.Context, w http.ResponseWriter, admin bool) bool {
	if ctx.UserID <= 0 {
		InboundError(ctx, w, 401, response.NeedLogin, "inbound_login_required")
		return false
	}
	if admin && !ctx.IsAdmin {
		InboundError(ctx, w, 403, response.NoAccessPrivileges, "inbound_permission_denied")
		return false
	}
	return true
}

func InboundSettings(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	var s inbound.Settings
	var err error
	if r.Method == http.MethodPut {
		var body struct {
			DNSBLEnabled         *bool                  `json:"dnsbl_enabled"`
			Zones                *[]string              `json:"dnsbl_zones"`
			DefaultSpamThreshold *float64               `json:"default_spam_threshold"`
			RiskAudit            *bool                  `json:"risk_block_dangerous_attachments"`
			CatchAll             inbound.CatchAllUpdate `json:"catch_all"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		if body.DNSBLEnabled == nil || body.Zones == nil || body.DefaultSpamThreshold == nil || body.RiskAudit == nil {
			inboundErr(ctx, w, inbound.ErrInvalid)
			return
		}
		s, err = inbound.PutSettings(r.Context(), inbound.Settings{DNSBLEnabled: *body.DNSBLEnabled, Zones: *body.Zones, DefaultSpamThreshold: *body.DefaultSpamThreshold, RiskAudit: *body.RiskAudit}, body.CatchAll)
		if err == nil {
			log.WithContext(ctx).Infof("audit_event=inbound_settings_changed actor_id=%d", ctx.UserID)
		}
	} else {
		s, err = inbound.GetSettings(r.Context())
	}
	if err != nil {
		inboundErr(ctx, w, err)
		return
	}
	s.Warnings = inbound.Warnings(s)
	for i := range s.Warnings {
		s.Warnings[i].Message = i18n.T(ctx, s.Warnings[i].Code)
	}
	recipients, err := inbound.CatchAllOptions(r.Context())
	if err != nil {
		inboundErr(ctx, w, err)
		return
	}
	inboundOK(w, struct {
		inbound.Settings
		CatchAllRecipients []inbound.CatchAllOption `json:"catch_all_recipients"`
	}{s, recipients})
}

func SpamSettings(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	var p models.SpamPreference
	var err error
	if r.Method == http.MethodPut {
		var body struct {
			Enabled   *bool    `json:"enabled"`
			Threshold *float64 `json:"threshold"`
			Bayes     *bool    `json:"bayes_enabled"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		if body.Enabled == nil || body.Threshold == nil || body.Bayes == nil {
			inboundErr(ctx, w, inbound.ErrInvalid)
			return
		}
		p, err = inbound.PutPreference(r.Context(), ctx.UserID, models.SpamPreference{Enabled: *body.Enabled, Threshold: *body.Threshold, BayesEnabled: *body.Bayes})
	} else {
		var s inbound.Settings
		s, err = inbound.GetSettings(r.Context())
		if err == nil {
			p, err = inbound.Preference(r.Context(), ctx.UserID, s.DefaultSpamThreshold)
		}
	}
	if err != nil {
		inboundErr(ctx, w, err)
		return
	}
	inboundOK(w, p)
}

func SpamRules(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Action string `json:"action"`
			Kind   string `json:"kind"`
			Value  string `json:"value"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		rule, err := inbound.AddRule(r.Context(), ctx.UserID, models.SpamRule{Action: body.Action, Kind: body.Kind, Value: body.Value})
		if err != nil {
			inboundErr(ctx, w, err)
			return
		}
		inboundOK(w, rule)
	case http.MethodDelete:
		var body struct {
			ID int `json:"id"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		if err := inbound.DeleteRule(r.Context(), ctx.UserID, body.ID); err != nil {
			inboundErr(ctx, w, err)
			return
		}
		inboundOK(w, "")
	default:
		rules, err := inbound.Rules(r.Context(), ctx.UserID)
		if err != nil {
			inboundErr(ctx, w, err)
			return
		}
		inboundOK(w, rules)
	}
}

func MarkSpam(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	var body struct {
		IDs  []int `json:"ids"`
		Spam *bool `json:"is_spam"`
	}
	if !inboundBody(ctx, w, r, &body) {
		return
	}
	if body.Spam == nil {
		inboundErr(ctx, w, inbound.ErrInvalid)
		return
	}
	if err := inbound.MarkSpam(r.Context(), ctx.UserID, body.IDs, *body.Spam); err != nil {
		inboundErr(ctx, w, err)
		return
	}
	inboundOK(w, "")
}
