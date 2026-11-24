package email

import (
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/del_email"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func RestoreTrash(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int `json:"ids"`
	}
	if !readMailJSON(ctx, w, r, &body, false) {
		return
	}
	if len(body.IDs) == 0 {
		response.Localized(ctx, response.ParamsError, "mail_ids_required").FPrintStatus(w, http.StatusBadRequest)
		return
	}
	n, err := del_email.Restore(ctx, body.IDs)
	if err != nil {
		log.WithContext(ctx).Errorf("Restore Trash: %v", err)
		response.Localized(ctx, response.ServerError, "mail_trash_restore_failed").FPrintStatus(w, http.StatusInternalServerError)
		return
	}
	response.NewSuccessResponse(map[string]int64{"restored": n}).FPrint(w)
}

func EmptyTrash(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	n, err := del_email.Empty(r.Context(), ctx.UserID)
	result := map[string]int64{"deleted": n}
	if err != nil {
		log.WithContext(ctx).Errorf("Empty Trash: %v", err)
		response.NewErrorResponse(response.ServerError, i18n.T(ctx, "mail_trash_empty_failed"), result).FPrintStatus(w, http.StatusInternalServerError)
		return
	}
	response.NewSuccessResponse(result).FPrint(w)
}
