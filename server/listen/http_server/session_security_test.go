package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/token"
	"github.com/Jinnrry/pmail/session"
	"github.com/Jinnrry/pmail/utils/password"
	"github.com/alexedwards/scs/v2"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

type authFixture struct {
	server *httptest.Server
	db     *xorm.Engine
	admin  models.User
	member models.User
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Sync2(&models.User{}, &models.APIToken{}); err != nil {
		t.Fatal(err)
	}
	hash, err := password.Hash("initial-password")
	if err != nil {
		t.Fatal(err)
	}
	f := &authFixture{db: engine,
		admin:  models.User{Account: "admin", Password: hash, IsAdmin: 1},
		member: models.User{Account: "member", Password: hash},
	}
	if _, err := engine.Insert(&f.admin, &f.member); err != nil {
		t.Fatal(err)
	}
	oldDB, oldSession, oldInit := db.Instance, session.Instance, config.IsInit
	db.Instance, session.Instance, config.IsInit = engine, scs.New(), true
	mux := http.NewServeMux()
	router(mux, "dev")
	mux.HandleFunc("/test/legacy", func(w http.ResponseWriter, r *http.Request) {
		data, _ := json.Marshal(f.admin)
		session.Instance.Put(r.Context(), "user", string(data))
		response.NewSuccessResponse(nil).FPrint(w)
	})
	f.server = httptest.NewServer(session.Instance.LoadAndSave(mux))
	t.Cleanup(func() {
		f.server.Close()
		db.Instance, session.Instance, config.IsInit = oldDB, oldSession, oldInit
		engine.Close()
	})
	return f
}

func (f *authFixture) call(t *testing.T, client *http.Client, path string, body any) response.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, f.server.URL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", f.server.URL)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var result response.Response
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func (f *authFixture) login(t *testing.T, account, pwd string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	result := f.call(t, client, "/api/login", map[string]any{"account": account, "password": pwd})
	if result.ErrorNo != 0 {
		t.Fatalf("login: %+v", result)
	}
	return client
}

func TestSessionPasswordChangeRevokesAllLogins(t *testing.T) {
	f := newAuthFixture(t)
	first := f.login(t, "member", "initial-password")
	second := f.login(t, "member", "initial-password")
	result := f.call(t, first, "/api/settings/modify_password", map[string]any{"old_password": "initial-password", "new_password": "new-password"})
	if result.ErrorNo != 0 {
		t.Fatalf("change password: %+v", result)
	}
	for _, client := range []*http.Client{first, second} {
		if result := f.call(t, client, "/api/user/info", nil); result.ErrorNo != response.NeedLogin {
			t.Fatalf("old session remains authenticated: %+v", result)
		}
	}
	f.login(t, "member", "new-password")
}

func TestSessionAdminChangesRevokeExistingLogins(t *testing.T) {
	for _, change := range []string{"disable", "password reset"} {
		t.Run(change, func(t *testing.T) {
			f := newAuthFixture(t)
			admin := f.login(t, "admin", "initial-password")
			member := f.login(t, "member", "initial-password")
			edit := map[string]any{"id": f.member.ID}
			if change == "disable" {
				edit["disabled"] = 1
			} else {
				edit["password"] = "reset-password"
			}
			if result := f.call(t, admin, "/api/user/edit", edit); result.ErrorNo != 0 {
				t.Fatal(result)
			}
			if change == "disable" {
				// Re-enabling without first using the old cookie must not resurrect it.
				edit["disabled"] = 0
				if result := f.call(t, admin, "/api/user/edit", edit); result.ErrorNo != 0 {
					t.Fatal(result)
				}
			}
			if result := f.call(t, member, "/api/user/info", nil); result.ErrorNo != response.NeedLogin {
				t.Fatalf("old session remains authenticated: %+v", result)
			}
			pwd := "initial-password"
			if change == "password reset" {
				pwd = "reset-password"
			}
			f.login(t, "member", pwd)
		})
	}
}

func TestSessionReadsCurrentAccountState(t *testing.T) {
	for _, field := range []string{"disabled", "is_admin"} {
		t.Run(field, func(t *testing.T) {
			f := newAuthFixture(t)
			admin := f.login(t, "admin", "initial-password")
			value := 0
			if field == "disabled" {
				value = 1
			}
			if _, err := f.db.Table(&models.User{}).ID(f.admin.ID).Update(map[string]any{field: value}); err != nil {
				t.Fatal(err)
			}
			want := response.NoAccessPrivileges
			if field == "disabled" {
				want = response.NeedLogin
			}
			if result := f.call(t, admin, "/api/user/list", nil); result.ErrorNo != want {
				t.Fatalf("stale permissions: %+v, want error %d", result, want)
			}
		})
	}
}

func TestSessionRejectsLegacyLogin(t *testing.T) {
	f := newAuthFixture(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	f.call(t, client, "/test/legacy", nil)
	if result := f.call(t, client, "/api/user/info", nil); result.ErrorNo != response.NeedLogin {
		t.Fatal(result)
	}
}

func TestSessionProfileEditPreservesCredentials(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.login(t, "admin", "initial-password")
	member := f.login(t, "member", "initial-password")
	result := f.call(t, admin, "/api/user/edit", map[string]any{"id": f.member.ID, "username": "Updated"})
	if result.ErrorNo != 0 {
		t.Fatal(result)
	}
	if result := f.call(t, member, "/api/user/info", nil); result.ErrorNo != 0 {
		t.Fatal(result)
	}
	f.login(t, "member", "initial-password")
}

func TestCrossOriginCookieWriteRejectedButBearerWriteAllowed(t *testing.T) {
	f := newAuthFixture(t)
	browser := f.login(t, "admin", "initial-password")
	_, raw, err := token.Create(f.admin.ID, "cors-fixture", token.Admin, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	// The user edit route mutates only the fixture account and is shared by
	// both requests: only authentication mode changes.
	call := func(client *http.Client, bearer, display string) (int, response.Response) {
		data, _ := json.Marshal(map[string]any{"id": f.member.ID, "display_name": display})
		req, err := http.NewRequest(http.MethodPost, f.server.URL+"/api/user/edit", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://foreign.invalid")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var body response.Response
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, body
	}
	status, body := call(browser, "", "cookie must not edit")
	if status != 403 || body.ErrorNo != response.NoAccessPrivileges {
		t.Fatalf("cross-origin cookie: status=%d body=%+v", status, body)
	}
	status, body = call(&http.Client{}, raw, "bearer edit")
	if status != 200 || body.ErrorNo != 0 {
		t.Fatalf("cross-origin bearer: status=%d body=%+v", status, body)
	}
}
