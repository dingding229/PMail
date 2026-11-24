package controllers

import (
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/translation"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func TranslationAPIPath(path string) bool {
	switch path {
	case "/api/admin/translation", "/api/admin/translation/test", "/api/settings/translation", "/api/settings/ai", "/api/settings/ai/test", "/api/email/translate", "/api/email/ai":
		return true
	}
	return false
}

func translationResponse(ctx *context.Context, w http.ResponseWriter, data any, err error) {
	if err == nil {
		inboundOK(w, data)
		return
	}
	code := translation.ErrorCode(err)
	status, number := http.StatusBadRequest, response.ParamsError
	switch code {
	case "translation_mail_not_found":
		status = http.StatusNotFound
	case "translation_daily_limit", "translation_upstream_rate_limited", "ai_busy":
		status = http.StatusTooManyRequests
	case "translation_disabled", "ai_not_configured", "ai_config_disabled":
		status = http.StatusConflict
	case "translation_timeout":
		status, number = http.StatusGatewayTimeout, response.ServerError
	case "translation_upstream_auth", "translation_connection_failed", "translation_response_invalid", "translation_provider_failed":
		status, number = http.StatusBadGateway, response.ServerError
	case "internal_error":
		status, number = http.StatusInternalServerError, response.ServerError
		log.WithContext(ctx).Warn("Translation API internal failure")
	}
	InboundError(ctx, w, status, number, code)
}

func translationUser(ctx *context.Context, w http.ResponseWriter) bool {
	if !inboundAccess(ctx, w, false) {
		return false
	}
	if ctx.TokenScope != "" {
		InboundError(ctx, w, http.StatusForbidden, response.NoAccessPrivileges, "inbound_permission_denied")
		return false
	}
	return true
}

func TranslationAdmin(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	if r.Method == http.MethodPut {
		var in translation.PolicyPatch
		if !inboundBody(ctx, w, r, &in) {
			return
		}
		data, err := translation.SetPolicy(r.Context(), in)
		if err == nil {
			log.WithContext(ctx).Infof("audit_event=translation_policy_changed actor_id=%d", ctx.UserID)
		}
		translationResponse(ctx, w, data, err)
		return
	}
	data, err := translation.GetPolicy(r.Context())
	translationResponse(ctx, w, data, err)
}

func TranslationTest(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	data, err := translation.Default.TestWeb(r.Context())
	translationResponse(ctx, w, data, err)
}

func TranslationPreferences(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !translationUser(ctx, w) {
		return
	}
	if r.Method == http.MethodPut {
		var in translation.Preferences
		if !inboundBody(ctx, w, r, &in) {
			return
		}
		data, err := translation.SetPreferences(r.Context(), ctx.UserID, in)
		if err == nil {
			log.WithContext(ctx).Infof("audit_event=translation_preferences_changed actor_id=%d", ctx.UserID)
		}
		translationResponse(ctx, w, data, err)
		return
	}
	data, err := translation.GetPreferences(r.Context(), ctx.UserID)
	translationResponse(ctx, w, data, err)
}

func UserAISettings(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !translationUser(ctx, w) {
		return
	}
	switch r.Method {
	case http.MethodPut:
		var in translation.AIInput
		if !inboundBody(ctx, w, r, &in) {
			return
		}
		data, err := translation.SetAI(r.Context(), ctx.UserID, in)
		if err == nil {
			log.WithContext(ctx).Infof("audit_event=user_ai_config_changed actor_id=%d", ctx.UserID)
		}
		translationResponse(ctx, w, data, err)
	case http.MethodDelete:
		err := translation.RemoveAI(r.Context(), ctx.UserID)
		if err == nil {
			log.WithContext(ctx).Infof("audit_event=user_ai_config_removed actor_id=%d", ctx.UserID)
		}
		translationResponse(ctx, w, nil, err)
	default:
		data, err := translation.GetAI(r.Context(), ctx.UserID)
		translationResponse(ctx, w, data, err)
	}
}

func UserAITest(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !translationUser(ctx, w) {
		return
	}
	data, err := translation.Default.TestAI(r.Context(), ctx.UserID)
	translationResponse(ctx, w, data, err)
}

func EmailTranslate(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var in translation.TranslateRequest
	if !translationUser(ctx, w) || !inboundBody(ctx, w, r, &in) {
		return
	}
	data, err := translation.Default.Translate(r.Context(), ctx.UserID, ctx.Lang, in)
	translationResponse(ctx, w, data, err)
}

func EmailAI(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var in translation.AIRequest
	if !translationUser(ctx, w) || !inboundBody(ctx, w, r, &in) {
		return
	}
	data, err := translation.Default.Analyze(r.Context(), ctx.UserID, ctx.Lang, in)
	translationResponse(ctx, w, data, err)
}
