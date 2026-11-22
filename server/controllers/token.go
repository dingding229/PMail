package controllers

import (
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
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	row, raw, err := token.Create(ctx.UserID, body.Name, body.ExpiresAt)
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
