package email

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/utils/send"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/utils/context"
)

func composeFixture(t *testing.T) (*context.Context, *context.Context) {
	t.Helper()
	cfg := &config.Config{DbType: config.DBTypeSQLite, DbDSN: filepath.Join(t.TempDir(), "mail.db"), Domain: "example.test"}
	engine, err := db.Open(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	old, oldCfg := db.Instance, config.Instance
	db.Instance, config.Instance = engine, cfg
	t.Cleanup(func() { db.Instance, config.Instance = old, oldCfg; _ = engine.Close() })
	if err := db.SyncSchema(engine); err != nil {
		t.Fatal(err)
	}
	if err := domain.Initialize(cfg); err != nil {
		t.Fatal(err)
	}
	a, b := models.User{Account: "owner", Name: "Owner", Password: "unused"}, models.User{Account: "other", Name: "Other", Password: "unused", IsAdmin: 1}
	for _, u := range []*models.User{&a, &b} {
		if _, err := engine.Insert(u); err != nil {
			t.Fatal(err)
		}
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	return &context.Context{Context: stdctx.Background(), UserID: a.ID, UserName: a.Name}, &context.Context{Context: stdctx.Background(), UserID: b.ID, UserName: b.Name, IsAdmin: true}
}

func callCompose(t *testing.T, fn func(*context.Context, http.ResponseWriter, *http.Request), ctx *context.Context, payload string) (int, int, json.RawMessage) {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	fn(ctx, w, r)
	var res struct {
		Err  int             `json:"errorNo"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid API body: %v: %s", err, w.Body.String())
	}
	return w.Code, res.Err, res.Data
}

func TestDraftSaveUpdateDetailOwnerAndDelete(t *testing.T) {
	owner, other := composeFixture(t)
	status, code, data := callCompose(t, SaveDraft, owner, `{"to":[{"name":"Friend","email":"friend@example.test"}],"cc":[{"name":"Cc","email":"cc@example.test"}],"bcc":[{"name":"Bcc","email":"bcc@example.test"}],"subject":"first","html":"<pre><code class=language-go>x</code></pre>","attrs":[{"name":"a.py","data":"data:text/plain;base64,cHJpbnQoMSk="}]}`)
	var saved struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(data, &saved); err != nil || status != 200 || code != 0 || saved.ID <= 0 {
		t.Fatalf("draft save %d/%d %s", status, code, data)
	}
	id, _ := json.Marshal(saved.ID)
	idText := string(id)
	if status, _, _ := callCompose(t, SaveDraft, other, `{"id":`+idText+`,"subject":"intruder"}`); status != 404 {
		t.Fatalf("non-owner update = %d", status)
	}
	if _, code, _ := callCompose(t, EmailDetail, other, `{"id":`+idText+`}`); code == 0 {
		t.Fatal("non-owner admin read a draft")
	}
	if _, code, _ := callCompose(t, SaveDraft, owner, `{"id":`+idText+`,"to":[{"email":"friend@example.test"}],"cc":[{"email":"cc@example.test"}],"bcc":[{"email":"bcc@example.test"}],"subject":"updated","text":"`+"```go\\nx\\n```"+`"}`); code != 0 {
		t.Fatalf("draft update: errorNo %d", code)
	}
	if _, code, _ := callCompose(t, DeleteDraft, other, `{"ids":[`+idText+`]}`); code != 0 {
		t.Fatalf("idempotent unauthorized delete errorNo %d", code)
	}
	_, code, data = callCompose(t, EmailDetail, owner, `{"id":`+idText+`}`)
	if code != 0 || !strings.Contains(string(data), `"subject":"updated"`) || !strings.Contains(string(data), `"text_format":"markdown"`) || !strings.Contains(string(data), `"previewable":true`) || !strings.Contains(string(data), `"bcc_list":[{"name":"","address":"bcc@example.test"}]`) {
		t.Fatalf("draft detail: %s", data)
	}
	rows, count := list.GetEmailList(owner, dto.SearchTag{Type: 0, Status: 4, GroupId: -1}, "", false, 0, 15)
	if count != 1 || len(rows) != 1 || rows[0].Id != saved.ID {
		t.Fatalf("draft folder count=%d length=%d", count, len(rows))
	}
	if _, code, _ := callCompose(t, DeleteDraft, owner, `{"id":`+idText+`}`); code != 0 {
		t.Fatalf("draft delete: errorNo %d", code)
	}
	if exists, err := db.Instance.ID(saved.ID).Exist(new(models.Email)); err != nil || exists {
		t.Fatalf("orphan draft: %v %v", exists, err)
	}
}

func TestStarAndUnreadShareOwnerMailboxFlags(t *testing.T) {
	owner, other := composeFixture(t)
	mail := models.Email{Subject: "flag me"}
	if _, err := db.Instance.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	for _, u := range []*context.Context{owner, other} {
		if _, err := db.Instance.Insert(&models.UserEmail{UserID: u.UserID, EmailID: mail.Id, IsRead: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Instance.Insert(&models.UserEmail{UserID: owner.UserID, EmailID: mail.Id, IsRead: 1, GroupId: 11}); err != nil {
		t.Fatal(err)
	}
	id, _ := json.Marshal(mail.Id)
	if _, code, _ := callCompose(t, Star, owner, `{"ids":[`+string(id)+`],"starred":true}`); code != 0 {
		t.Fatalf("star: errorNo %d", code)
	}
	if _, code, _ := callCompose(t, MarkRead, owner, `{"ids":[`+string(id)+`],"is_read":false}`); code != 0 {
		t.Fatalf("unread: errorNo %d", code)
	}
	for _, tc := range []struct {
		ctx  *context.Context
		read int8
		star bool
	}{{owner, 0, true}, {other, 1, false}} {
		var row models.UserEmail
		if exists, err := db.Instance.Where("user_id=? AND email_id=?", tc.ctx.UserID, mail.Id).Get(&row); err != nil || !exists || row.IsRead != tc.read || row.IsStarred != tc.star {
			t.Fatalf("flags=%+v error=%v", row, err)
		}
	}
	rows, count := list.GetEmailList(owner, dto.SearchTag{Type: -1, Status: -1, GroupId: -1, Starred: true}, "", false, 0, 15)
	if count != 1 || len(rows) != 1 || !rows[0].IsStarred {
		t.Fatalf("starred folder: %d %v", count, rows)
	}
	if _, code, _ := callCompose(t, MarkRead, owner, `{"ids":[`+string(id)+`]}`); code != 0 {
		t.Fatalf("default seen: errorNo %d", code)
	}
	if _, code, _ := callCompose(t, Star, owner, `{"ids":[`+string(id)+`],"starred":false}`); code != 0 {
		t.Fatalf("clear star: errorNo %d", code)
	}
	var row models.UserEmail
	if exists, err := db.Instance.Where("user_id=? AND email_id=?", owner.UserID, mail.Id).Get(&row); err != nil || !exists || row.IsRead != 1 || row.IsStarred {
		t.Fatalf("clear/default flags=%+v error=%v", row, err)
	}
}

func TestDraftRemainsAfterFailureAndIsRemovedAfterAcceptedSend(t *testing.T) {
	owner, _ := composeFixture(t)
	_, code, data := callCompose(t, SaveDraft, owner, `{"subject":"saved","attrs":[{"name":"a.py","data":"data:text/plain;base64,eA=="}]}`)
	var saved struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(data, &saved); err != nil || code != 0 || saved.ID == 0 {
		t.Fatalf("save: %s, %d", data, code)
	}
	id, _ := json.Marshal(saved.ID)
	body := `{"draft_id":` + string(id) + `,"to":[{"email":"owner@example.test"}],"subject":"send draft","text":"content"}`
	failed := true
	send.SetLocalDelivery(func(_ *context.Context, email *parsemail.Email, _ []*parsemail.User) (error, map[string]error) {
		if len(email.Attachments) != 1 || email.Attachments[0].Filename != "a.py" {
			t.Error("saved draft attachment missing")
		}
		if failed {
			err := errors.New("fixture rejected message")
			return err, map[string]error{"owner@example.test": err}
		}
		return nil, map[string]error{"owner@example.test": nil}
	})
	t.Cleanup(func() { send.SetLocalDelivery(nil) })
	if _, code, _ := callCompose(t, Send, owner, body); code == 0 {
		t.Fatal("first send should fail")
	}
	if _, code, _ := callCompose(t, EmailDetail, owner, `{"id":`+string(id)+`}`); code != 0 {
		t.Fatal("failed send deleted draft")
	}
	failed = false
	if _, code, data := callCompose(t, Send, owner, body); code != 0 {
		t.Fatalf("accepted send error %d: %s", code, data)
	}
	if found, err := db.Instance.ID(saved.ID).Exist(new(models.Email)); found || err != nil {
		t.Fatalf("accepted draft remains: %v, %v", found, err)
	}
}

func TestSaveDraftTwiceKeepsIDAndUpdatesContent(t *testing.T) {
	owner, _ := composeFixture(t)
	type saved struct {
		ID int `json:"id"`
	}
	var first, second saved
	status, code, data := callCompose(t, SaveDraft, owner, `{"subject":"before", "text":"initial body"}`)
	if err := json.Unmarshal(data, &first); err != nil || status != 200 || code != 0 || first.ID <= 0 {
		t.Fatalf("initial save: HTTP %d, errorNo %d, data %s", status, code, data)
	}
	payload, err := json.Marshal(map[string]any{"id": first.ID, "subject": "after", "text": "revised body"})
	if err != nil {
		t.Fatal(err)
	}
	status, code, data = callCompose(t, SaveDraft, owner, string(payload))
	if err := json.Unmarshal(data, &second); err != nil || status != 200 || code != 0 || second.ID != first.ID {
		t.Fatalf("repeat save of %d: HTTP %d, errorNo %d, data %s", first.ID, status, code, data)
	}
	var row models.Email
	if found, err := db.Instance.ID(first.ID).Get(&row); err != nil || !found || row.Subject != "after" || row.Text.String != "revised body" || row.UpdateTime.IsZero() {
		t.Fatalf("stored draft not updated: id=%d, subject=%q, updated=%v, err=%v", row.Id, row.Subject, row.UpdateTime, err)
	}
}
