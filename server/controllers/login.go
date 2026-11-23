package controllers

import (
	"errors"
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/session"
	"github.com/Jinnrry/pmail/utils/clientip"
	"github.com/Jinnrry/pmail/utils/context"
)

type loginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

func Login(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var data loginRequest
	if !readRequest(ctx, w, req, &data, false) {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	u, challenge, err := auth.BeginWebLogin(data.Account, data.Password, clientip.Source(req))
	if errors.Is(err, auth.ErrLocked) {
		apiError(ctx, w, http.StatusTooManyRequests, response.ParamsError, "rate_limit")
		return
	}
	if errors.Is(err, auth.ErrCredentials) {
		response.Localized(ctx, response.ParamsError, "aperror").FPrint(w)
		return
	}
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	if challenge != "" {
		success(w, map[string]any{"two_factor_required": true, "challenge": challenge, "expires_in": 300})
		return
	}
	completeLogin(ctx, w, req, &u)
}

func LoginSecondFactor(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	var body struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
		Recovery  bool   `json:"recovery"`
	}
	if !readRequest(ctx, w, req, &body, false) {
		return
	}
	u, err := auth.FinishWebLogin(req.Context(), body.Challenge, body.Code, clientip.Source(req), body.Recovery)
	if err != nil {
		factorError(ctx, w, err)
		return
	}
	completeLogin(ctx, w, req, &u)
}

func completeLogin(ctx *context.Context, w http.ResponseWriter, req *http.Request, u *models.User) {
	if err := session.Login(req.Context(), u); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, map[string]any{"account": u.Account, "name": u.Name, "is_admin": u.IsAdmin, "domains": domain.Names()})
}

func Logout(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		apiError(ctx, w, http.StatusMethodNotAllowed, response.ParamsError, "method_not_allowed")
		return
	}

	if err := session.Instance.Destroy(req.Context()); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, i18n.T(ctx, "succ"))
}
