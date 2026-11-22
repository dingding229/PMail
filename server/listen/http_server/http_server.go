package http_server

import (
	"errors"
	"fmt"
	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/controllers"
	"github.com/Jinnrry/pmail/controllers/email"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
	"time"
)

// Close the bound socket even if Serve has not started yet.
var httpServer *http.Server
var httpSocket net.Listener

func HttpStop() {
	if httpServer != nil {
		httpServer.Close()
		httpSocket.Close()
		httpServer, httpSocket = nil, nil
	}
}

func router(mux *http.ServeMux, version string) {
	mux.HandleFunc("/", staticWeb())
	registerAppRoutes(mux, version)
}

func registerAppRoutes(mux *http.ServeMux, version string) {
	mux.HandleFunc("GET "+acmePrefix+"{token}", controllers.AcmeChallenge)
	mux.HandleFunc("/api/ping", controllers.Ping)
	mux.HandleFunc("GET /api/bootstrap", contextIterceptor(controllers.Bootstrap(version)))
	mux.HandleFunc("POST /api/register", contextIterceptor(controllers.Register))
	mux.HandleFunc("/api/login", contextIterceptor(controllers.Login))
	mux.HandleFunc("/api/logout", contextIterceptor(controllers.Logout))
	mux.HandleFunc("/api/group", contextIterceptor(controllers.GetUserGroup))
	mux.HandleFunc("/api/group/list", contextIterceptor(controllers.GetUserGroupList))
	mux.HandleFunc("/api/group/add", contextIterceptor(controllers.AddGroup))
	mux.HandleFunc("/api/group/del", contextIterceptor(controllers.DelGroup))
	mux.HandleFunc("/api/email/list", contextIterceptor(email.EmailList))
	mux.HandleFunc("/api/email/del", contextIterceptor(email.EmailDelete))
	mux.HandleFunc("POST /api/email/star", contextIterceptor(email.Star))
	mux.HandleFunc("POST /api/email/spam", contextIterceptor(controllers.MarkSpam))
	mux.HandleFunc("GET /api/settings/spam", contextIterceptor(controllers.SpamSettings))
	mux.HandleFunc("PUT /api/settings/spam", contextIterceptor(controllers.SpamSettings))
	for _, method := range []string{"GET", "POST", "DELETE"} {
		mux.HandleFunc(method+" /api/spam/rules", contextIterceptor(controllers.SpamRules))
	}
	mux.HandleFunc("GET /api/admin/inbound", contextIterceptor(controllers.InboundSettings))
	mux.HandleFunc("PUT /api/admin/inbound", contextIterceptor(controllers.InboundSettings))

	mux.HandleFunc("POST /api/email/draft/save", contextIterceptor(email.SaveDraft))
	mux.HandleFunc("POST /api/email/draft/delete", contextIterceptor(email.DeleteDraft))
	mux.HandleFunc("/api/email/read", contextIterceptor(email.MarkRead))
	mux.HandleFunc("/api/email/detail", contextIterceptor(email.EmailDetail))
	mux.HandleFunc("/api/email/move", contextIterceptor(email.Move))
	mux.HandleFunc("/api/email/send", contextIterceptor(email.Send))
	mux.HandleFunc("/api/email/forward", contextIterceptor(email.EmailForward))
	mux.HandleFunc("/api/forward/get", contextIterceptor(controllers.ForwardGet))
	mux.HandleFunc("/api/forward/set", contextIterceptor(controllers.ForwardSet))
	mux.HandleFunc("/api/queue/list", contextIterceptor(controllers.QueueList))
	mux.HandleFunc("/api/queue/retry", contextIterceptor(controllers.QueueRetry))
	mux.HandleFunc("/api/queue/delete", contextIterceptor(controllers.QueueDelete))
	mux.HandleFunc("/api/external/list", contextIterceptor(controllers.ExternalList))
	mux.HandleFunc("/api/external/create", contextIterceptor(controllers.ExternalCreate))
	mux.HandleFunc("/api/external/update", contextIterceptor(controllers.ExternalUpdate))
	mux.HandleFunc("/api/external/delete", contextIterceptor(controllers.ExternalDelete))
	mux.HandleFunc("/api/external/test", contextIterceptor(controllers.ExternalTest))
	mux.HandleFunc("/api/external/sync_now", contextIterceptor(controllers.ExternalSyncNow))
	mux.HandleFunc("/api/external/presets", contextIterceptor(controllers.ExternalPresets))
	mux.HandleFunc("/api/settings/modify_password", contextIterceptor(controllers.ModifyPassword))
	mux.HandleFunc("/api/rule/get", contextIterceptor(controllers.GetRule))
	mux.HandleFunc("/api/rule/add", contextIterceptor(controllers.UpsertRule))
	mux.HandleFunc("/api/rule/update", contextIterceptor(controllers.UpsertRule))
	mux.HandleFunc("/api/rule/del", contextIterceptor(controllers.DelRule))
	mux.HandleFunc("GET /api/attachments/{id}", contextIterceptor(controllers.GetAttachments))
	mux.HandleFunc("GET /api/attachments/preview/{id}", contextIterceptor(controllers.PreviewAttachment))
	mux.HandleFunc("GET /api/attachments/download/{id}", contextIterceptor(controllers.Download))
	legacyInline := contextIterceptor(controllers.GetAttachments)
	mux.HandleFunc("GET /attachments/{emailId}/{cid}", legacyInline)
	mux.HandleFunc("POST /attachments/{emailId}/{cid}", legacyInline)
	mux.HandleFunc("GET /attachments/download/{emailId}/{index}", contextIterceptor(controllers.Download))
	mux.HandleFunc("/api/user/create", contextIterceptor(controllers.CreateUser))
	mux.HandleFunc("/api/user/edit", contextIterceptor(controllers.EditUser))
	mux.HandleFunc("/api/user/info", contextIterceptor(controllers.Info))
	mux.HandleFunc("/api/user/list", contextIterceptor(controllers.UserList))
	mux.HandleFunc("/api/domain/list", contextIterceptor(controllers.DomainList))
	mux.HandleFunc("/api/domain/create", contextIterceptor(controllers.DomainCreate))
	mux.HandleFunc("/api/domain/delete", contextIterceptor(controllers.DomainDelete))
	mux.HandleFunc("/api/domain/set_default", contextIterceptor(controllers.DomainSetDefault))
	mux.HandleFunc("GET /api/domain/dns/zone", contextIterceptor(controllers.DomainZone))
	mux.HandleFunc("/api/domain/dns", contextIterceptor(controllers.DomainDNS))
	mux.HandleFunc("/api/address/list", contextIterceptor(controllers.AddressList))
	mux.HandleFunc("/api/address/create", contextIterceptor(controllers.AddressCreate))
	mux.HandleFunc("/api/address/delete", contextIterceptor(controllers.AddressDelete))
	mux.HandleFunc("/api/address/set_primary", contextIterceptor(controllers.AddressSetPrimary))
	mux.HandleFunc("/api/address/update", contextIterceptor(controllers.AddressUpdate))
	mux.HandleFunc("/api/user/reset_password", contextIterceptor(controllers.UserResetPassword))
	mux.HandleFunc("/api/user/set_quota", contextIterceptor(controllers.UserSetQuota))
	mux.HandleFunc("/api/user/delete", contextIterceptor(controllers.UserDelete))
	mux.HandleFunc("/api/user/me", contextIterceptor(controllers.UserMe))
	mux.HandleFunc("/api/user/change_password", contextIterceptor(controllers.ModifyPassword))
	mux.HandleFunc("/api/token/list", contextIterceptor(controllers.TokenList))
	mux.HandleFunc("/api/token/create", contextIterceptor(controllers.TokenCreate))
	mux.HandleFunc("/api/token/delete", contextIterceptor(controllers.TokenDelete))
	for _, route := range []struct {
		pattern string
		handler controllers.HandlerFunc
	}{
		{"GET /api/admin/telegram", controllers.AdminTelegram}, {"PUT /api/admin/telegram", controllers.AdminTelegram}, {"POST /api/admin/telegram/test", controllers.AdminTelegramTest},
		{"GET /api/settings/telegram", controllers.TelegramSettings}, {"PUT /api/settings/telegram", controllers.TelegramSettings}, {"DELETE /api/settings/telegram", controllers.TelegramSettings},
		{"POST /api/settings/telegram/bind", controllers.TelegramBind}, {"POST /api/settings/telegram/test", controllers.TelegramTest},
		{"GET /api/admin/update", controllers.UpdateInfo}, {"POST /api/admin/update/check", controllers.UpdateCheck}, {"POST /api/admin/update/apply", controllers.UpdateApply}, {"GET /api/admin/update/apply/{job_id}", controllers.UpdateJob},
		{"POST /api/compose/inline-image", controllers.InlineImage}, {"GET /api/compose/inline-image/{cid}", controllers.InlineImage},
		{"GET /api/settings/signatures", controllers.Signatures}, {"POST /api/settings/signatures", controllers.Signatures}, {"PUT /api/settings/signatures/{id}", controllers.Signatures}, {"DELETE /api/settings/signatures/{id}", controllers.Signatures},
	} {
		mux.HandleFunc(route.pattern, contextIterceptor(route.handler))
	}

}

