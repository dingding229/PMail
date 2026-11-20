package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/user"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/password"
	log "github.com/sirupsen/logrus"
)

func apiError(w http.ResponseWriter, status, code int, msg string) {
	w.WriteHeader(status)
	response.NewErrorResponse(code, msg, "").FPrint(w)
}

func adminOnly(ctx *context.Context, w http.ResponseWriter) bool {
	if ctx.IsAdmin {
		return true
	}
	apiError(w, http.StatusForbidden, response.NoAccessPrivileges, "No Access Privileges")
	return false
}

func readRequest(w http.ResponseWriter, r *http.Request, dst any, readOnly bool) bool {
	if r.Method == http.MethodGet && readOnly {
		return true
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		apiError(w, http.StatusMethodNotAllowed, response.ParamsError, "Method not allowed")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(dst); err != nil {
		if readOnly && errors.Is(err, io.EOF) {
			return true
		}
		apiError(w, http.StatusBadRequest, response.ParamsError, "Invalid JSON request")
		return false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		apiError(w, http.StatusBadRequest, response.ParamsError, "Invalid JSON request")
		return false
	}
	return true
}

func serviceError(ctx *context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, address.ErrInvalid), errors.Is(err, user.ErrInvalid), errors.Is(err, password.ErrEmpty):
		apiError(w, http.StatusBadRequest, response.ParamsError, err.Error())
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, address.ErrNotFound), errors.Is(err, user.ErrNotFound):
		apiError(w, http.StatusNotFound, response.ParamsError, err.Error())
	case errors.Is(err, address.ErrForbidden):
		apiError(w, http.StatusForbidden, response.NoAccessPrivileges, err.Error())
	case errors.Is(err, user.ErrRevoked):
		apiError(w, http.StatusUnauthorized, response.NeedLogin, err.Error())
	case errors.Is(err, domain.ErrDefault), errors.Is(err, domain.ErrInUse), errors.Is(err, domain.ErrExists), errors.Is(err, address.ErrExists), errors.Is(err, address.ErrPrimary), errors.Is(err, user.ErrExists), errors.Is(err, user.ErrLastAdmin):
		apiError(w, http.StatusConflict, response.ParamsError, err.Error())
	default:
		log.WithContext(ctx).Errorf("API operation failed: %v", err)
		apiError(w, http.StatusInternalServerError, response.ServerError, "Internal server error")
	}
}

func success(w http.ResponseWriter, data any) { response.NewSuccessResponse(data).FPrint(w) }
