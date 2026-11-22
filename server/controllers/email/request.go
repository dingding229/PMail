package email

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/Jinnrry/pmail/i18n"
	svccompose "github.com/Jinnrry/pmail/services/compose"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	"io"
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/utils/context"
)

// The legacy mail controllers report validation errors in the HTTP 200
// envelope. Keep that convention while avoiding raw decoder errors.
func readMailJSON(ctx *context.Context, w http.ResponseWriter, r *http.Request, dst any, optional bool) bool {
	return readMailJSONLimit(ctx, w, r, dst, optional, 64<<10)
}

func readMailJSONLimit(ctx *context.Context, w http.ResponseWriter, r *http.Request, dst any, optional bool, limit int64) bool {
	if r.Method != http.MethodPost && !(r.Method == http.MethodGet && optional) {
		w.Header().Set("Allow", "POST")
		response.Localized(ctx, response.ParamsError, "method_not_allowed").FPrintStatus(w, http.StatusMethodNotAllowed)
		return false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		mailTooLarge(ctx, w)
		return false
	}
	if err == nil && optional && len(bytes.TrimSpace(data)) == 0 {
		return true
	}
	if err != nil || json.Unmarshal(data, dst) != nil {
		response.Localized(ctx, response.ParamsError, "invalid_json").FPrint(w)
		return false
	}
	return true
}

func mailTooLarge(ctx *context.Context, w http.ResponseWriter) {
	response.Localized(ctx, response.ParamsError, "mail_resource_limit").FPrintStatus(w, http.StatusRequestEntityTooLarge)
}

func readComposeJSON(ctx *context.Context, w http.ResponseWriter, r *http.Request, dst any) bool {
	// Select the compose limit even in direct controller use and tests.
	return readMailJSONLimit(ctx, w, r, dst, false, mailbudget.JSON())
}

func inlineError(ctx *context.Context, w http.ResponseWriter, err error) bool {
	if !errors.Is(err, svccompose.ErrRef) {
		return false
	}
	response.NewErrorResponse(response.ParamsError, i18n.T(ctx, "inline_image_invalid_reference"), map[string]string{"code": "inline_image_invalid_reference"}).FPrintStatus(w, 400)
	return true
}
