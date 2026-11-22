package controllers

import (
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/user"
	"github.com/Jinnrry/pmail/session"
	"github.com/Jinnrry/pmail/utils/context"
)

func ModifyPassword(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var b struct {
		Old      string `json:"old_password"`
		New      string `json:"new_password"`
		Password string `json:"password"`
	}
	if !readRequest(ctx, w, r, &b, false) {
		return
	}
	if b.New == "" {
		b.New = b.Password
	}
	if b.Old == "" || b.New == "" {
		apiError(ctx, w, http.StatusBadRequest, response.ParamsError, "password_fields_required")
		return
	}
	if err := user.ChangePassword(ctx.UserID, ctx.AuthVersion, b.Old, b.New); err != nil {
		serviceError(ctx, w, err)
		return
	}
	if err := session.Instance.Destroy(r.Context()); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, nil)
}
