package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/domain"
)

func TestBootstrapPrimaryDomain(t *testing.T) {
	actor := c3Fixture(t)
	cfg := config.Instance
	cfg.Domains = []string{"secondary.example.test"}
	if err := domain.Initialize(cfg); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		userID      int
		wantDomains []string
	}{
		{"anonymous", 0, []string{}},
		{"authenticated", actor.UserID, []string{cfg.Domain, cfg.Domains[0]}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := *actor
			ctx.UserID = tc.userID
			w := httptest.NewRecorder()
			Bootstrap("dev")(&ctx, w, httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil))
			var got struct {
				LoggedIn      bool     `json:"logged_in"`
				PrimaryDomain string   `json:"primary_domain"`
				WebDomain     string   `json:"web_domain"`
				Domains       []string `json:"domains"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
				t.Fatalf("bootstrap HTTP %d: %s (%v)", w.Code, w.Body, err)
			}
			if got.PrimaryDomain != cfg.Domain || got.WebDomain != cfg.WebDomain || got.LoggedIn != (tc.userID != 0) ||
				got.Domains == nil || !slices.Equal(got.Domains, tc.wantDomains) {
				t.Fatalf("unexpected bootstrap: %s; want primary=%q web=%q domains=%v", w.Body, cfg.Domain, cfg.WebDomain, tc.wantDomains)
			}
		})
	}
}
