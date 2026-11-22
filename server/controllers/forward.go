package controllers

import (
	"errors"
	log "github.com/sirupsen/logrus"
	"net/http"

	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/utils/context"
)

func ForwardGet(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		apiError(ctx, w, http.StatusMethodNotAllowed, 100, "method_not_allowed")
		return
	}
	setting, targets, err := forward.Get(ctx.UserID)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, map[string]any{"setting": apiForward(setting), "targets": targets})
}

func ForwardSet(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled  bool     `json:"enabled"`
		Targets  []string `json:"targets"`
		KeepCopy *bool    `json:"keep_copy"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	keep := true
	if body.KeepCopy != nil {
		keep = *body.KeepCopy
	}
	setting, err := forward.Set(ctx.UserID, body.Enabled, body.Targets, keep)
	if err != nil {
		log.WithContext(ctx).Warnf("Set forwarding settings: %v", err)
		apiError(ctx, w, http.StatusBadRequest, 100, forwardErrorKey(err))
		return
	}
	success(w, apiForward(setting))
}

func forwardErrorKey(err error) string {
	switch {
	case errors.Is(err, forward.ErrInvalidTarget):
		return "forward_invalid_target"
	case errors.Is(err, forward.ErrSelfTarget):
		return "forward_self_target"
	case errors.Is(err, forward.ErrTooManyTargets):
		return "forward_too_many_targets"
	default:
		return "forward_settings_failed"
	}
}
