package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/compose"
	"github.com/Jinnrry/pmail/services/telegram"
	"github.com/Jinnrry/pmail/services/update"
	"github.com/Jinnrry/pmail/utils/context"
)

func C9Path(path string) bool {
	for _, prefix := range []string{"/api/admin/telegram", "/api/settings/telegram", "/api/settings/signatures", "/api/admin/update", "/api/compose/inline-image"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
func c9Error(ctx *context.Context, w http.ResponseWriter, err error) {
	for _, pair := range []struct {
		err    error
		status int
	}{
		{telegram.ErrToken, 400}, {telegram.ErrProxy, 400}, {telegram.ErrInvalid, 400}, {telegram.ErrNotConfigured, 409}, {telegram.ErrNotBound, 409}, {telegram.ErrBindExpired, 409}, {telegram.ErrRateLimited, 429}, {telegram.ErrUnavailable, 502},
		{update.ErrRate, 429}, {update.ErrDisabled, 409}, {update.ErrSidecar, 502}, {update.ErrJob, 404}, {update.ErrBusy, 409},
		{compose.ErrType, 415}, {compose.ErrTooLarge, 413}, {compose.ErrQuota, 409}, {compose.ErrRef, 400}, {compose.ErrSignature, 400}, {compose.ErrSignatureLimit, 409}, {compose.ErrNotFound, 404},
	} {
		if errors.Is(err, pair.err) {
			InboundError(ctx, w, pair.status, response.ParamsError, pair.err.Error())
			return
		}
	}
	InboundError(ctx, w, 500, response.ServerError, "internal_error")
}
func c9Response(ctx *context.Context, w http.ResponseWriter, data any, err error) {
	if err != nil {
		c9Error(ctx, w, err)
		return
	}
	inboundOK(w, data)
}
func AdminTelegram(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	if r.Method != http.MethodPut {
		s, err := telegram.AdminGet(r.Context())
		c9Response(ctx, w, s, err)
		return
	}
	var body struct {
		Token *string         `json:"bot_token"`
		Proxy json.RawMessage `json:"proxy"`
	}
	if !inboundBody(ctx, w, r, &body) {
		return
	}
	var proxy *string
	clear := body.Proxy != nil && bytes.Equal(bytes.TrimSpace(body.Proxy), []byte("null"))
	if body.Proxy != nil && !clear {
		var v string
		if json.Unmarshal(body.Proxy, &v) != nil {
			c9Error(ctx, w, telegram.ErrProxy)
			return
		}
		proxy = &v
	}
	s, err := telegram.AdminPut(r.Context(), body.Token, proxy, clear)
	c9Response(ctx, w, s, err)
}
func AdminTelegramTest(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	s, err := telegram.AdminTest(r.Context())
	c9Response(ctx, w, s, err)
}
func TelegramSettings(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	switch r.Method {
	case http.MethodPut:
		var body struct {
			Filters *telegram.Filters `json:"filters"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		if body.Filters == nil {
			c9Error(ctx, w, telegram.ErrInvalid)
			return
		}
		s, err := telegram.SetFilters(r.Context(), ctx.UserID, *body.Filters)
		c9Response(ctx, w, s, err)
	case http.MethodDelete:
		c9Response(ctx, w, nil, telegram.Unbind(r.Context(), ctx.UserID))
	default:
		s, err := telegram.UserGet(r.Context(), ctx.UserID)
		c9Response(ctx, w, s, err)
	}
}
func TelegramBind(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	s, err := telegram.NewCode(r.Context(), ctx.UserID)
	c9Response(ctx, w, s, err)
}
func TelegramTest(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	c9Response(ctx, w, nil, telegram.UserTest(r.Context(), ctx.UserID, i18n.T(ctx, "telegram_test_message")))
}
func UpdateInfo(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	inboundOK(w, localizedUpdate(ctx, update.Current().Info()))
}
func UpdateCheck(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	c := update.Current()
	if err := c.Refresh(r.Context(), true); err != nil {
		c9Error(ctx, w, err)
		return
	}
	inboundOK(w, localizedUpdate(ctx, c.Info()))
}
func localizedUpdate(ctx *context.Context, info update.Info) update.Info {
	if info.Error != nil {
		message := i18n.T(ctx, *info.Error)
		info.Error = &message
	}
	return info
}
func UpdateApply(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	job, err := update.Apply(r.Context())
	if err != nil {
		c9Error(ctx, w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	response.NewSuccessResponse(job).FPrintStatus(w, http.StatusAccepted)
}
func UpdateJob(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	job, err := update.Status(r.Context(), r.PathValue("job_id"))
	if job.Message != nil {
		m := i18n.T(ctx, *job.Message)
		job.Message = &m
	}
	c9Response(ctx, w, job, err)
}
func InlineImage(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, compose.MaxImage+(64<<10))
		reader, err := r.MultipartReader()
		if err != nil {
			c9Error(ctx, w, compose.ErrType)
			return
		}
		f, err := reader.NextPart()
		if err != nil || f.FormName() != "file" || f.FileName() == "" {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				c9Error(ctx, w, compose.ErrTooLarge)
			} else {
				c9Error(ctx, w, compose.ErrType)
			}
			return
		}
		defer f.Close()
		img, err := compose.Upload(r.Context(), ctx.UserID, f.FileName(), f)
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			err = compose.ErrTooLarge
		}
		c9Response(ctx, w, img, err)
		return
	}
	img, err := compose.Preview(r.Context(), ctx.UserID, r.PathValue("cid"), r.Method == http.MethodHead)
	if err != nil {
		c9Error(ctx, w, err)
		return
	}
	w.Header().Set("Content-Type", img.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(img.Size))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "sandbox")
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, bytes.NewReader(img.Content))
	}
}
func Signatures(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	id := 0
	if s := r.PathValue("id"); s != "" {
		var err error
		id, err = strconv.Atoi(s)
		if err != nil || id <= 0 {
			c9Error(ctx, w, compose.ErrNotFound)
			return
		}
	}
	switch r.Method {
	case http.MethodGet:
		list, err := compose.Signatures(r.Context(), ctx.UserID)
		c9Response(ctx, w, list, err)
	case http.MethodDelete:
		c9Response(ctx, w, nil, compose.DeleteSignature(r.Context(), ctx.UserID, id))
	default:
		var body compose.Signature
		// Escape overhead may be larger than the 64 KiB decoded HTML field.
		r.Body = http.MaxBytesReader(w, r.Body, 448<<10)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			c9Error(ctx, w, compose.ErrSignature)
			return
		}
		if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
			c9Error(ctx, w, compose.ErrSignature)
			return
		}
		record, err := compose.SaveSignature(r.Context(), ctx.UserID, id, body)
		c9Response(ctx, w, record, err)
	}
}
