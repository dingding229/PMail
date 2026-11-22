package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/context"
)

const bootstrapVersion = "4.0.0-dev"

type bootstrapFeatures struct {
	RegisterKey bool `json:"register_key"`
}

type bootstrapResponse struct {
	Version      string            `json:"version"`
	NeedRegister bool              `json:"need_register"`
	LoggedIn     bool              `json:"logged_in"`
	DefaultLang  string            `json:"default_lang"`
	WebDomain    string            `json:"web_domain"`
	Domains      []string          `json:"domains"`
	Features     bootstrapFeatures `json:"features"`
}

func Bootstrap(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	result := bootstrapResponse{
		Version:      bootstrapVersion,
		NeedRegister: true, // The legacy setup listener may not have a database yet.
		LoggedIn:     ctx.UserID > 0,
		DefaultLang:  config.DefaultLang(),
		WebDomain:    config.Instance.WebDomain,
		Domains:      []string{},
		Features:     bootstrapFeatures{RegisterKey: true},
	}
	if db.Instance != nil {
		hasUser, err := db.Instance.Context(r.Context()).Exist(&models.User{})
		if err != nil {
			serviceError(ctx, w, err)
			return
		}
		result.NeedRegister = !hasUser
	} else if config.IsInit {
		apiError(w, http.StatusServiceUnavailable, response.ServerError, "Database unavailable")
		return
	}
	if result.LoggedIn {
		result.Domains = domain.Names()
	}
	_ = json.NewEncoder(w).Encode(result)
}
