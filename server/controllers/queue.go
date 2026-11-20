package controllers

import (
	"errors"
	"net/http"

	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/utils/context"
)

func QueueList(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	rows, err := queue.List(ctx.UserID, ctx.IsAdmin)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, rows)
}

func QueueAction(ctx *context.Context, w http.ResponseWriter, r *http.Request, fn func(int, int, bool) error) {
	var body struct {
		ID int `json:"id"`
	}
	if !readRequest(w, r, &body, false) {
		return
	}
	if body.ID <= 0 {
		apiError(w, http.StatusBadRequest, 100, "invalid queue id")
		return
	}
	if err := fn(body.ID, ctx.UserID, ctx.IsAdmin); err != nil {
		if errors.Is(err, queue.ErrNotFound) {
			apiError(w, http.StatusNotFound, 100, err.Error())
		} else {
			serviceError(ctx, w, err)
		}
		return
	}
	success(w, nil)
}

func QueueRetry(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	QueueAction(ctx, w, r, queue.Retry)
}

func QueueDelete(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	QueueAction(ctx, w, r, queue.Delete)
}
