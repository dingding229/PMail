package controllers

import (
	"errors"
	"net/http"

	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/utils/context"
)

func QueueList(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	rows, err := queue.List(ctx.UserID, ctx.IsAdmin)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	for i := range rows {
		rows[i].LastError = i18n.Delivery(ctx, "queue_detail", rows[i].LastError)
	}
	success(w, rows)
}

func QueueAction(ctx *context.Context, w http.ResponseWriter, r *http.Request, fn func(int, int, bool) error) {
	var body struct {
		ID int `json:"id"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	if body.ID <= 0 {
		apiError(ctx, w, http.StatusBadRequest, 100, "queue_invalid_id")
		return
	}
	if err := fn(body.ID, ctx.UserID, ctx.IsAdmin); err != nil {
		if errors.Is(err, queue.ErrNotFound) {
			apiError(ctx, w, http.StatusNotFound, 100, "queue_not_found")
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
