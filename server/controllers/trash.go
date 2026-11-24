package controllers

import (
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/del_email"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func TrashSettings(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Days *int `json:"trash_retention_days"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		if body.Days == nil || *body.Days < 0 || *body.Days > del_email.MaxRetentionDays {
			InboundError(ctx, w, http.StatusBadRequest, response.ParamsError, "trash_retention_invalid")
			return
		}
		if err := del_email.SetRetention(r.Context(), *body.Days); err != nil {
			inboundErr(ctx, w, err)
			return
		}
		log.WithContext(ctx).Infof("audit_event=trash_retention_changed actor_id=%d trash_retention_days=%d", ctx.UserID, *body.Days)
		inboundOK(w, map[string]int{"trash_retention_days": *body.Days})
		return
	}
	days, err := del_email.Retention(r.Context())
	if err != nil {
		inboundErr(ctx, w, err)
		return
	}
	inboundOK(w, map[string]int{"trash_retention_days": days})
}
