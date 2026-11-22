//go:build integration

package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/registration"
	"github.com/Jinnrry/pmail/session"
)

// Exercise the HTTP router, persistent session store and a separate empty DB.
// TestMaster's explicitly bootstrapped integration DB is not reused here.
func TestRegistrationKeyHTTPFlow(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PMAIL_ADMIN_PASSWORD", "")
	t.Setenv("PMAIL_WEB_ROOT", filepath.Join(root, "web"))
	t.Setenv("PMAIL_DEFAULT_LANG", "zh-CN")
	cfg := &config.Config{DbType: config.DBTypeSQLite, DbDSN: filepath.Join(root, "registration.db"), Domain: "registration.test", WebDomain: "mail.registration.test"}
	engine, err := db.Open(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	oldDB, oldConfig, oldInit, oldRoot, oldSession := db.Instance, config.Instance, config.IsInit, config.ROOT_PATH, session.Instance
	db.Instance, config.Instance, config.IsInit, config.ROOT_PATH = engine, cfg, true, root
	initializedSession := false
	t.Cleanup(func() {
		// Prepare resets pending before consulting the database, even if an
		// early assertion stopped the fixture before successful registration.
		_ = registration.Prepare(context.Background(), "cleanup-owner", "fixture-cleanup-password")
		if initializedSession {
			session.Stop()
		}
		db.Instance, config.Instance, config.IsInit, config.ROOT_PATH, session.Instance = oldDB, oldConfig, oldInit, oldRoot, oldSession
		if oldDB != nil {
			_ = domain.Refresh()
		}
		_ = engine.Close()
	})
	if err := db.SyncSchema(engine); err != nil {
		t.Fatal(err)
	}
	if err := domain.Initialize(cfg); err != nil {
		t.Fatal(err)
	}
	if err := registration.Prepare(context.Background(), "admin", ""); err != nil {
		t.Fatal(err)
	}
	key, err := config.ReadRegistrationKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := registration.Prepare(context.Background(), "admin", ""); err != nil {
		t.Fatal(err)
	}
	if keyAfter, err := config.ReadRegistrationKey(); err != nil || keyAfter != key {
		t.Fatal("pending registration key did not persist")
	}
	session.Init()
	initializedSession = true
	mux := http.NewServeMux()
	router(mux, "registration-fixture")
	handler := webMiddleware(apiSessions(mux))
	call := func(method, path string, body map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(payload))
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	requireBoot := func(pending, loggedIn bool, cookies ...*http.Cookie) {
		t.Helper()
		w := call(http.MethodGet, "/api/bootstrap", nil, cookies...)
		var b struct {
			NeedRegister *bool `json:"need_register"`
			LoggedIn     *bool `json:"logged_in"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil || w.Code != 200 || b.NeedRegister == nil || *b.NeedRegister != pending || b.LoggedIn == nil || *b.LoggedIn != loggedIn {
			t.Fatalf("bootstrap HTTP %d: %s", w.Code, w.Body.String())
		}
	}
	requireEnvelope := func(w *httptest.ResponseRecorder, status, code int) json.RawMessage {
		t.Helper()
		var env struct {
			ErrorNo int             `json:"errorNo"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || w.Code != status || env.ErrorNo != code || len(env.Data) == 0 {
			t.Fatalf("registration HTTP %d, errorNo %d; want %d/%d; body: %s", w.Code, env.ErrorNo, status, code, w.Body.String())
		}
		return env.Data
	}
	requireBoot(true, false)
	assertUnknownAPI(t, call(http.MethodGet, "/api/not-a-route", nil), "找不到请求的资源")
	requireEnvelope(call(http.MethodPost, "/api/user/info", nil), 403, response.NoAccessPrivileges)
	body := map[string]string{"registration_key": "wrong", "account": "owner", "password": "registration-password"}
	requireEnvelope(call(http.MethodPost, "/api/register", body), 403, response.NoAccessPrivileges)
	body["registration_key"] = key
	w := call(http.MethodPost, "/api/register", body)
	data := requireEnvelope(w, 200, 0)
	var user struct {
		ID        int    `json:"id"`
		Admin     int    `json:"is_admin"`
		Account   string `json:"account"`
		Addresses []struct {
			Address string `json:"address"`
			Primary bool   `json:"is_primary"`
		} `json:"addresses"`
	}
	if err := json.Unmarshal(data, &user); err != nil || user.ID <= 0 || user.Admin != 1 || user.Account != "owner" || len(user.Addresses) != 1 || user.Addresses[0].Address != "owner@registration.test" || !user.Addresses[0].Primary {
		t.Fatalf("new administrator: %s", w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "session" || !cookies[0].HttpOnly {
		t.Fatal("registration failed to set an HttpOnly session cookie")
	}
	requireEnvelope(call(http.MethodPost, "/api/user/info", nil, cookies...), 200, 0)
	requireBoot(false, true, cookies...)
	if _, err := config.ReadRegistrationKey(); !os.IsNotExist(err) {
		t.Fatalf("registration key still accessible: %v", err)
	}
	requireEnvelope(call(http.MethodPost, "/api/register", body), 403, response.NoAccessPrivileges)
	if count, err := engine.Count(&models.User{}); err != nil || count != 1 {
		t.Fatalf("registration created %d users: %v", count, err)
	}
}
