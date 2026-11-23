package http_server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/mailaddress"
)

func TestAddressViewsAPIEnforcesOwnershipAndFiltersSearchAndCounts(t *testing.T) {
	f := newAuthFixture(t)
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Domain: "address-api.test"}
	if err := domain.Initialize(cfg); err != nil {
		t.Fatal(err)
	}
	newAddress := func(owner int, local string, primary bool) models.MailboxAddress {
		t.Helper()
		a := models.MailboxAddress{UserID: owner, DomainID: domain.Default().ID, LocalPart: local, IsPrimary: primary, CanSend: true}
		if _, err := f.db.Insert(&a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	memberMain := newAddress(f.member.ID, "member", true)
	memberAlias := newAddress(f.member.ID, "member-alias", false)
	adminAddress := newAddress(f.admin.ID, "admin", true)
	for _, local := range []string{"member", "member-alias"} {
		e := models.Email{Subject: "shared search token " + local, To: local + "@address-api.test"}
		if _, err := f.db.Insert(&e); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Insert(&models.UserEmail{UserID: f.member.ID, EmailID: e.Id}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := mailaddress.Backfill(t.Context(), f.db); err != nil || n != 2 {
		t.Fatalf("backfill: %d %v", n, err)
	}
	member := f.login(t, "member", "initial-password")
	admin := f.login(t, "admin", "initial-password")
	call := func(client *http.Client, method, path string, body any, code int) any {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", f.server.URL)
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out struct {
			ErrorNo int `json:"errorNo"`
			Data    any `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != code || (code == 200 && out.ErrorNo != 0) {
			t.Fatalf("%s: HTTP %d, result %+v", path, res.StatusCode, out)
		}
		return out.Data
	}
	query := func(address any) map[string]any {
		return map[string]any{"keyword": "shared search token", "address": address, "tag": `{"type":0,"status":-1,"group_id":0}`}
	}
	call(member, "POST", "/api/email/list", query(adminAddress.ID), 403)
	call(admin, "POST", "/api/email/list", query(memberAlias.ID), 403)
	call(member, "POST", "/api/group", map[string]any{"address": fmt.Sprint(adminAddress.ID)}, 403)
	call(member, "POST", "/api/email/list", query("-2"), 400)
	displayed := call(member, "POST", "/api/email/list", query(memberAlias.ID), 200).(map[string]any)["list"].([]any)
	if len(displayed) != 1 {
		t.Fatalf("search matches: %+v", displayed)
	}
	item := displayed[0].(map[string]any)
	if item["title"] != "shared search token member-alias" || item["mailbox_addresses"].([]any)[0].(map[string]any)["address"] != "member-alias@address-api.test" {
		t.Fatalf("matched address labels: %+v", item)
	}
	groups := call(member, "POST", "/api/group", map[string]any{"address": memberAlias.ID}, 200).([]any)
	if groups[0].(map[string]any)["unread"] != float64(1) {
		t.Fatalf("filtered inbox count: %+v", groups[0])
	}
	counts := call(member, "GET", "/api/email/address_counts", nil, 200).(map[string]any)
	if counts["all_unread"] != float64(2) || len(counts["addresses"].([]any)) != 2 {
		t.Fatalf("counts: %+v", counts)
	}
	// A draft follows From, then moves to the new sender's Drafts view when edited.
	draft := map[string]any{"from_address": "member-alias@address-api.test", "subject": "draft attribution"}
	created := call(member, "POST", "/api/email/draft/save", draft, 200).(map[string]any)
	id := created["id"]
	draftMatches := func(address int) int {
		t.Helper()
		out := call(member, "POST", "/api/email/list", map[string]any{"address": address, "tag": `{"type":0,"status":4,"group_id":-1}`}, 200).(map[string]any)
		if rows, ok := out["list"].([]any); ok {
			return len(rows)
		}
		return 0
	}
	if draftMatches(memberAlias.ID) != 1 || draftMatches(memberMain.ID) != 0 {
		t.Fatal("draft did not inherit alias From")
	}
	draft["id"], draft["from_address"] = id, "member@address-api.test"
	call(member, "POST", "/api/email/draft/save", draft, 200)
	if draftMatches(memberMain.ID) != 1 || draftMatches(memberAlias.ID) != 0 {
		t.Fatal("changing sender did not move draft")
	}
}
