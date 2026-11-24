package http_server

import (
	"github.com/Jinnrry/pmail/controllers"
	"net/http"
	"net/url"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/registration"
	"github.com/Jinnrry/pmail/session"
	log "github.com/sirupsen/logrus"
)

func inNamespace(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func apiOrAttachment(path string) bool {
	return inNamespace(path, "/api") || inNamespace(path, "/attachments")
}

// Do not load or vary by session for static assets, ACME, or health checks.
func apiSessions(next *http.ServeMux) http.Handler {
	withSession := session.SecureCookies(session.Instance.Cookie.Name, session.Instance.LoadAndSave(next))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiOrAttachment(r.URL.Path) && r.URL.Path != "/api/ping" {
			if _, pattern := next.Handler(r); pattern == "/" || pattern == "" {
				next.ServeHTTP(w, r)
				return
			}
			if r.URL.Path != "/api/bootstrap" && r.URL.Path != "/api/register" {
				pending, err := registration.Pending(r.Context())
				if err != nil || pending {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Cache-Control", "no-store")
					ctx := i18n.ForRequest(r)
					if err != nil {
						log.WithContext(ctx).Errorf("Read registration state: %v", err)
						if controllers.InboundAPIPath(r.URL.Path) || controllers.C9Path(r.URL.Path) || controllers.TranslationAPIPath(r.URL.Path) || controllers.AddressApprovalAPIPath(r.URL.Path) {
							controllers.InboundError(ctx, w, http.StatusInternalServerError, response.ServerError, "inbound_internal_error")
						} else {
							response.Localized(ctx, response.ServerError, "internal_error").FPrintStatus(w, http.StatusInternalServerError)
						}
					} else {
						if controllers.InboundAPIPath(r.URL.Path) || controllers.C9Path(r.URL.Path) || controllers.TranslationAPIPath(r.URL.Path) || controllers.AddressApprovalAPIPath(r.URL.Path) {
							controllers.InboundError(ctx, w, http.StatusForbidden, response.NoAccessPrivileges, "inbound_permission_denied")
						} else {
							response.Localized(ctx, response.NoAccessPrivileges, "registration_required").FPrintStatus(w, http.StatusForbidden)
						}
					}
					return
				}
			}
			withSession.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func webMiddleware(next http.Handler) http.Handler {
	origins := map[string]bool{}
	for _, origin := range config.CORSOrigins() {
		origin = strings.TrimSpace(origin)
		u, err := url.Parse(origin)
		if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && origin == u.Scheme+"://"+u.Host {
			origins[origin] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for name, value := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"Referrer-Policy":        "same-origin",
			"X-Frame-Options":        "SAMEORIGIN",
		} {
			if w.Header().Get(name) == "" {
				w.Header().Set(name, value)
			}
		}
		if len(origins) == 0 || !apiOrAttachment(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		preflightMethod := r.Header.Get("Access-Control-Request-Method")
		preflight := r.Method == http.MethodOptions && preflightMethod != ""
		w.Header().Add("Vary", "Origin")
		if preflight {
			w.Header().Add("Vary", "Access-Control-Request-Method, Access-Control-Request-Headers")
			if !origins[origin] || !allowedCORSRequest(preflightMethod, r.Header.Get("Access-Control-Request-Headers")) {
				http.Error(w, i18n.T(i18n.ForRequest(r), "cors_denied"), http.StatusForbidden)
				return
			}
		}
		if origins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
		}
		if preflight {
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language, Lang, Token, X-PMail-Lang, Range")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedCORSRequest(method, headers string) bool {
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodPost && method != http.MethodPut && method != http.MethodDelete && method != http.MethodOptions {
		return false
	}
	for header := range strings.SplitSeq(headers, ",") {
		switch strings.ToLower(strings.TrimSpace(header)) {
		case "", "authorization", "content-type", "accept-language", "lang", "token", "x-pmail-lang", "range":
		default:
			return false
		}
	}
	return true
}
