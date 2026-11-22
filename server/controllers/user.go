package controllers

import (
	"net/http"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/user"
	"github.com/Jinnrry/pmail/utils/context"
)

type userRequest struct {
	ID          int    `json:"id"`
	Account     string `json:"account"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Disabled    *int   `json:"disabled"`
	QuotaMB     *int64 `json:"quota_mb"`
}

func (b userRequest) Name() string {
	if b.DisplayName != "" {
		return b.DisplayName
	}
	return b.Username
}

func CreateUser(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var b userRequest
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, &b, false) {
		return
	}
	var disabled int
	var mb int64
	if b.Disabled != nil {
		disabled = *b.Disabled
	}
	if b.QuotaMB != nil {
		mb = *b.QuotaMB
	}
	u, err := user.Create(b.Account, b.Name(), b.Password, disabled, mb)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, apiUser(u))
}

func EditUser(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var b userRequest
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, &b, false) {
		return
	}
	u, err := user.Edit(user.Patch{ID: b.ID, Account: b.Account, Name: b.Name(), Password: b.Password, Disabled: b.Disabled, QuotaMB: b.QuotaMB})
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, apiUser(u))
}

func UserList(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var b struct {
		Page int `json:"current_page"`
		Size int `json:"page_size"`
	}
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, &b, true) {
		return
	}
	if b.Page == 0 {
		b.Page = 1
	}
	if b.Size == 0 {
		b.Size = 15
	}
	if b.Page < 1 || b.Page > 1_000_000 || b.Size < 1 || b.Size > 100 {
		serviceError(ctx, w, user.ErrInvalid)
		return
	}
	var list []models.User
	n, err := db.Instance.Asc("id").Limit(b.Size, (b.Page-1)*b.Size).FindAndCount(&list)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	items, err := user.Views(list)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, map[string]any{"current_page": b.Page, "total_page": (n + int64(b.Size) - 1) / int64(b.Size), "list": mapViews(items, apiCurrentUser)})
}

func Info(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	success(w, map[string]any{"account": ctx.UserAccount, "name": ctx.UserName, "is_admin": ctx.IsAdmin, "domains": domain.Names()})
}

func UserMe(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var b struct {
		DisplayName *string `json:"display_name"`
		Lang        *string `json:"lang"`
	}
	if !readRequest(ctx, w, r, &b, true) {
		return
	}
	patch := user.Patch{ID: ctx.UserID}
	if b.DisplayName != nil {
		if !user.ValidName(*b.DisplayName) {
			apiError(ctx, w, http.StatusBadRequest, response.ParamsError, "invalid_display_name")
			return
		}
		patch.Name = *b.DisplayName
	}
	if b.Lang != nil {
		lang, ok := i18n.Preference(*b.Lang)
		if !ok {
			apiError(ctx, w, http.StatusBadRequest, response.ParamsError, "invalid_language")
			return
		}
		patch.Lang = &lang
	}
	var u models.User
	var err error
	if b.DisplayName != nil || b.Lang != nil {
		u, err = user.Edit(patch)
	} else {
		u, err = user.Get(ctx.UserID)
	}
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	views, err := user.Views([]models.User{u})
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, apiCurrentUser(views[0]))
}

func UserResetPassword(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var b userRequest
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, &b, false) {
		return
	}
	if b.ID <= 0 || b.Password == "" {
		serviceError(ctx, w, user.ErrInvalid)
		return
	}
	if _, err := user.Edit(user.Patch{ID: b.ID, Password: b.Password}); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, nil)
}
func UserSetQuota(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var b userRequest
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, &b, false) {
		return
	}
	if b.ID <= 0 || b.QuotaMB == nil {
		serviceError(ctx, w, user.ErrInvalid)
		return
	}
	if _, err := user.Edit(user.Patch{ID: b.ID, QuotaMB: b.QuotaMB}); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, nil)
}
func UserDelete(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var b userRequest
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, &b, false) {
		return
	}
	if b.ID <= 0 {
		serviceError(ctx, w, user.ErrInvalid)
		return
	}
	disabled := 1
	if _, err := user.Edit(user.Patch{ID: b.ID, Disabled: &disabled}); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, nil)
}
