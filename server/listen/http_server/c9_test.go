package http_server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/dkimkey"
)

func TestC9MultipartPreviewAndDraftReference(t *testing.T) {
	f := newAuthFixture(t)
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	member := f.login(t, "member", "initial-password")
	admin := f.login(t, "admin", "initial-password")
	key, err := dkimkey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	d := models.Domain{Name: "example.test", IsDefault: true, DkimSelector: "default", DkimPrivateKey: key}
	if _, err := f.db.Insert(&d); err != nil {
		t.Fatal(err)
	}
	if err := domain.Refresh(); err != nil {
		t.Fatal(err)
	}
	a := models.MailboxAddress{DomainID: d.ID, UserID: f.member.ID, LocalPart: "member", CanSend: true, IsPrimary: true}
	if _, err := f.db.Insert(&a); err != nil {
		t.Fatal(err)
	}
	call := func(client *http.Client, method, path, ct string, body io.Reader, status int) []byte {
		t.Helper()
		req, err := http.NewRequest(method, f.server.URL+path, body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", f.server.URL)
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		result, err := io.ReadAll(res.Body)
		if err != nil || res.StatusCode != status {
			t.Fatalf("%s %s: status %d wanted %d, %s %v", method, path, res.StatusCode, status, result, err)
		}
		return result
	}
	var form bytes.Buffer
	mw := multipart.NewWriter(&form)
	part, err := mw.CreateFormFile("file", "upload.png")
	if err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\ninert fixture")
	_, _ = part.Write(png)
	if err = mw.Close(); err != nil {
		t.Fatal(err)
	}
	raw := call(member, "POST", "/api/compose/inline-image", mw.FormDataContentType(), &form, 200)
	var out struct {
		ErrorNo int `json:"errorNo"`
		Data    struct {
			CID string `json:"cid"`
			URL string `json:"url"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &out); err != nil || out.ErrorNo != 0 || out.Data.CID == "" {
		t.Fatalf("upload %s %v", raw, err)
	}
	if got := call(member, "GET", out.Data.URL, "", nil, 200); !bytes.Equal(got, png) {
		t.Fatal("wrong protected bytes")
	}
	call(admin, "GET", out.Data.URL, "", nil, 404)
	draft := fmt.Sprintf(`{"from_address":"member@example.test","html":%q,"inline_images":[{"cid":%q}]}`, `<p>hello<img src="cid:`+out.Data.CID+`"></p>`, out.Data.CID)
	raw = call(member, "POST", "/api/email/draft/save", "application/json", strings.NewReader(draft), 200)
	var dResult struct {
		ErrorNo int `json:"errorNo"`
		Data    struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &dResult); err != nil || dResult.ErrorNo != 0 || dResult.Data.ID <= 0 {
		t.Fatalf("draft %s %v", raw, err)
	}
	if ok, err := f.db.Where("draft_id=? AND cid=?", dResult.Data.ID, out.Data.CID).Exist(&models.DraftInlineImage{}); err != nil || !ok {
		t.Fatal("draft lost image reference", err)
	}
	call(member, "POST", "/api/email/draft/delete", "application/json", strings.NewReader(fmt.Sprintf(`{"id":%d}`, dResult.Data.ID)), 200)
	if ok, err := f.db.Where("draft_id=?", dResult.Data.ID).Exist(&models.DraftInlineImage{}); err != nil || ok {
		t.Fatal("draft left an image reference", err)
	}
}

func TestOnlineUpdateRequiresSameOriginAdminAndVersionConfirmation(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.login(t, "admin", "initial-password")
	member := f.login(t, "member", "initial-password")
	for _, tc := range []struct {
		name, path, method, contentType, body, origin string
		client                                        *http.Client
		status                                        int
	}{
		{"anonymous", "/api/admin/update/apply", "POST", "application/json", `{"version":"v0.0.4"}`, f.server.URL, f.server.Client(), 401},
		{"member", "/api/admin/update/apply", "POST", "application/json", `{"version":"v0.0.4"}`, f.server.URL, member, 403},
		{"cross-origin", "/api/admin/update/apply", "POST", "application/json", `{"version":"v0.0.4"}`, "https://elsewhere.test", admin, 403},
		{"unconfirmed", "/api/admin/update/apply", "POST", "application/json", `{}`, f.server.URL, admin, 400},
		{"job member", "/api/admin/update/apply/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "GET", "", "", f.server.URL, member, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.client
			req, err := http.NewRequest(tc.method, f.server.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Origin", tc.origin)
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.status {
				raw, _ := io.ReadAll(res.Body)
				t.Fatalf("HTTP %d, want %d: %s", res.StatusCode, tc.status, raw)
			}
		})
	}
}