func HttpStart(httpsReady bool, version string) {
	mux := http.NewServeMux()
	port := config.Instance.HttpPort
	if port == 0 {
		port = 80
	}

	var handler http.Handler = mux
	if httpsReady {
		mux.HandleFunc("GET "+acmePrefix+"{token}", controllers.AcmeChallenge)
		mux.HandleFunc("/api/ping", controllers.Ping)
		catalog := http.NewServeMux()
		registerAppRoutes(catalog, version)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if inNamespace(r.URL.Path, "/api") {
				if _, pattern := catalog.Handler(r); pattern == "" {
					apiNotFound(w, r)
					return
				}
			}
			controllers.Interceptor(w, r)
		})
	} else {
		// Serve HTTP directly rather than redirecting to an unavailable HTTPS listener.
		router(mux, version)
		handler = apiSessions(mux)
	}
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      webMiddleware(handler),
		ReadTimeout:  90 * time.Second,
		WriteTimeout: 90 * time.Second,
	}
	socket, err := net.Listen("tcp", server.Addr)
	if err != nil {
		panic(err)
	}
	httpServer, httpSocket = server, socket
	log.Infof("HTTP listening on %s", server.Addr)
	go func() {
		if err := server.Serve(socket); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("HTTP %s: %v", server.Addr, err)
		}
	}()
}
