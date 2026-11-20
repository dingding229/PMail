package controllers

import (
	"errors"
	"net/http"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/external"
	"github.com/Jinnrry/pmail/utils/context"
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
	success(w, rows)
}

func ExternalCreate(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body externalRequest
	if !readRequest(w, r, &body, false) {
		return
	}
	if body.Password == "" {
		apiError(w, http.StatusBadRequest, responseParams, "password is required")
		return
	}
	row, err := external.Create(ctx.UserID, body.ExternalAccount, body.Password)
	if err != nil {
		apiError(w, http.StatusBadRequest, responseParams, err.Error())
		return
	}
	success(w, row)
}

func ExternalUpdate(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body externalRequest
	if !readRequest(w, r, &body, false) {
		return
	}
	row, err := external.Update(ctx.UserID, ctx.IsAdmin, body.ExternalAccount, body.Password)
	if err != nil {
		apiError(w, http.StatusBadRequest, responseParams, err.Error())
		return
	}
	success(w, row)
}

func ExternalDelete(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if !readRequest(w, r, &body, false) {
		return
	}
	if err := external.Delete(ctx.UserID, ctx.IsAdmin, body.ID); err != nil {
		apiError(w, http.StatusNotFound, responseParams, err.Error())
		return
	}
	success(w, nil)
}

func ExternalTest(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body externalRequest
	if !readRequest(w, r, &body, false) {
		return
	}
	folders, err := external.TestConnection(body.ExternalAccount, body.Password)
	if err != nil {
		apiError(w, http.StatusBadRequest, responseParams, err.Error())
		return
	}
	success(w, map[string]any{"folders": folders})
}

func ExternalSyncNow(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID int `json:"id"`
	}
	if !readRequest(w, r, &body, false) {
		return
	}
	if err := external.SyncNow(ctx, ctx.UserID, ctx.IsAdmin, body.ID); err != nil {
		if errors.Is(err, external.ErrSyncInProgress) {
			apiError(w, http.StatusConflict, responseParams, "external account is already syncing")
		} else {
			apiError(w, http.StatusNotFound, responseParams, err.Error())
		}
		return
	}
	success(w, map[string]string{"status": "started", "message": "external sync started"})
}

func ExternalPresets(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	success(w, external.Presets())
}

const responseParams = 100
