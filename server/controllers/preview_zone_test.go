package controllers

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/context"
)

func c3Fixture(t *testing.T) *context.Context {
	t.Helper()
	cfg := &config.Config{DbType: config.DBTypeSQLite, DbDSN: filepath.Join(t.TempDir(), "mail.db"), Domain: "example.test", WebDomain: "mail.example.test"}
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
	return &context.Context{Context: stdctx.Background(), UserID: 17, Lang: "zh-CN"}
}

func TestAttachmentPreviewAccessAndUnsupported(t *testing.T) {
	ctx := c3Fixture(t)
	data, _ := json.Marshal([]parsemail.Attachment{{Filename: "script.py", ContentType: "text/plain", Content: []byte("print(7)")}, {Filename: "binary.txt", ContentType: "text/plain", Content: []byte{'x', 0, 'y'}}})
	mail := models.Email{Subject: "preview", Attachments: string(data)}
	if _, err := db.Instance.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Instance.Insert(&models.UserEmail{UserID: ctx.UserID, EmailID: mail.Id}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		owner, index, status int
		contains             string
	}{{17, 0, 200, `"language":"python"`}, {17, 1, 415, "不是受支持的文本"}, {18, 0, 404, "找不到附件"}} {
		r := httptest.NewRequest(http.MethodGet, "/api/attachments/preview", nil)
		r.SetPathValue("id", fmt.Sprintf("%d.%d", mail.Id, tc.index))
		w := httptest.NewRecorder()
		actor := *ctx
		actor.UserID = tc.owner
		PreviewAttachment(&actor, w, r)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("preview status=%d body=%s", w.Code, w.Body.String())
		}
	}
}

func TestZoneDownloadRequiresAdminAndUsesBindText(t *testing.T) {
	ctx := c3Fixture(t)
	t.Setenv("PMAIL_PUBLIC_IPV4", "203.0.113.17")
	t.Setenv("PMAIL_PUBLIC_IPV6", "2001:db8::17")
	t.Setenv("PMAIL_WEB_MODE", "public")
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/domain/dns/zone?id=%d", domain.Default().ID), nil)
	for _, admin := range []bool{false, true} {
		w := httptest.NewRecorder()
		ctx.IsAdmin = admin
		DomainZone(ctx, w, r)
		if !admin {
			if w.Code != 403 {
				t.Fatalf("nonadmin zone status=%d", w.Code)
			}
			continue
		}
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "example.test.zone") || !strings.Contains(w.Body.String(), "smtp 3600 IN A 203.0.113.17") {
			t.Fatalf("zone status=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
		}
	}
}
