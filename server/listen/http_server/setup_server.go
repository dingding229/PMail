package http_server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/controllers"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/utils/securefile"
	log "github.com/sirupsen/logrus"
)

var setupServer *http.Server
var setupTokenFile string
var setupActive atomic.Bool

func setupOnly(expected string, next http.HandlerFunc) http.HandlerFunc {
	hash := sha256.Sum256([]byte(expected))
	return func(w http.ResponseWriter, r *http.Request) {
		token := sha256.Sum256([]byte(r.Header.Get("X-PMail-Setup-Token")))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if expected == "" || subtle.ConstantTimeCompare(hash[:], token[:]) != 1 {
			response.Localized(i18n.ForRequest(r), response.NoAccessPrivileges, "setup_token_required").FPrintStatus(w, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func SetupStart(version string) error {
	if os.Getenv("PMAIL_LEGACY_SETUP") != "1" {
		return errors.New("legacy setup is disabled")
	}
	port := config.Instance.HttpPort
	if port <= 0 {
		port, _ = strconv.Atoi(os.Getenv("setup_port"))
	}
	if port <= 0 || port > 65535 {
		port = 80
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		listener.Close()
		return err
	}
	token := hex.EncodeToString(raw[:])
	path := filepath.Join(config.ROOT_PATH, "config", "legacy_setup_token")
	if err = securefile.Write(path, []byte(token+"\n"), false); err != nil {
		listener.Close()
		return err
	}
	config.Instance.SetSetupPort(port)
	mux := http.NewServeMux()
	var mu sync.Mutex
	serialize := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { mu.Lock(); defer mu.Unlock(); h(w, r) }
	}
	mux.HandleFunc("/", staticWeb())
	guarded := setupOnly(token, contextIterceptor(controllers.Setup))
	mux.HandleFunc("POST /api/setup", serialize(func(w http.ResponseWriter, r *http.Request) {
		if !setupActive.Load() {
			apiNotFound(w, r)
			return
		}
		guarded(w, r)
	}))
	mux.HandleFunc("/api/ping", controllers.Ping)
	mux.HandleFunc("GET /api/bootstrap", serialize(contextIterceptor(controllers.Bootstrap(version))))
	server := &http.Server{Handler: webMiddleware(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second}
	setupServer, setupTokenFile = server, path
	setupActive.Store(true)
	log.Infof("Legacy setup is local at http://%s; setup token file: %q", listener.Addr(), path)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("legacy setup listener stopped: %v", err)
		}
	}()
	return nil
}

func SetupStop() {
	if setupServer == nil {
		return
	}
	setupActive.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := setupServer.Shutdown(ctx); err != nil {
		_ = setupServer.Close()
	}
	if err := os.Remove(setupTokenFile); err != nil && !os.IsNotExist(err) {
		log.Warnf("remove consumed setup token: %v", err)
	}
	setupServer, setupTokenFile = nil, ""
	log.Info("Legacy setup stopped")
}
