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
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
)

func TestAddressApprovalHTTPPermissionsAndPrivateNotices(t *testing.T) {
	f := newAuthFixture(t)
	old := config.Instance
	config.Instance = &config.Config{Domain: "approval-http.test", AllowUserAlias: true, SecretKey: "test-only"}
	t.Cleanup(func() { config.Instance = old })
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	member := f.login(t, "member", "initial-password")
	admin := f.login(t, "admin", "initial-password")
	call := func(client *http.Client, method, path, body string, status int, key string) json.RawMessage {
		t.Helper()
		r, err := http.NewRequestWithContext(t.Context(), method, f.server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", f.server.URL)
		r.Header.Set("X-PMail-Lang", "en")
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out struct {
			ErrorNo int             `json:"errorNo"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("%s %s: status %d: %v", method, path, res.StatusCode, err)
		}
		if res.StatusCode != status || (status == 200 && out.ErrorNo != 0) {
			t.Fatalf("%s %s: got %d (%d), want %d", method, path, res.StatusCode, out.ErrorNo, status)
		}
		if key != "" {
			var detail map[string]string
			if err := json.Unmarshal(out.Data, &detail); err != nil || detail["code"] != key {
				t.Fatalf("%s %s did not return %s", method, path, key)
			}
		}
		return out.Data
	}
	const requests = "/api/admin/address-requests"
	const settings = "/api/admin/address-settings"
	const overview = "/api/admin/overview"
	for _, action := range []struct{ m, p, b string }{
		{"GET", requests, ""}, {"GET", overview, ""}, {"GET", settings, ""}, {"PUT", settings, `{"address_approval_required":false}`}, {"POST", requests + "/1/approve", ""}, {"POST", requests + "/1/reject", `{}`},
	} {
		call(member, action.m, action.p, action.b, 403, "inbound_permission_denied")
	}
	var policy models.AddressSettings
	if err := json.Unmarshal(call(admin, "GET", settings, "", 200, ""), &policy); err != nil || !policy.AddressApprovalRequired {
		t.Fatal("approval was not enabled by default")
	}
	create := func(client *http.Client, userID int, name string) address.View {
		t.Helper()
		b := fmt.Sprintf(`{"user_id":%d,"domain_id":%d,"local_part":%q}`, userID, domain.Default().ID, name)
		var row address.View
		if err := json.Unmarshal(call(client, "POST", "/api/address/create", b, 200, ""), &row); err != nil {
			t.Fatal(err)
		}
		return row
	}
	first := create(member, 0, "decide-one")
	second := create(member, 0, "decide-two")
	if first.Status != models.AddressPending || second.Status != models.AddressPending || first.RequestedAt == nil || first.CanSend {
		t.Fatal("user address activated before approval")
	}
	var info struct {
		Pending int `json:"pending_address_requests"`
	}
	if err := json.Unmarshal(call(admin, "GET", overview, "", 200, ""), &info); err != nil || info.Pending != 2 {
		t.Fatal("overview did not count pending requests")
	}
	var page address.RequestPage
	if err := json.Unmarshal(call(admin, "GET", requests+"?status=pending&page=2&page_size=1", "", 200, ""), &page); err != nil || len(page.Items) != 1 || page.Total != 2 || page.Pending != 2 || page.Page != 2 || page.Items[0].Applicant != "member" || page.Items[0].Address != first.Address {
		t.Fatal("unexpected paginated request data")
	}
	call(admin, "POST", fmt.Sprintf("%s/%d/reject", requests, first.ID), `{"reason":"Please use a different name."}`, 200, "")
	call(admin, "POST", fmt.Sprintf("%s/%d/approve", requests, second.ID), "", 200, "")
	call(admin, "POST", fmt.Sprintf("%s/%d/reject", requests, second.ID), "", 409, "address_already_reviewed")
	var rows []address.View
	if err := json.Unmarshal(call(member, "GET", "/api/address/list", "", 200, ""), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[1].Status != models.AddressRejected || rows[1].RejectReason != "Please use a different name." || rows[2].Status != models.AddressActive || !rows[2].CanSend {
		t.Fatal("result status not shown on address list")
	}
	var notices []models.UserEmail
	if err := f.db.Find(&notices); err != nil || len(notices) != 2 {
		t.Fatal("review should create exactly two notice copies")
	}
	for _, link := range notices {
		if link.UserID != f.member.ID {
			t.Fatal("notice reached anyone besides the applicant")
		}
	}
	if row := create(admin, f.member.ID, "decide-one"); row.Status != models.AddressActive {
		t.Fatal("admin provisioning wasn't active or rejected address still reserved")
	}
	for i := 0; i < 10; i++ {
		create(member, 0, fmt.Sprintf("next-%d", i))
	}
	call(member, "POST", "/api/address/create", fmt.Sprintf(`{"domain_id":%d,"local_part":"too-many"}`, domain.Default().ID), 409, "address_pending_limit")
	call(admin, "PUT", settings, `{"address_approval_required":false}`, 200, "")
	if row := create(member, 0, "policy-off"); row.Status != models.AddressActive {
		t.Fatal("configuration did not disable approval for future requests")
	}
}
