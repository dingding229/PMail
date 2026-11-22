package controllers

import (
	"errors"
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/session"
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
	u, err := auth.Authenticate(data.Account, data.Password, req.RemoteAddr)
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
	if err := session.Login(req.Context(), &u); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, map[string]any{"account": u.Account, "name": u.Name, "is_admin": u.IsAdmin, "domains": domain.Names()})
}

func Logout(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	if err := session.Instance.Destroy(req.Context()); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, i18n.T(ctx, "succ"))
}
