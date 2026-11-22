package email

import (
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/detail"
	"github.com/Jinnrry/pmail/utils/context"
	"net/http"
)

type markReadRequest struct {
	IDs []int `json:"ids"`
}

func MarkRead(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var reqData markReadRequest
	if !readMailJSON(ctx, w, req, &reqData, false) {
		return
	}

	if len(reqData.IDs) <= 0 {
		response.Localized(ctx, response.ParamsError, "mail_ids_required").FPrint(w)
		return
	}

	for _, id := range reqData.IDs {
		detail.GetEmailDetail(ctx, id, true)
	}

	response.NewSuccessResponse(i18n.T(ctx, "succ")).FPrint(w)

}
