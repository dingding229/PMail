package http_server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
)

func TestMailReportRoutesPermissionsAndApply(t *testing.T) {
	f := newAuthFixture(t)
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	admin := f.login(t, "admin", "initial-password")
	member := f.login(t, "member", "initial-password")
	const pref = "/api/settings/report-mail"
	const prefix = "/api/admin/mail-reports/"
	call := func(client *http.Client, method, p, body string, status int) map[string]any {
		t.Helper()
		r, err := http.NewRequestWithContext(t.Context(), method, f.server.URL+p, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Origin", f.server.URL)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		x, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer x.Body.Close()
		var out struct {
			ErrorNo int            `json:"errorNo"`
			Data    map[string]any `json:"data"`
		}
		if err := json.NewDecoder(x.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if x.StatusCode != status || (status == 200 && out.ErrorNo != 0) {
			t.Fatalf("%s %s: %d %+v", method, p, x.StatusCode, out)
		}
		return out.Data
	}
	if _, err := f.db.Insert(&models.Domain{Name: "report.test", IsDefault: true}); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"summary", "records", "reports"} {
		call(http.DefaultClient, "GET", prefix+view, "", 401)
		call(member, "GET", prefix+view, "", 403)
		call(admin, "GET", prefix+view+"?domain=report.test", "", 200)
	}
	p := call(member, "GET", pref, "", 200)
	if p["enabled"] != true || p["group_id"] != float64(0) || p["mark_read"] != true || p["forward_reports"] != false {
		t.Fatal(p)
	}
	g := models.Group{Name: "Other", UserId: f.member.ID + 99}
	if _, err := f.db.Insert(&g); err != nil {
		t.Fatal(err)
	}
	if x := call(member, "PUT", pref, fmt.Sprintf(`{"group_id":%d}`, g.ID), 400); x["code"] != "report_mail_invalid_group" {
		t.Fatal(x)
	}
	call(member, "PUT", pref, `{"forward_reports":true}`, 200)
	if y := call(member, "PUT", pref, `{"enabled":true,"group_id":0,"mark_read":false}`, 200); y["forward_reports"] != true || y["mark_read"] != false {
		t.Fatal(y)
	}
	mail := models.Email{Subject: "report in a generic wrapper", ProjectionReady: true}
	if _, err := f.db.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Insert(&models.MailAttachmentInfo{EmailID: mail.Id, Filename: "rpt.test!report.test!1795392000!1795478400.xml.gz", ContentType: "application/gzip"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Insert(&models.UserEmail{UserID: f.member.ID, EmailID: mail.Id}); err != nil {
		t.Fatal(err)
	}
	ap := call(member, "POST", pref+"/apply", `{}`, 200)
	if ap["moved"] != float64(1) || ap["group_id"] == float64(0) {
		t.Fatal(ap)
	}
	// This detail request is what /mail/inbox?mail=<id> uses; scope is
	// ownership, not folder, even though the link is no longer in Inbox.
	if d := call(member, "POST", "/api/email/detail", fmt.Sprintf(`{"id":%d}`, mail.Id), 200); d["subject"] != mail.Subject {
		t.Fatal(d)
	}
	if again := call(member, "POST", pref+"/apply", `{}`, 200); again["moved"] != float64(0) {
		t.Fatal(again)
	}
}
