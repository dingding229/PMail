package email

import (
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/del_email"
	"github.com/Jinnrry/pmail/services/mailevents"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type emailDeleteRequest struct {
	IDs       []int `json:"ids"`
	ForcedDel bool  `json:"forcedDel"`
}

func EmailDelete(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var reqData emailDeleteRequest
	if !readMailJSON(ctx, w, req, &reqData, false) {
		return
	}

	if len(reqData.IDs) <= 0 {
		response.Localized(ctx, response.ParamsError, "mail_ids_required").FPrint(w)
		return
	}

	if err := del_email.DelEmail(ctx, reqData.IDs, reqData.ForcedDel); err != nil {
		log.WithContext(ctx).Errorf("Delete mail: %v", err)
		response.Localized(ctx, response.ServerError, "internal_error").FPrint(w)
		return
	}
	mailevents.Publish(ctx.UserID)
	response.NewSuccessResponse(i18n.T(ctx, "succ")).FPrint(w)

}
