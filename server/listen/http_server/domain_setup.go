package http_server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/registration"
	"github.com/Jinnrry/pmail/utils/clientip"
	log "github.com/sirupsen/logrus"
)

// SaveDomainFunc persists the primary mail domain chosen on the first-run page.
type SaveDomainFunc func(mailDomain string) error

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
	socket, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	fmt.Printf("PMail 首次设置 / First-run setup: 打开网页设置邮件域名 / open the web page to set the mail domain; 注册密钥文件 / Registration key file: %q\n", config.RegistrationKeyPath())
	return serveDomainSetup(socket, version, save, 5*time.Second)
}

func serveDomainSetup(socket net.Listener, version string, save SaveDomainFunc, drainTimeout time.Duration) error {
	done := make(chan struct{})

	server := &http.Server{
		Handler:           domainSetupHandler(version, save, func() { close(done) }),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second, WriteTimeout: 30 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(socket) }()
	select {
	case err := <-serveErr:
		return err
	case <-done:
	}
	ctx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		// The domain is committed. Do not let an unrelated slow connection
		// abort startup; release the port and continue into the normal server.
		log.Warnf("forcing first-run HTTP listener shutdown: %v", err)
		_ = server.Close()
	}
	return nil
}

func domainSetupHandler(version string, save SaveDomainFunc, onSaved func()) http.Handler {
	committed := false
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
			"default_lang": i18n.Default(), "web_domain": "", "primary_domain": "", "domains": []string{},
			"features": map[string]bool{"register_key": true},
		})
	})
	mux.HandleFunc("POST /api/setup/domain", func(w http.ResponseWriter, r *http.Request) {
		reply := func(status, errorNo int, msg string) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"errorNo": errorNo, "errorMsg": msg, "data": ""})
		}
		if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || media != "application/json" {
			reply(http.StatusUnsupportedMediaType, 100, "json_media_type_required")
			return
		}
		if (r.Header.Get("Origin") != "" || r.Header.Get("Referer") != "") && !sameOrigin(r) {
			noteOriginRejection(r)
			reply(http.StatusForbidden, 405, "origin_forbidden")
			return
		}
		var body domainSetupBody
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		if err := dec.Decode(&body); err != nil || dec.Decode(new(any)) != io.EOF {
			reply(http.StatusBadRequest, 100, "invalid_request")
			return
		}
		finish, ok := auth.ReserveRegistration(clientip.Source(r))
		if !ok {
			w.Header().Set("Retry-After", "600")
			reply(http.StatusTooManyRequests, 429, "registration_rate_limit")
			return
		}
		failed := false
		defer func() { finish(failed) }()
		if err := registration.VerifyKey(body.Key); err != nil {
			if errors.Is(err, registration.ErrInvalidKey) {
				failed = true
				reply(http.StatusForbidden, 405, "registration_key_invalid")
			} else {
				log.Errorf("read first-run registration key: %v", err)
				reply(http.StatusInternalServerError, 500, "internal_error")
			}
			return
		}
		name, err := domain.Normalize(body.Domain)
		if err != nil || !strings.Contains(name, ".") || net.ParseIP(name) != nil {
			reply(http.StatusBadRequest, 100, "domain_invalid")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if committed {
			reply(http.StatusConflict, 100, "mail_domain_already_configured")
			return
		}
		if err := save(name); err != nil {
			log.Errorf("save mail domain: %v", err)
			reply(http.StatusInternalServerError, 500, "internal_error")
			return
		}
		log.Infof("audit_event=mail_domain_configured domain=%q", name)
		reply(http.StatusOK, 0, "")
		committed = true
		onSaved()
	})

	return webMiddleware(mux)
}
