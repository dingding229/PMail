package email

import (
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/group"
	"github.com/Jinnrry/pmail/services/mailevents"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
	"net/http"
)

type moveRequest struct {
	GroupId   int    `json:"group_id"`
	GroupName string `json:"group_name"`
	IDs       []int  `json:"ids"`
}

func Move(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var reqData moveRequest
	if !readMailJSON(ctx, w, req, &reqData, false) {
		return
	}

	if len(reqData.IDs) <= 0 {
		response.Localized(ctx, response.ParamsError, "mail_ids_required").FPrint(w)
		return
	}

	if name, ok := models.GroupCodeToName[reqData.GroupId]; ok {
		err := group.Move2DefaultBox(ctx, reqData.IDs, name)
		if err != nil {
			log.WithContext(ctx).Errorf("Move mail: %v", err)
			response.Localized(ctx, response.ServerError, "mail_move_failed").FPrint(w)
			return
		}
	} else if !group.MoveMailToGroup(ctx, reqData.IDs, reqData.GroupId) {
		response.Localized(ctx, response.ServerError, "mail_move_failed").FPrint(w)
		return
	}
	mailevents.Publish(ctx.UserID)
	response.NewSuccessResponse(i18n.T(ctx, "succ")).FPrint(w)

}
