package attachments

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/testschema"
	pctx "github.com/Jinnrry/pmail/utils/context"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestBackfillAndStreamOnlyAuthorizedSelectedPart(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	defer e.Close()
	old := db.Instance
	db.Instance = e
	defer func() { db.Instance = old }()
	if err = testschema.Sync(e); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("fixture\n"), 150000)
	stored, err := json.Marshal([]parsemail.Attachment{{Filename: "sibling.bin", Content: []byte{0}}, {Filename: "large.txt", ContentType: "text/plain", Content: data, ContentID: "part-b"}})
	if err != nil {
		t.Fatal(err)
	}
	mail := models.Email{Attachments: string(stored)}
	if _, err = e.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Insert(&models.UserEmail{UserID: 2, EmailID: mail.Id}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = mailstore.Backfill(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err = mailstore.Backfill(ctx, e); err != nil {
		t.Fatal(err)
	}
	if a := Find(&pctx.Context{UserID: 1}, mail.Id, 1, ""); a != nil {
		t.Fatal("cross-owner attachment access")
	}
	a := Find(&pctx.Context{UserID: 2}, mail.Id, -1, "part-b")
	if a == nil || a.Index != 1 || a.ID == 0 || a.legacy != nil {
		t.Fatalf("metadata: %+v", a)
	}
	actual, err := io.ReadAll(a.Open(ctx))
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("stream bytes=%d err=%v", len(actual), err)
	}
	p, ok := a.Preview(ctx)
	if !ok || !p.Truncated || p.Size != len(data) || len(p.Content) > parsemail.PreviewLimit {
		t.Fatalf("preview=%v size=%d truncated=%v", ok, p.Size, p.Truncated)
	}
}
