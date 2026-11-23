package http_server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/registration"
	log "github.com/sirupsen/logrus"
)

// SaveDomainFunc persists the primary mail domain chosen on the first-run page.
type SaveDomainFunc func(mailDomain, webDomain string) error

type domainSetupBody struct {
	Key    string `json:"registration_key"`
	Domain string `json:"domain"`
}

// DomainSetup blocks while serving only the web app, /api/ping,
// /api/bootstrap and POST /api/setup/domain. It returns after an administrator
// holding the registration key has saved the primary mail domain, so the rest
// of PMail can start without any domain in the environment.
func DomainSetup(version string, save SaveDomainFunc) error {
	if _, err := config.EnsureRegistrationKey(); err != nil {
		return fmt.Errorf("create registration key: %w", err)
	}
	port := config.Instance.HttpPort
	if port == 0 {
		port = 80
	}
	if version == "" {
		version = "dev"
	}
	done := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/", staticWeb())
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errorNo":0,"errorMsg":"","data":"pong"}`))
	})
	mux.HandleFunc("GET /api/bootstrap", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": version, "need_register": true, "need_domain": true, "logged_in": false,
			"default_lang": "zh-CN", "web_domain": "", "primary_domain": "", "domains": []string{},
			"features": map[string]bool{"register_key": true},
		})
	})
	mux.HandleFunc("POST /api/setup/domain", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		reply := func(status, errorNo int, msg string) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"errorNo": errorNo, "errorMsg": msg, "data": ""})
		}
		if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || media != "application/json" {
			reply(http.StatusUnsupportedMediaType, 100, "json_media_type_required")
			return
		}
		var body domainSetupBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			reply(http.StatusBadRequest, 100, "invalid_request")
			return
		}
		if err := registration.VerifyKey(body.Key); err != nil {
			time.Sleep(time.Second) // slow down key guessing
			reply(http.StatusForbidden, 405, "registration_key_invalid")
			return
		}
		name, err := domain.Normalize(body.Domain)
		if err != nil || !strings.Contains(name, ".") {
			reply(http.StatusBadRequest, 100, "domain_invalid")
			return
		}
		if err := save(name, setupWebDomain(r.Host, name)); err != nil {
			log.Errorf("save mail domain: %v", err)
			reply(http.StatusInternalServerError, 500, "internal_error")
			return
		}
		log.Infof("audit_event=mail_domain_configured domain=%q", name)
		reply(http.StatusOK, 0, "")
		once.Do(func() { close(done) })
	})

	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: webMiddleware(mux), ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	socket, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	fmt.Printf("PMail 首次设置 / First-run setup: 打开网页设置邮件域名 / open the web page to set the mail domain; 注册密钥文件 / Registration key file: %q\n", config.RegistrationKeyPath())
	go func() {
		if err := server.Serve(socket); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("first-run setup HTTP %s: %v", server.Addr, err)
		}
	}()
	<-done
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

// setupWebDomain keeps the host name the administrator used to open the page
// (for example pm.example.com) as the web domain, unless it is an IP address
// or the default mail.<domain>.
func setupWebDomain(host, mailDomain string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	web, err := domain.Normalize(host)
	if err != nil || net.ParseIP(host) != nil || !strings.Contains(web, ".") || web == "mail."+mailDomain || web == "localhost" {
		return ""
	}
	return web
}
