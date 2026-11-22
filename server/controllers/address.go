package controllers

import (
	"net/http"
	"strconv"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/user"
	"github.com/Jinnrry/pmail/utils/context"
)

type addressRequest struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	DomainID  int    `json:"domain_id"`
	LocalPart string `json:"local_part"`
	CanSend   *bool  `json:"can_send"`
}

func addressTarget(ctx *context.Context, w http.ResponseWriter, id int) (int, bool) {
	if id == 0 {
		id = ctx.UserID
	}
	if id != ctx.UserID && !ctx.IsAdmin {
		adminOnly(ctx, w)
		return 0, false
	}
	if id <= 0 {
		serviceError(ctx, w, address.ErrInvalid)
		return 0, false
	}
	return id, true
}

func AddressList(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body addressRequest
	if !readRequest(ctx, w, r, &body, true) {
		return
	}
	if r.Method == http.MethodGet && r.URL.Query().Has("user_id") {
		var err error
		body.UserID, err = strconv.Atoi(r.URL.Query().Get("user_id"))
		if err != nil {
			serviceError(ctx, w, address.ErrInvalid)
			return
		}
	}
	id, ok := addressTarget(ctx, w, body.UserID)
	if !ok {
		return
	}
	if _, err := user.Get(id); err != nil {
		serviceError(ctx, w, err)
		return
	}
	rows, err := address.List(id)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, rows)
}

func AddressCreate(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !ctx.IsAdmin && !config.Instance.AllowUserAlias {
		adminOnly(ctx, w)
		return
	}
	var body addressRequest
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	id, ok := addressTarget(ctx, w, body.UserID)
	if !ok {
		return
	}
	if body.DomainID <= 0 {
		serviceError(ctx, w, address.ErrInvalid)
		return
	}
	canSend := body.CanSend == nil || *body.CanSend
	row, err := address.Create(id, body.DomainID, body.LocalPart, canSend)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, row)
}

func addressAction(ctx *context.Context, w http.ResponseWriter, r *http.Request, fn func(addressRequest) error) {
	var body addressRequest
	if !readRequest(ctx, w, r, &body, false) {
		return
	}
	if body.ID <= 0 {
		serviceError(ctx, w, address.ErrInvalid)
		return
	}
	if err := fn(body); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, nil)
}
func AddressDelete(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	addressAction(ctx, w, r, func(b addressRequest) error { return address.Delete(b.ID, ctx.UserID, ctx.IsAdmin) })
}
func AddressSetPrimary(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	addressAction(ctx, w, r, func(b addressRequest) error { return address.SetPrimary(b.ID, ctx.UserID, ctx.IsAdmin) })
}
func AddressUpdate(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	addressAction(ctx, w, r, func(b addressRequest) error {
		if b.CanSend == nil {
			return address.ErrInvalid
		}
		return address.SetCanSend(b.ID, ctx.UserID, ctx.IsAdmin, *b.CanSend)
	})
}
