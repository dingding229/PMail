package http_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/mailstore"
	"xorm.io/xorm"
)

func TestInboundHTTPSettingsRulesMailboxAndRiskGuards(t *testing.T) {
	t.Setenv("PMAIL_CATCH_ALL", "")
	f := newAuthFixture(t)
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	member := f.login(t, "member", "initial-password")
	admin := f.login(t, "admin", "initial-password")
	var raw string
	var headers http.Header
	call := func(client *http.Client, method, path, body string, status int, key string) map[string]any {
		t.Helper()
		r, err := http.NewRequest(method, f.server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", f.server.URL)
		r.Header.Set("X-PMail-Lang", "en")
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		raw = string(b)
		headers = res.Header
		if res.StatusCode != status {
			t.Fatalf("%s %s: want %d got %d %s", method, path, status, res.StatusCode, raw)
		}
		if !strings.Contains(headers.Get("Content-Type"), "application/json") || method == http.MethodHead {
			return nil
		}
		var out map[string]any
		if err = json.Unmarshal(b, &out); err != nil {
			t.Fatalf("JSON %q: %v", raw, err)
		}
		if key != "" {
			err, ok := out["data"].(map[string]any)
			if !ok || err["code"] != key || out["errorMsg"] == "" {
				t.Fatalf("code=%q body=%q", key, raw)
			}
		}
		return out
	}
	call(http.DefaultClient, "GET", "/api/settings/spam", "", 401, "inbound_login_required")
	call(member, "GET", "/api/admin/inbound", "", 403, "inbound_permission_denied")
	const clamavPath = "/api/admin/inbound/clamav"
	t.Setenv("PMAIL_CLAMAV", "")
	call(http.DefaultClient, "GET", clamavPath, "", 401, "inbound_login_required")
	call(member, "GET", clamavPath, "", 403, "inbound_permission_denied")
	call(member, "POST", clamavPath+"/update", `{}`, 403, "inbound_permission_denied")
	if got := call(admin, "GET", clamavPath, "", 200, "")["data"].(map[string]any)["enabled"]; got != false {
		t.Fatalf("ClamAV default enabled=%v", got)
	}
	call(admin, "POST", clamavPath+"/update", `{}`, 409, "clamav_disabled")
	call(admin, "PUT", clamavPath, `{"enabled":true}`, 200, "")
	t.Setenv("PMAIL_CLAMAV", "false")
	got := call(admin, "GET", clamavPath, "", 200, "")["data"].(map[string]any)
	if got["enabled"] != false || got["env_controlled"] != true {
		t.Fatalf("ClamAV override: %+v", got)
	}
	call(admin, "PUT", clamavPath, `{"enabled":true}`, 409, "clamav_env_controlled")
	t.Setenv("PMAIL_CLAMAV", "")
	call(admin, "PUT", clamavPath, `{"enabled":false}`, 200, "")
	call(member, "PUT", "/api/settings/spam", `{"enabled":true,"threshold":25,"bayes_enabled":true}`, 400, "inbound_invalid_parameters")
	call(admin, "PUT", "/api/admin/inbound", `{"dnsbl_enabled":false,"dnsbl_zones":[],"default_spam_threshold":8,"risk_block_dangerous_attachments":true}`, 200, "")
	call(admin, "PUT", "/api/admin/inbound", fmt.Sprintf(`{"dnsbl_enabled":false,"dnsbl_zones":[],"default_spam_threshold":8,"risk_block_dangerous_attachments":true,"catch_all":{"enabled":false,"recipient_id":%d}}`, f.member.ID), 200, "")
	out := call(admin, "GET", "/api/admin/inbound", "", 200, "")
	data, ok := out["data"].(map[string]any)
	if !ok {
		t.Fatal(raw)
	}
	policy, ok := data["catch_all"].(map[string]any)
	if !ok || policy["enabled"] != false || policy["recipient_id"] != float64(f.member.ID) || policy["env_controlled"] != false || !strings.Contains(raw, `"catch_all_recipients":`) {
		t.Fatal(raw)
	}
	t.Setenv("PMAIL_CATCH_ALL", "true")
	call(admin, "GET", "/api/admin/inbound", "", 200, "")
	if !strings.Contains(raw, `"catch_all":{"enabled":true,"env_controlled":true,`) {
		t.Fatal(raw)
	}
	t.Setenv("PMAIL_CATCH_ALL", "")
	if p := call(member, "GET", "/api/settings/spam", "", 200, ""); !strings.Contains(raw, `"threshold":8`) || p["errorNo"] != float64(0) {
		t.Fatal(raw)
	}
	call(member, "PUT", "/api/settings/spam", `{"enabled":true,"threshold":3,"bayes_enabled":false}`, 200, "")
	rule := `{"action":"block","kind":"domain","value":"test.example"}`
	created := call(member, "POST", "/api/spam/rules", rule, 200, "")
	call(member, "POST", "/api/spam/rules", rule, 409, "spam_rule_conflict")
	if out := call(admin, "GET", "/api/spam/rules", "", 200, ""); !strings.Contains(raw, `"data":[]`) || out["errorNo"] != float64(0) {
		t.Fatal(raw)
	}
	row, ok := created["data"].(map[string]any)
	if !ok {
		t.Fatalf("created response %+v", created)
	}
	deleteReq := fmt.Sprintf(`{"id":%v}`, row["id"])
	call(admin, "DELETE", "/api/spam/rules", deleteReq, 404, "inbound_resource_not_found")
	call(member, "DELETE", "/api/spam/rules", deleteReq, 200, "")
	msg := &parsemail.Email{Subject: "OTP: 928547", Text: []byte("Inert example"), Attachments: []*parsemail.Attachment{{Filename: "test.sh", ContentType: "text/plain", ContentID: "risk-fixture", Content: []byte("inert fixture")}}}
	wire, _ := json.Marshal(msg.Attachments)
	analysis := inbound.Analyze(context.Background(), msg, inbound.Envelope{}, nil)
	if analysis == nil || len(analysis.HighRiskParts) != 1 {
		t.Fatalf("analysis: %+v", analysis)
	}
	mail := models.Email{Subject: msg.Subject, Attachments: string(wire), InboundAnalysis: inbound.Storage(analysis)}
	mailstore.Prepare(&mail)
	if err := db.Transaction(f.db, func(tx *xorm.Session) error {
		if _, err := tx.Insert(&mail); err != nil {
			return err
		}
		if err := mailstore.WriteParts(tx, &mail, false); err != nil {
			return err
		}
		_, err := tx.Insert(&models.UserEmail{UserID: f.member.ID, EmailID: mail.Id})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	call(admin, "POST", "/api/email/spam", fmt.Sprintf(`{"ids":[%d],"is_spam":true}`, mail.Id), 404, "inbound_resource_not_found")
	call(member, "POST", "/api/email/spam", fmt.Sprintf(`{"ids":[%d],"is_spam":true}`, mail.Id), 200, "")
	call(member, "POST", "/api/email/list", `{"tag":"{\"status\":5,\"type\":0,\"group_id\":-1}"}`, 200, "")
	if !strings.Contains(raw, `"spam":{"verdict":"spam"}`) || !strings.Contains(raw, `"risk":{"level":"high"}`) || strings.Contains(raw, `"findings"`) || strings.Contains(raw, `"confidence"`) {
		t.Fatalf("summary: %s", raw)
	}
	call(member, "POST", "/api/email/detail", fmt.Sprintf(`{"id":%d}`, mail.Id), 200, "")
	if !strings.Contains(raw, `"code":"928547"`) || !strings.Contains(raw, `"score"`) || !strings.Contains(raw, `"attachment_index":0`) || !strings.Contains(raw, `"confidence"`) {
		t.Fatalf("details: %s", raw)
	}
	newID := fmt.Sprintf("%d.0", mail.Id)
	path := "/api/attachments/download/" + newID
	call(member, "GET", path, "", 409, "risk_confirmation_required")
	call(member, "HEAD", path, "", 409, "")
	call(member, "GET", "/api/attachments/"+newID, "", 409, "risk_confirmation_required")
	call(member, "GET", fmt.Sprintf("/attachments/%d/risk-fixture", mail.Id), "", 409, "risk_confirmation_required")
	call(member, "GET", fmt.Sprintf("/attachments/download/%d/0", mail.Id), "", 409, "risk_confirmation_required")
	call(member, "GET", "/api/attachments/preview/"+newID+"?confirm_risk=1", "", 403, "risk_preview_forbidden")
	call(member, "GET", path+"?confirm_risk=1", "", 200, "")
	if raw != "inert fixture" || headers.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(headers.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download %v %q", headers, raw)
	}
}
