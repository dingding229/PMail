package http_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/token"
)

func TestTranslationRoutesAuthorizationAvailabilityAndAccountCleanup(t *testing.T) {
	f := newAuthFixture(t)
	old := config.Instance
	config.Instance = &config.Config{SecretKey: "fixture-only"}
	t.Cleanup(func() { config.Instance = old })
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	admin := f.login(t, "admin", "initial-password")
	member := f.login(t, "member", "initial-password")
	_, raw, err := token.Create(f.member.ID, "verify-scope", token.Read, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	call := func(client *http.Client, method, path, body, bearer string, want int, code string) map[string]any {
		t.Helper()
		r, err := http.NewRequestWithContext(t.Context(), method, f.server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", f.server.URL)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out struct {
			ErrorNo int            `json:"errorNo"`
			Data    map[string]any `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("%s %s response: %v", method, path, err)
		}
		if res.StatusCode != want || (code != "" && out.Data["code"] != code) || (want == 200 && out.ErrorNo != 0) {
			t.Fatalf("%s %s: status %d error %d, code %v", method, path, res.StatusCode, out.ErrorNo, out.Data["code"])
		}
		return out.Data
	}
	const policy = "/api/admin/translation"
	call(http.DefaultClient, "GET", policy, "", "", 401, "inbound_login_required")
	call(member, "GET", policy, "", "", 403, "inbound_permission_denied")
	call(http.DefaultClient, "PUT", "/api/settings/ai", `{}`, raw, 403, "inbound_permission_denied")
	call(admin, "PUT", policy, `{"web_enabled":true,"ai_enabled":true}`, "", 200, "")
	mail := models.Email{Subject: "Your weekly account summary is ready to view"}
	if _, err := f.db.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Insert(&models.UserEmail{UserID: f.member.ID, EmailID: mail.Id}); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"id":%d,"engine":"ai"}`, mail.Id)
	call(admin, "POST", "/api/email/translate", body, "", 404, "translation_mail_not_found")
	call(member, "POST", "/api/email/translate", body, "", 409, "ai_not_configured")
	call(member, "PUT", "/api/settings/translation", `{"target_lang":"zh-CN","translate_engine":"web","auto_translate":true,"ai_auto_summary":true}`, "", 200, "")
	data := call(member, "PUT", "/api/settings/ai", `{"base_url":"https://model.example.test/v1","model":"test-model","api_key":"fixture-only-do-not-echo"}`, "", 200, "")
	if data["api_key_set"] != true || data["api_key"] != nil {
		t.Fatal("incorrect key redaction")
	}
	data = call(member, "POST", "/api/email/detail", fmt.Sprintf(`{"id":%d}`, mail.Id), "", 200, "")
	if data["ai_available"] != true || data["needs_translation"] != true || data["translation_available"] != true {
		t.Fatal("incorrect email detail availability")
	}
	call(admin, "POST", "/api/user/delete", fmt.Sprintf(`{"id":%d}`, f.member.ID), "", 200, "")
	if alive, err := f.db.ID(f.member.ID).Exist(&models.AIConfig{}); err != nil || alive {
		t.Fatal("account deletion left AI settings")
	}
}
