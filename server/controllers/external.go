package controllers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/external"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	log "github.com/sirupsen/logrus"
)

type externalRequest struct {
	models.ExternalAccount
	Password string `json:"password"`
}

func ExternalList(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	rows, err := external.List(ctx.UserID, ctx.IsAdmin)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	for i := range rows {
		localizeExternal(ctx, &rows[i])
	}
	success(w, mapViews(rows, apiExternal))
}

func ExternalCreate(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body externalRequest
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	if body.Password == "" {
		apiError(ctx, w, http.StatusBadRequest, responseParams, "external_password_required")
		return
	}
	row, err := external.Create(ctx.UserID, body.ExternalAccount, body.Password)
	if err != nil {
		externalFailure(ctx, w, err, "external_invalid_config")
		return
	}
	localizeExternal(ctx, &row)
	success(w, apiExternal(row))
}

func ExternalUpdate(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body externalRequest
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	row, err := external.Update(ctx.UserID, ctx.IsAdmin, body.ExternalAccount, body.Password)
	if err != nil {
		externalFailure(ctx, w, err, "external_invalid_config")
		return
	}
	localizeExternal(ctx, &row)
	success(w, apiExternal(row))
}

func ExternalDelete(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	if err := external.Delete(ctx.UserID, ctx.IsAdmin, body.ID); err != nil {
		if errors.Is(err, external.ErrNotFound) {
			apiError(ctx, w, http.StatusNotFound, responseParams, "external_not_found")
		} else {
			serviceError(ctx, w, err)
		}
		return
	}
	success(w, nil)
}

func ExternalTest(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body externalRequest
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	if body.Password == "" {
		apiError(ctx, w, http.StatusBadRequest, responseParams, "external_password_required")
		return
	}
	folders, err := external.TestConnectionContext(r.Context(), body.ExternalAccount, body.Password)
	if err != nil {
		externalFailure(ctx, w, err, "external_connection_failed")
		return
	}
	success(w, map[string]any{"folders": folders})
}

func ExternalSyncNow(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	if err := external.SyncNow(ctx, ctx.UserID, ctx.IsAdmin, body.ID); err != nil {
		if errors.Is(err, external.ErrSyncInProgress) {
			apiError(ctx, w, http.StatusConflict, responseParams, "external_sync_busy")
		} else {
			if errors.Is(err, external.ErrNotFound) {
				apiError(ctx, w, http.StatusNotFound, responseParams, "external_not_found")
			} else {
				serviceError(ctx, w, err)
			}
		}
		return
	}
	success(w, map[string]string{"status": "started", "message": i18n.T(ctx, "external_sync_started")})
}

func ExternalPresets(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	success(w, external.Presets())
}

const responseParams = 100

func localizeExternal(ctx *context.Context, row *models.ExternalAccount) {
	if row.LastError == "" {
		return
	}
	// Persisted diagnostics may contain provider or database details. Do not
	// send them to the browser; expose the localized operational category.
	detail := strings.ToLower(row.LastError)
	key := "external_sync_failed"
	switch {
	case strings.Contains(detail, mailbudget.ErrLimit.Error()):
		key = "mail_resource_limit"
	case strings.Contains(detail, "authentication"), strings.Contains(detail, "invalid credentials"):
		key = "external_invalid_auth"
	case strings.Contains(detail, external.ErrPrivateHost.Error()):
		key = "external_private_host"
	case strings.Contains(detail, "timeout"), strings.Contains(detail, "deadline exceeded"):
		key = "external_timeout"
	case strings.Contains(detail, "quota exceeded"):
		key = "quota_exceeded"
	}
	row.LastError = i18n.T(ctx, key)
}

func externalFailure(ctx *context.Context, w http.ResponseWriter, err error, fallback string) {
	if errors.Is(err, mailbudget.ErrLimit) {
		apiError(ctx, w, http.StatusRequestEntityTooLarge, responseParams, "mail_resource_limit")
		return
	}
	if db.IsUniqueViolation(err) {
		serviceError(ctx, w, err)
		return
	}
	if errors.Is(err, external.ErrEndpointOwner) {
		apiError(ctx, w, http.StatusForbidden, 405, "external_endpoint_owner")
		return
	}
	if errors.Is(err, external.ErrCredentialRace) {
		apiError(ctx, w, http.StatusConflict, responseParams, "external_credential_conflict")
		return
	}
	key := fallback
	switch {
	case errors.Is(err, external.ErrNotFound):
		key = "external_not_found"
	case errors.Is(err, external.ErrPasswordRequired):
		key = "external_endpoint_password_required"
	case errors.Is(err, external.ErrControlCharacters):
		key = "external_control_characters"
	case errors.Is(err, external.ErrHostRequired):
		key = "external_host_required"
	case errors.Is(err, external.ErrPrivateHost):
		key = "external_private_host"
	case errors.Is(err, external.ErrProtocol):
		key = "external_invalid_protocol"
	case errors.Is(err, external.ErrTLSMode):
		key = "external_invalid_tls"
	case errors.Is(err, external.ErrPort):
		key = "external_invalid_port"
	case errors.Is(err, external.ErrAuth):
		key = "external_invalid_auth"
	}
	log.WithContext(ctx).Warnf("External mailbox operation: %v", err)
	apiError(ctx, w, http.StatusBadRequest, responseParams, key)
}
