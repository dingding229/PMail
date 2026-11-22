package email

import (
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/detail"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func MarkRead(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var body struct {
		IDs    []int `json:"ids"`
		IsRead *bool `json:"is_read"`
	}
	if !readMailJSON(ctx, w, req, &body, false) {
		return
	}
	read := body.IsRead == nil || *body.IsRead
	updateFlags(ctx, w, body.IDs, &read, nil)
}

func Star(ctx *context.Context, w http.ResponseWriter, req *http.Request) {
	var body struct {
		IDs     []int `json:"ids"`
		Starred *bool `json:"starred"`
	}
	if !readMailJSON(ctx, w, req, &body, false) {
		return
	}
	if body.Starred == nil {
		response.Localized(ctx, response.ParamsError, "mail_star_required").FPrint(w)
		return
	}
	updateFlags(ctx, w, body.IDs, nil, body.Starred)
}

func validIDs(ids []int) bool {
	if len(ids) == 0 || len(ids) > 500 {
		return false
	}
	for _, id := range ids {
		if id <= 0 {
			return false
		}
	}
	return true
}

func updateFlags(ctx *context.Context, w http.ResponseWriter, ids []int, read, starred *bool) {
	if !validIDs(ids) {
		response.Localized(ctx, response.ParamsError, "mail_ids_required").FPrint(w)
		return
	}
	if err := detail.SetFlags(ctx, ids, read, starred); err != nil {
		log.WithContext(ctx).Errorf("Set mail flags: %v", err)
		response.Localized(ctx, response.ServerError, "internal_error").FPrint(w)
		return
	}
	response.NewSuccessResponse(i18n.T(ctx, "succ")).FPrint(w)
}
