package response

import (
	"encoding/json"
	"net/http"

	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/utils/context"
)

const (
	NeedSetup          = 402
	NeedLogin          = 403
	RouteNotFound      = 404
	NoAccessPrivileges = 405
	ParamsError        = 100
	ServerError        = 500
)

type Response struct {
	ErrorNo  int    `json:"errorNo"`
	ErrorMsg string `json:"errorMsg"`
	Data     any    `json:"data"`
}

func (p *Response) FPrint(w http.ResponseWriter) { p.FPrintStatus(w, http.StatusOK) }
func (p *Response) FPrintStatus(w http.ResponseWriter, status int) {
	bytesData, _ := json.Marshal(p)
	w.WriteHeader(status)
	_, _ = w.Write(bytesData)
}

func NewSuccessResponse(data any) *Response {
	return &Response{
		Data: data,
	}
}

func NewErrorResponse(errorNo int, errorMsg string, data any) *Response {
	return &Response{
		ErrorNo:  errorNo,
		ErrorMsg: errorMsg,
		Data:     data,
	}
}

func Localized(ctx *context.Context, errorNo int, key string, args ...any) *Response {
	return NewErrorResponse(errorNo, i18n.T(ctx, key, args...), "")
}
