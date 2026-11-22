package controllers

import (
	"errors"
	"github.com/Jinnrry/pmail/utils/clientip"
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/registration"
	"github.com/Jinnrry/pmail/services/user"
	"github.com/Jinnrry/pmail/session"
	"github.com/Jinnrry/pmail/utils/context"
)

func Register(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	ready, err := registration.Pending(r.Context())
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	if !ready {
		apiError(ctx, w, http.StatusForbidden, response.NoAccessPrivileges, "already_initialized")
		return
	}
	finish, ok := auth.ReserveRegistration(clientip.Source(r))
	if !ok {
		w.Header().Set("Retry-After", "600")
		apiError(ctx, w, http.StatusTooManyRequests, response.ParamsError, "registration_rate_limit")
		return
	}
	failed := false
	defer func() { finish(failed) }()
	var body struct {
		Key         string `json:"registration_key"`
		Account     string `json:"account"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		failed = true
		return
	}
	u, err := registration.Register(r.Context(), body.Key, body.Account, body.DisplayName, body.Password)
	switch {
	case errors.Is(err, registration.ErrInitialized):
		apiError(ctx, w, http.StatusForbidden, response.NoAccessPrivileges, "already_initialized")
		return
	case errors.Is(err, registration.ErrInvalidKey):
		failed = true
		apiError(ctx, w, http.StatusForbidden, response.NoAccessPrivileges, "registration_key_invalid")
		return
	case errors.Is(err, registration.ErrAccount), errors.Is(err, registration.ErrPassword), errors.Is(err, registration.ErrDisplayName):
		failed = true
		key := "registration_account_invalid"
		if errors.Is(err, registration.ErrPassword) {
			key = "registration_password_invalid"
		} else if errors.Is(err, registration.ErrDisplayName) {
			key = "invalid_display_name"
		}
		apiError(ctx, w, http.StatusBadRequest, response.ParamsError, key)
		return
	case err != nil:
		serviceError(ctx, w, err)
		return
	}
	views, err := user.Views([]models.User{u})
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	if err := session.Login(r.Context(), &u); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, apiCurrentUser(views[0]))
}
