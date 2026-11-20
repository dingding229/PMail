package controllers

import (
	"net/http"

	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/utils/context"
)

func ForwardGet(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		apiError(w, http.StatusMethodNotAllowed, 100, "Method not allowed")
		return
	}
	setting, targets, err := forward.Get(ctx.UserID)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, map[string]any{"setting": setting, "targets": targets})
}

func ForwardSet(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled  bool     `json:"enabled"`
		Targets  []string `json:"targets"`
		KeepCopy *bool    `json:"keep_copy"`
	}
	if !readRequest(w, r, &body, false) {
		return
	}
	keep := true
	if body.KeepCopy != nil {
		keep = *body.KeepCopy
	}
	setting, err := forward.Set(ctx.UserID, body.Enabled, body.Targets, keep)
	if err != nil {
		apiError(w, http.StatusBadRequest, 100, err.Error())
		return
	}
	success(w, setting)
}
