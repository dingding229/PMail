package http_server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/controllers"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/token"
	"github.com/Jinnrry/pmail/session"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/id"
	olog "log"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

var httpsServer *http.Server
var httpsSocket net.Listener

type nullWrite struct {
}

func (w *nullWrite) Write(p []byte) (int, error) {
	return len(p), nil
}

// HttpsStart binds the HTTPS port using the already validated certificate.
// Its return value controls whether the HTTP listener should redirect.
func HttpsStart(cert tls.Certificate, version string) bool {
	if config.Instance.HttpsEnabled == 2 {
		return false
	}
	port := config.Instance.HttpsPort
	if port == 0 {
		port = 443
	}
	mux := http.NewServeMux()
	router(mux, version)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      webMiddleware(apiSessions(mux)),
		ReadTimeout:  90 * time.Second,
		WriteTimeout: 90 * time.Second,
		ErrorLog:     olog.New(&nullWrite{}, "", olog.Ldate),
		TLSConfig:    &tls.Config{Certificates: []tls.Certificate{cert}},
	}
	socket, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Errorf("HTTPS unavailable; keeping HTTP enabled: %v", err)
		return false
	}
	httpsServer, httpsSocket = server, socket
	log.Infof("HTTPS listening on %s", server.Addr)
	go func() {
		defer socket.Close()
		if err := server.ServeTLS(socket, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("HTTPS %s: %v", server.Addr, err)
		}
	}()
	return true
}

func HttpsStop() {
	if httpsServer != nil {
		httpsServer.Close()
		httpsSocket.Close()
		httpsServer, httpsSocket = nil, nil
	}
}

// 注入context
func contextIterceptor(h controllers.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}

		publicBootstrap := r.URL.Path == "/api/bootstrap"
		publicRegister := r.URL.Path == "/api/register"
		publicLogin := r.URL.Path == "/api/login" || r.URL.Path == "/api/login/2fa"
		w.Header().Add("Vary", "Accept-Language, X-PMail-Lang")
		if publicBootstrap || publicRegister {
			w.Header().Set("Cache-Control", "no-store")
		}

		ctx := i18n.ForRequest(r)
		ctx.SetValue(context.LogID, id.GenLogID())
		isInbound := controllers.InboundAPIPath(r.URL.Path) || controllers.C9Path(r.URL.Path)
		refuse := func(status, code int, key string) {
			if isInbound {
				if status == http.StatusForbidden {
					key = "inbound_permission_denied"
				}
				controllers.InboundError(ctx, w, status, code, key)
				return
			}
			response.Localized(ctx, code, key).FPrintStatus(w, status)
		}
		if r.URL.Path == "/api/logout" && r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			refuse(http.StatusMethodNotAllowed, response.ParamsError, "method_not_allowed")
			return
		}
		if !(r.Method == http.MethodPost && r.URL.Path == "/api/compose/inline-image") && r.Method != http.MethodOptions && inNamespace(r.URL.Path, "/api") && (r.ContentLength != 0 || (r.Method == http.MethodPost && (publicLogin || publicRegister))) {
			media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || media != "application/json" {
				refuse(http.StatusUnsupportedMediaType, response.ParamsError, "json_media_type_required")
				return
			}
		}
		if !isSafeMethod(r.Method) && (publicRegister || publicLogin) &&
			(r.Header.Get("Origin") != "" || r.Header.Get("Referer") != "") && !sameOrigin(r) {
			noteOriginRejection(r)
			refuse(http.StatusForbidden, response.NoAccessPrivileges, "origin_forbidden")
			return
		}
		if publicRegister {
			h(ctx, w, r)
			return
		}
		if config.IsInit {
			scope := ""
			userInfo, err := session.CurrentUser(ctx)
			if err != nil {
				log.WithContext(ctx).Errorf("Validate session: %v", err)
				if isInbound {
					refuse(500, response.ServerError, "inbound_internal_error")
					return
				}
				response.Localized(ctx, response.ServerError, "internal_error").FPrint(w)
				return
			}
			if userInfo != nil && !isSafeMethod(r.Method) && !sameOrigin(r) {
				noteOriginRejection(r)
				refuse(http.StatusForbidden, response.NoAccessPrivileges, "origin_forbidden")
				return
			}
			if userInfo == nil {
				rawToken := r.Header.Get("Authorization")
				if strings.HasPrefix(strings.ToLower(rawToken), "bearer ") {
					rawToken = strings.TrimSpace(rawToken[7:])
				} else {
					rawToken = r.Header.Get("Token")
				}
				if rawToken != "" {
					if tokenUser, granted, tokenErr := token.Authenticate(rawToken); tokenErr == nil {
						if !token.Allows(granted, r.Method, r.URL.Path) {
							refuse(http.StatusForbidden, response.NoAccessPrivileges, "token_scope_forbidden")
							return
						}
						if granted == token.Legacy {
							w.Header().Set("Deprecation", "@0")
						}
						userInfo, scope = &tokenUser, granted
					}
				}
			}
			if userInfo != nil {
				ctx.UserID = userInfo.ID
				ctx.AuthVersion = userInfo.AuthVersion
				ctx.FactorVersion = userInfo.FactorVersion
				ctx.TokenScope = scope
				ctx.UserName = userInfo.Name
				ctx.UserAccount = userInfo.Account
				ctx.IsAdmin = userInfo.IsAdmin == 1 && (scope == "" || scope == token.Admin || scope == token.Legacy)
				if lang, ok := i18n.Preference(userInfo.Lang); ok && lang != "" {
					ctx.Lang = lang
				}
			} else if !publicLogin && !publicBootstrap {
				if isInbound {
					refuse(401, response.NeedLogin, "inbound_login_required")
					return
				}
				response.Localized(ctx, response.NeedLogin, "login_exp").FPrint(w)
				return
			}
		} else if r.URL.Path != "/api/setup" && !publicBootstrap {
			if isInbound {
				refuse(403, response.NoAccessPrivileges, "inbound_permission_denied")
				return
			}
			response.Localized(ctx, response.NeedSetup, "setup_required").FPrint(w)
			return
		}
		h(ctx, w, r)
	}
}
