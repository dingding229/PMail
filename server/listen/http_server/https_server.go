package http_server

import (
	"crypto/tls"
	"embed"
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
	"net"
	"net/http"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

//go:embed dist/*
var local embed.FS

var httpsServer *http.Server
var httpsSocket net.Listener

type nullWrite struct {
}

func (w *nullWrite) Write(p []byte) (int, error) {
	return len(p), nil
}

// HttpsStart binds the HTTPS port using the already validated certificate.
// Its return value controls whether the HTTP listener should redirect.
func HttpsStart(cert tls.Certificate) bool {
	if config.Instance.HttpsEnabled == 2 {
		return false
	}
	port := config.Instance.HttpsPort
	if port == 0 {
		port = 443
	}
	mux := http.NewServeMux()
	router(mux)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      session.Instance.LoadAndSave(mux),
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

		ctx := &context.Context{}
		ctx.Context = r.Context()
		ctx.SetValue(context.LogID, id.GenLogID())
		lang := r.Header.Get("Lang")
		if lang == "" {
			lang = "en"
		}
		ctx.Lang = lang

		if config.IsInit {
			userInfo, err := session.CurrentUser(ctx)
			if err != nil {
				log.WithContext(ctx).Errorf("Validate session: %v", err)
				response.NewErrorResponse(response.ServerError, i18n.GetText(ctx.Lang, "unknowError"), "").FPrint(w)
				return
			}
			if userInfo != nil {
				ctx.UserID = userInfo.ID
				ctx.AuthVersion = userInfo.AuthVersion
				ctx.UserName = userInfo.Name
				ctx.UserAccount = userInfo.Account
				ctx.IsAdmin = userInfo.IsAdmin == 1
			}

			if ctx.UserID == 0 {
				rawToken := r.Header.Get("Authorization")
				if strings.HasPrefix(strings.ToLower(rawToken), "bearer ") {
					rawToken = strings.TrimSpace(rawToken[7:])
				} else {
					rawToken = r.Header.Get("Token")
				}
				if rawToken != "" {
					if tokenUser, tokenErr := token.Authenticate(rawToken); tokenErr == nil {
						ctx.UserID, ctx.UserName, ctx.UserAccount, ctx.IsAdmin, ctx.AuthVersion = tokenUser.ID, tokenUser.Name, tokenUser.Account, tokenUser.IsAdmin == 1, tokenUser.AuthVersion
					}
				}
			}
			if ctx.UserID == 0 {
				if r.URL.Path != "/api/ping" && r.URL.Path != "/api/login" {
					response.NewErrorResponse(response.NeedLogin, i18n.GetText(ctx.Lang, "login_exp"), "").FPrint(w)
					return
				}
			}
		} else if r.URL.Path != "/api/setup" {
			response.NewErrorResponse(response.NeedSetup, "", "").FPrint(w)
			return
		}
		h(ctx, w, r)
	}
}
