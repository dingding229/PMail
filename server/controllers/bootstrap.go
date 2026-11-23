package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/registration"
	"github.com/Jinnrry/pmail/utils/context"
)

type bootstrapFeatures struct {
	RegisterKey bool `json:"register_key"`
}

type bootstrapResponse struct {
	Version       string            `json:"version"`
	NeedRegister  bool              `json:"need_register"`
	LoggedIn      bool              `json:"logged_in"`
	DefaultLang   string            `json:"default_lang"`
	WebDomain     string            `json:"web_domain"`
	PrimaryDomain string            `json:"primary_domain"`
	Domains       []string          `json:"domains"`
	Features      bootstrapFeatures `json:"features"`
}

func Bootstrap(version string) HandlerFunc {
	if version == "" {
		version = "dev"
	}
	return func(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
		pending, err := registration.Pending(r.Context())
		if err != nil {
			serviceError(ctx, w, err)
			return
		}
		result := bootstrapResponse{
			Version:       version,
			NeedRegister:  pending,
			LoggedIn:      ctx.UserID > 0,
			DefaultLang:   i18n.Default(),
			WebDomain:     config.Instance.WebDomain,
			PrimaryDomain: primaryDomain(),
			Domains:       []string{},
			Features:      bootstrapFeatures{RegisterKey: true},
		}
		if result.LoggedIn {
			result.Domains = domain.Names()
		}
		_ = json.NewEncoder(w).Encode(result)
	}
}

// primaryDomain is the current default mail domain (admins can change it),
// falling back to the configured PMAIL_DOMAIN before domains are loaded.
func primaryDomain() string {
	if name := domain.DefaultName(); name != "" {
		return name
	}
	return config.Instance.Domain
}
