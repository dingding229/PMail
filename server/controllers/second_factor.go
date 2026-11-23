package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/session"
	"github.com/Jinnrry/pmail/utils/clientip"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func factorError(ctx *context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrLocked):
		InboundError(ctx, w, 429, response.ParamsError, "rate_limit")
	case errors.Is(err, auth.ErrCredentials):
		InboundError(ctx, w, 400, response.ParamsError, "aperror")
	case errors.Is(err, auth.ErrFactorCode), errors.Is(err, auth.ErrAppInput):
		InboundError(ctx, w, 400, response.ParamsError, err.Error())
	case errors.Is(err, auth.ErrChallenge), errors.Is(err, auth.ErrFactorState), errors.Is(err, auth.ErrAppLimit):
		InboundError(ctx, w, 409, response.ParamsError, err.Error())
	case errors.Is(err, auth.ErrAppMissing):
		InboundError(ctx, w, 404, response.ParamsError, err.Error())
	default:
		// Authentication errors must never include factor material or request bodies.
		log.WithContext(ctx).Error("2FA or app-password operation failed")
		apiError(ctx, w, 500, response.ServerError, "internal_error")
	}
}

func factorAccess(ctx *context.Context, w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	if ctx.TokenScope != "" {
		apiError(ctx, w, 403, response.NoAccessPrivileges, "access_denied")
		return nil, false
	}
	u, err := session.CurrentUser(r.Context())
	if err != nil {
		factorError(ctx, w, err)
		return nil, false
	}
	if u == nil || u.ID != ctx.UserID {
		apiError(ctx, w, 401, response.NeedLogin, "login_exp")
		return nil, false
	}
	return u, true
}

func FactorStatus(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	u, ok := factorAccess(ctx, w, r)
	if ok {
		success(w, map[string]bool{"enabled": u.TOTPEnabled})
	}
}

func SetupFactor(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	u, ok := factorAccess(ctx, w, r)
	if !ok {
		return
	}
	setup, err := auth.SetupFactor(r.Context(), *u)
	if err != nil {
		factorError(ctx, w, err)
		return
	}
	success(w, setup)
}

func EnableFactor(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	u, ok := factorAccess(ctx, w, r)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	fresh, codes, err := auth.EnableFactor(r.Context(), *u, body.Code, clientip.Source(r))
	if err != nil {
		factorError(ctx, w, err)
		return
	}
	if err := session.Login(r.Context(), &fresh); err != nil {
		factorError(ctx, w, err)
		return
	}
	log.WithContext(ctx).Infof("audit_event=two_factor_enabled actor_id=%d", u.ID)
	success(w, map[string]any{"enabled": true, "recovery_codes": codes})
}

func ManageFactor(disable bool) HandlerFunc {
	return func(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
		u, ok := factorAccess(ctx, w, r)
		if !ok {
			return
		}
		var body struct {
			Password string `json:"password"`
			Code     string `json:"code"`
			Recovery bool   `json:"recovery"`
		}
		if !readRequest(ctx, w, r, &body, false) {
			return
		}
		fresh, codes, err := auth.ManageFactor(r.Context(), *u, body.Password, body.Code, clientip.Source(r), body.Recovery, disable)
		if err != nil {
			factorError(ctx, w, err)
			return
		}
		if disable {
			if err := session.Login(r.Context(), &fresh); err != nil {
				factorError(ctx, w, err)
				return
			}
			log.WithContext(ctx).Infof("audit_event=two_factor_disabled actor_id=%d", u.ID)
			success(w, map[string]bool{"enabled": false})
			return
		}
		success(w, map[string]any{"enabled": true, "recovery_codes": codes})
	}
}

func ResetUserFactor(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if _, ok := factorAccess(ctx, w, r); !ok || !adminOnly(ctx, w) {
		return
	}
	var body struct {
		ID int `json:"id"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	if body.ID <= 0 || body.ID == ctx.UserID {
		apiError(ctx, w, 400, response.ParamsError, "two_factor_reset_target")
		return
	}
	if err := auth.ResetFactor(r.Context(), body.ID); err != nil {
		factorError(ctx, w, err)
		return
	}
	log.WithContext(ctx).Infof("audit_event=two_factor_admin_reset actor_id=%d target_id=%d", ctx.UserID, body.ID)
	success(w, nil)
}

func AppPasswords(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	u, ok := factorAccess(ctx, w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		rows, err := auth.AppPasswords(r.Context(), u.ID)
		if err != nil {
			factorError(ctx, w, err)
			return
		}
		success(w, rows)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	value, err := auth.CreateAppPassword(r.Context(), *u, body.Name)
	if err != nil {
		factorError(ctx, w, err)
		return
	}
	success(w, value)
}

func DeleteAppPassword(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	u, ok := factorAccess(ctx, w, r)
	if !ok {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		factorError(ctx, w, auth.ErrAppInput)
		return
	}
	if err := auth.DeleteAppPassword(r.Context(), *u, id); err != nil {
		factorError(ctx, w, err)
		return
	}
	success(w, nil)
}
