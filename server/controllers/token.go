package controllers

import (
	"errors"
	"net/http"
	"time"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/token"
	"github.com/Jinnrry/pmail/utils/context"
)

func TokenList(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	rows, err := token.List(ctx.UserID)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, mapViews(rows, apiToken))
}
func TokenCreate(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string     `json:"name"`
		ExpiresAt    *time.Time `json:"expires_at"`
		Scope        string     `json:"scope"`
		NeverExpires bool       `json:"never_expires"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	row, raw, err := token.Create(ctx.UserID, body.Name, body.Scope, body.ExpiresAt, body.NeverExpires)
	if errors.Is(err, token.ErrScope) || errors.Is(err, token.ErrExpiry) {
		key := "token_invalid_scope"
		if errors.Is(err, token.ErrExpiry) {
			key = "token_invalid_expiry"
		}
		apiError(ctx, w, http.StatusBadRequest, response.ParamsError, key)
		return
	}
	if errors.Is(err, token.ErrAdminScope) {
		apiError(ctx, w, http.StatusForbidden, response.NoAccessPrivileges, "token_scope_forbidden")
		return
	}
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, map[string]any{"token": raw, "item": apiToken(row)})
}
func TokenDelete(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	if body.ID <= 0 {
		apiError(ctx, w, http.StatusBadRequest, response.ParamsError, "invalid_id")
		return
	}
	if err := token.Delete(body.ID, ctx.UserID, ctx.IsAdmin); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, nil)
}
