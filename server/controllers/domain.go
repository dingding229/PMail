package controllers

import (
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
	success(w, domain.List())
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
	success(w, item)
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
		ID int `json:"id"`
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
