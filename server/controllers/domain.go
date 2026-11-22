package controllers

import (
	"github.com/Jinnrry/pmail/dto/response"
	"mime"
	"net/http"
	"strconv"

	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func DomainList(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, new(struct{}), true) {
		return
	}
	success(w, mapViews(domain.List(), apiDomain))
}

func DomainCreate(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, &body, false) {
		return
	}
	item, err := domain.Create(body.Name)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, apiDomain(item))
}

func domainAction(ctx *context.Context, w http.ResponseWriter, r *http.Request, fn func(int) error) {
	var body struct {
		ID int `json:"id"`
	}
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, &body, false) {
		return
	}
	if body.ID <= 0 {
		serviceError(ctx, w, domain.ErrInvalid)
		return
	}
	if err := fn(body.ID); err != nil {
		serviceError(ctx, w, err)
		return
	}
	success(w, nil)
}
func DomainDelete(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	domainAction(ctx, w, r, domain.Delete)
}
func DomainSetDefault(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	domainAction(ctx, w, r, domain.SetDefault)
}
func DomainDNS(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      int  `json:"id"`
		Refresh bool `json:"refresh"`
	}
	if !adminOnly(ctx, w) || !readRequest(ctx, w, r, &body, true) {
		return
	}
	if r.Method == http.MethodGet {
		body.ID, _ = strconv.Atoi(r.URL.Query().Get("id"))
	}
	if body.ID <= 0 {
		serviceError(ctx, w, domain.ErrInvalid)
		return
	}
	if r.Method == http.MethodPost {
		w.Header().Set("Cache-Control", "no-store")
		guide, err := domain.Guide(r.Context(), body.ID, body.Refresh, ctx.Lang)
		if err != nil {
			serviceError(ctx, w, err)
			return
		}
		success(w, apiDNSGuide(guide))
		return
	}
	records, err := domain.DNS(r.Context(), body.ID)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	for i := range records {
		if records[i].LookupError != "" {
			log.WithContext(ctx).Debugf("DNS lookup %s %s: %s", records[i].Name, records[i].Type, records[i].LookupError)
			records[i].LookupError = i18n.T(ctx, "dns_lookup_error")
		}
	}
	success(w, records)
}

func DomainZone(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !adminOnly(ctx, w) {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		apiError(ctx, w, http.StatusMethodNotAllowed, response.ParamsError, "method_not_allowed")
		return
	}
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id <= 0 {
		serviceError(ctx, w, domain.ErrInvalid)
		return
	}
	name, text, err := domain.Zone(r.Context(), id, ctx.Lang)
	if err != nil {
		serviceError(ctx, w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".zone"}))
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(text))
	}
}
