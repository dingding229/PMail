package http_server

import (
	"errors"
	"fmt"
	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/controllers"
	"github.com/Jinnrry/pmail/controllers/email"
	"github.com/Jinnrry/pmail/session"
	log "github.com/sirupsen/logrus"
	"io/fs"
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

func router(mux *http.ServeMux) {
	fe, err := fs.Sub(local, "dist")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(fe)))
	// 挑战请求类似这样 /.well-known/acme-challenge/QPyMAyaWw9s5JvV1oruyqWHG7OqkHMJEHPoUz2046KM
	mux.HandleFunc("/.well-known/", controllers.AcmeChallenge)
	mux.HandleFunc("/api/ping", controllers.Ping)
	mux.HandleFunc("/api/login", contextIterceptor(controllers.Login))
	mux.HandleFunc("/api/logout", contextIterceptor(controllers.Logout))
	mux.HandleFunc("/api/group", contextIterceptor(controllers.GetUserGroup))
	mux.HandleFunc("/api/group/list", contextIterceptor(controllers.GetUserGroupList))
	mux.HandleFunc("/api/group/add", contextIterceptor(controllers.AddGroup))
	mux.HandleFunc("/api/group/del", contextIterceptor(controllers.DelGroup))
	mux.HandleFunc("/api/email/list", contextIterceptor(email.EmailList))
	mux.HandleFunc("/api/email/del", contextIterceptor(email.EmailDelete))
	mux.HandleFunc("/api/email/read", contextIterceptor(email.MarkRead))
	mux.HandleFunc("/api/email/detail", contextIterceptor(email.EmailDetail))
	mux.HandleFunc("/api/email/move", contextIterceptor(email.Move))
	mux.HandleFunc("/api/email/send", contextIterceptor(email.Send))
	mux.HandleFunc("/api/settings/modify_password", contextIterceptor(controllers.ModifyPassword))
	mux.HandleFunc("/api/rule/get", contextIterceptor(controllers.GetRule))
	mux.HandleFunc("/api/rule/add", contextIterceptor(controllers.UpsertRule))
	mux.HandleFunc("/api/rule/update", contextIterceptor(controllers.UpsertRule))
	mux.HandleFunc("/api/rule/del", contextIterceptor(controllers.DelRule))
	mux.HandleFunc("/attachments/", contextIterceptor(controllers.GetAttachments))
	mux.HandleFunc("/attachments/download/", contextIterceptor(controllers.Download))
	mux.HandleFunc("/api/user/create", contextIterceptor(controllers.CreateUser))
	mux.HandleFunc("/api/user/edit", contextIterceptor(controllers.EditUser))
	mux.HandleFunc("/api/user/info", contextIterceptor(controllers.Info))
	mux.HandleFunc("/api/user/list", contextIterceptor(controllers.UserList))
	mux.HandleFunc("/api/plugin/settings/", contextIterceptor(controllers.SettingsHtml))
	mux.HandleFunc("/api/plugin/list", contextIterceptor(controllers.GetPluginList))
}

func HttpStart(httpsReady bool) {
	mux := http.NewServeMux()
	port := config.Instance.HttpPort
	if port == 0 {
		port = 80
	}

	var handler http.Handler = mux
	if httpsReady {
		mux.HandleFunc("/.well-known/", controllers.AcmeChallenge)
		mux.HandleFunc("/api/ping", controllers.Ping)
		mux.HandleFunc("/", controllers.Interceptor)
	} else {
		// Serve HTTP directly rather than redirecting to an unavailable HTTPS listener.
		router(mux)
		handler = session.Instance.LoadAndSave(mux)
	}
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      handler,
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
