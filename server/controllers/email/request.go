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
