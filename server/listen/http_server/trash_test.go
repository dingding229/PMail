package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/del_email"
)

func TestTrashAPISettingsDatesOwnershipAndEmpty(t *testing.T) {
	f := newAuthFixture(t)
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	admin := f.login(t, "admin", "initial-password")
	member := f.login(t, "member", "initial-password")
	var mail models.Email
	if _, err := f.db.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	at := int64(1764000000000)
	link := models.UserEmail{EmailID: mail.Id, UserID: f.member.ID, Status: consts.EmailStatusDel, DeletedAt: &at}
	if _, err := f.db.Insert(&link); err != nil {
		t.Fatal(err)
	}
	call := func(client *http.Client, method, path string, body any, status int) any {
		t.Helper()
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		r, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", f.server.URL)
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		res, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var envelope struct {
			ErrorNo int `json:"errorNo"`
			Data    any `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != status || (status == 200 && envelope.ErrorNo != 0) {
			t.Fatalf("%s %s HTTP=%d response=%+v", method, path, res.StatusCode, envelope)
		}
		return envelope.Data
	}
	const settings = "/api/admin/trash"
	call(member, "GET", settings, nil, 403)
	call(member, "PUT", settings, map[string]any{"trash_retention_days": 0}, 403)
	call(admin, "PUT", settings, map[string]any{"trash_retention_days": 3651}, 400)
	if result := call(admin, "GET", settings, nil, 200).(map[string]any); result["trash_retention_days"] != float64(30) {
		t.Fatalf("default: %+v", result)
	}
	listTrash := func() map[string]any {
		t.Helper()
		x := call(member, "POST", "/api/email/list", map[string]any{"tag": `{"type":-1,"status":3,"group_id":-1}`}, 200).(map[string]any)
		return x["list"].([]any)[0].(map[string]any)
	}
	if x := listTrash(); x["deleted_at"] != float64(at) || x["purge_at"] != float64(at+30*del_email.DayMillis) {
		t.Fatalf("dates: %+v", x)
	}
	call(admin, "PUT", settings, map[string]any{"trash_retention_days": 0}, 200)
	if x := listTrash(); x["deleted_at"] != float64(at) || x["purge_at"] != nil {
		t.Fatalf("disabled purge dates: %+v", x)
	} else if _, ok := x["purge_at"]; !ok {
		t.Fatal("purge_at must be present and null")
	}
	body := map[string]any{"ids": []int{mail.Id}}
	if out := call(admin, "POST", "/api/email/restore", body, 200).(map[string]any); out["restored"] != float64(0) {
		t.Fatal(out)
	}
	if out := call(admin, "POST", "/api/email/trash/empty", nil, 200).(map[string]any); out["deleted"] != float64(0) {
		t.Fatal(out)
	}
	if out := call(member, "POST", "/api/email/restore", body, 200).(map[string]any); out["restored"] != float64(1) {
		t.Fatal(out)
	}
	x := call(member, "POST", "/api/email/list", map[string]any{"tag": `{"type":0,"status":-1,"group_id":0}`}, 200).(map[string]any)
	if _, ok := x["list"].([]any)[0].(map[string]any)["deleted_at"]; ok {
		t.Fatal("non-Trash response changed")
	}
	call(member, "POST", "/api/email/del", body, 200)
	if out := call(member, "POST", "/api/email/trash/empty", nil, 200).(map[string]any); out["deleted"] != float64(1) {
		t.Fatal(out)
	}
}
