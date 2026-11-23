package email

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/utils/context"
)

func addressFilter(ctx *context.Context, w http.ResponseWriter, query string, body json.RawMessage) (mailaddress.Filter, bool) {
	value, err := mailaddress.Value(query, body)
	var filter mailaddress.Filter
	if err == nil {
		filter, err = mailaddress.Validate(ctx.UserID, value)
	}
	if err == nil {
		return filter, true
	}
	switch {
	case errors.Is(err, mailaddress.ErrForbidden):
		response.Localized(ctx, response.NoAccessPrivileges, "mail_address_forbidden").FPrintStatus(w, http.StatusForbidden)
	case errors.Is(err, mailaddress.ErrInvalid):
		response.Localized(ctx, response.ParamsError, "mail_address_invalid").FPrintStatus(w, http.StatusBadRequest)
	default:
		response.Localized(ctx, response.ServerError, "internal_error").FPrintStatus(w, http.StatusInternalServerError)
	}
	return filter, false
}

func AddressCounts(ctx *context.Context, w http.ResponseWriter, _ *http.Request) {
	result, err := mailaddress.Unread(ctx.UserID)
	if err != nil {
		response.Localized(ctx, response.ServerError, "internal_error").FPrintStatus(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	response.NewSuccessResponse(result).FPrint(w)
}

func preferredFrom(owner, emailID int) string {
	matched, err := mailaddress.ForMessage(owner, emailID)
	if err == nil {
		for _, a := range matched {
			if a.CanSend {
				return a.Address
			}
		}
	}
	return "" // default to primary via the existing sender validator
}
