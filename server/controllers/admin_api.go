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

func apiError(ctx *context.Context, w http.ResponseWriter, status, code int, key string) {
	response.Localized(ctx, code, key).FPrintStatus(w, status)
}

func adminOnly(ctx *context.Context, w http.ResponseWriter) bool {
	if ctx.IsAdmin {
		return true
	}
	apiError(ctx, w, http.StatusForbidden, response.NoAccessPrivileges, "access_denied")
	return false
}

func readRequest(ctx *context.Context, w http.ResponseWriter, r *http.Request, dst any, readOnly bool) bool {
	if r.Method == http.MethodGet && readOnly {
		return true
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		apiError(ctx, w, http.StatusMethodNotAllowed, response.ParamsError, "method_not_allowed")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(dst); err != nil {
		if readOnly && errors.Is(err, io.EOF) {
			return true
		}
		apiError(ctx, w, http.StatusBadRequest, response.ParamsError, "invalid_json")
		return false
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		apiError(ctx, w, http.StatusBadRequest, response.ParamsError, "invalid_json")
		return false
	}
	return true
}

var serviceErrors = []struct {
	err    error
	status int
	code   int
	key    string
}{
	{domain.ErrInvalid, 400, response.ParamsError, "invalid_domain"},
	{address.ErrInvalid, 400, response.ParamsError, "invalid_address"},
	{user.ErrInvalid, 400, response.ParamsError, "invalid_user"},
	{user.ErrInvalidQuota, 400, response.ParamsError, "quota_invalid"},
	{user.ErrBadPassword, 400, response.ParamsError, "bad_current_password"},
	{password.ErrEmpty, 400, response.ParamsError, "password_empty"},
	{domain.ErrNotFound, 404, response.ParamsError, "domain_not_found"},
	{address.ErrNotFound, 404, response.ParamsError, "address_not_found"},
	{user.ErrNotFound, 404, response.ParamsError, "user_not_found"},
	{address.ErrForbidden, 403, response.NoAccessPrivileges, "address_forbidden"},
	{user.ErrRevoked, 401, response.NeedLogin, "login_exp"},
	{domain.ErrDefault, 409, response.ParamsError, "domain_default"},
	{domain.ErrInUse, 409, response.ParamsError, "domain_in_use"},
	{domain.ErrExists, 409, response.ParamsError, "domain_exists"},
	{address.ErrExists, 409, response.ParamsError, "address_exists"},
	{address.ErrPrimary, 409, response.ParamsError, "address_primary"},
	{user.ErrExists, 409, response.ParamsError, "user_exists"},
	{user.ErrLastAdmin, 409, response.ParamsError, "user_last_admin"},
}

func serviceError(ctx *context.Context, w http.ResponseWriter, err error) {
	for _, entry := range serviceErrors {
		if errors.Is(err, entry.err) {
			apiError(ctx, w, entry.status, entry.code, entry.key)
			return
		}
	}
	log.WithContext(ctx).Errorf("API operation failed: %v", err)
	apiError(ctx, w, http.StatusInternalServerError, response.ServerError, "internal_error")
}

func success(w http.ResponseWriter, data any) { response.NewSuccessResponse(data).FPrint(w) }
