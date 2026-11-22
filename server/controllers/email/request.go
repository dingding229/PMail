package email

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/utils/context"
)

// The legacy mail controllers report validation errors in the HTTP 200
// envelope. Keep that convention while avoiding raw decoder errors.
func readMailJSON(ctx *context.Context, w http.ResponseWriter, r *http.Request, dst any, optional bool) bool {
	if r.Method != http.MethodPost && !(r.Method == http.MethodGet && optional) {
		w.Header().Set("Allow", "POST")
		response.Localized(ctx, response.ParamsError, "method_not_allowed").FPrintStatus(w, http.StatusMethodNotAllowed)
		return false
	}
	data, err := io.ReadAll(r.Body)
	if err == nil && optional && len(bytes.TrimSpace(data)) == 0 {
		return true
	}
	if err != nil || json.Unmarshal(data, dst) != nil {
		response.Localized(ctx, response.ParamsError, "invalid_json").FPrint(w)
		return false
	}
	return true
}
