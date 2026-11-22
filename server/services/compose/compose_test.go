package compose

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	pctx "github.com/Jinnrry/pmail/utils/context"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func fixture(t *testing.T) (int, int) {
	t.Helper()
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	old := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = old; e.Close() })
	if err = db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	a, b := models.User{Account: "compose-a", Password: "fixture"}, models.User{Account: "compose-b", Password: "fixture"}
	if _, err = e.Insert(&a, &b); err != nil {
		t.Fatal(err)
	}
	return a.ID, b.ID
}
func TestInlineOwnershipDraftRetentionRelatedAndCleanup(t *testing.T) {
	uid, other := fixture(t)
	ctx := context.Background()
	blob := []byte("\x89PNG\r\n\x1a\nfixture")
	if _, err := Upload(ctx, uid, "false.png", strings.NewReader("<html>")); !errors.Is(err, ErrType) {
		t.Fatal(err)
	}
	png, err := Upload(ctx, uid, "../image.png", bytes.NewReader(blob))
	if err != nil || png.Filename != "image.png" {
		t.Fatalf("upload %+v %v", png, err)
	}
	if _, err = Preview(ctx, other, png.CID, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign preview")
	}
	raw := `<p>hello<img src="cid:` + png.CID + `"></p>`
	refs := []Ref{{CID: png.CID}}
	e := &parsemail.Email{From: &parsemail.User{EmailAddress: "sender@example.test"}, MsgID: "fixture@example.test", HTML: []byte(raw)}
	if err = Apply(ctx, other, e, refs, nil); !errors.Is(err, ErrRef) {
		t.Fatal("foreign CID resolved")
	}
	if err = Apply(ctx, uid, e, refs, nil); err != nil {
		t.Fatal(err)
	}
	wire, err := e.BuildBytes(&pctx.Context{Context: ctx}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte("multipart/related")) || !bytes.Contains(wire, []byte("multipart/alternative")) || !bytes.Contains(wire, []byte("Content-Id: <"+png.CID+">")) {
		t.Fatalf("missing structured CID: %s", wire)
	}
	parsed, err := parsemail.ParseLimited(nil, bytes.NewReader(wire), len(wire))
	if err != nil || len(parsed.Attachments) != 1 || !bytes.Equal(parsed.Attachments[0].Content, blob) || !bytes.Contains(parsed.Text, []byte("hello")) {
		t.Fatalf("roundtrip %+v %v", parsed, err)
	}
	if _, err = db.Instance.Insert(&models.Email{Id: 31, Status: 4, SendUserID: uid}); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(db.Instance, func(tx *xorm.Session) error { return SyncDraft(tx, uid, 31, raw, refs) }); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(25 * time.Hour)
	if err = Cleanup(ctx, future); err != nil {
		t.Fatal(err)
	}
	if _, err = Preview(ctx, uid, png.CID, false); err != nil {
		t.Fatal("referenced image purged", err)
	}
	if err = db.Transaction(db.Instance, func(tx *xorm.Session) error { return DeleteDraft(tx, 31) }); err != nil {
		t.Fatal(err)
	}
	if err = Cleanup(ctx, future); err != nil {
		t.Fatal(err)
	}
	if _, err = Preview(ctx, uid, png.CID, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("stale image not purged")
	}
}
func TestSignatureSanitizationOwnershipAndSelection(t *testing.T) {
	uid, other := fixture(t)
	ctx := context.Background()
	address := models.MailboxAddress{UserID: uid, LocalPart: "sender", DomainID: 1}
	if _, err := db.Instance.Insert(&address); err != nil {
		t.Fatal(err)
	}
	s := Signature{Name: "Example", HTML: `<p style="color: red; background-image: url(https://no.test/a)"><b>Nice</b><img src="https://no.test/a" alt="Logo"><script>hidden</script><a href="javascript:go()">x</a></p>`, Default: true, Addresses: []int{address.ID}}
	if _, err := SaveSignature(ctx, other, 0, s); !errors.Is(err, ErrSignature) {
		t.Fatal("foreign address accepted")
	}
	rec, err := SaveSignature(ctx, uid, 0, s)
	if err != nil || !strings.Contains(rec.HTML, "Logo") || !strings.Contains(rec.HTML, "<b>Nice</b>") || strings.Contains(rec.HTML, "no.test") || strings.Contains(rec.HTML, "javascript") || strings.Contains(rec.HTML, "hidden") {
		t.Fatalf("unsafe or erased HTML %+v %v", rec, err)
	}
	if _, err = SaveSignature(ctx, other, rec.ID, Signature{Name: "foreign"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign update accepted")
	}
	items, err := Signatures(ctx, uid)
	if err != nil || SelectSignature(items, address.ID) == nil || SelectSignature(items, 999) == nil {
		t.Fatalf("signature selection %v %v", items, err)
	}
}
