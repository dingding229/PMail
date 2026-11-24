package mailarchive

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/inbound"
	pctx "github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/send"
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
	a, b := models.User{Account: "a", Name: "A", Password: "fixture"}, models.User{Account: "b", Name: "B", Password: "fixture"}
	if _, err := e.Insert(&a, &b); err != nil {
		t.Fatal(err)
	}
	return a.ID, b.ID
}
func raw(subject, id, from, status string) []byte {
	return []byte("From: " + from + "\r\nTo: a@example.test\r\nSubject: " + subject + "\r\nMessage-ID: <" + id + ">\r\nStatus: " + status + "\r\nX-Status: F\r\n\r\nFirst\r\nFrom body\r\n>From another\r\nEnd\r\n")
}
func write(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMboxRDFlagsRoundTripAndDedup(t *testing.T) {
	a, b := fixture(t)
	f := builtins[0]
	var mbox bytes.Buffer
	for i, x := range []struct{ title, status string }{{"Alpha", "RO"}, {"Beta", "O"}} {
		r := raw(x.title, x.title+"@example.test", "outside@example.test", x.status)
		if err := writeMbox(&mbox, string(r), i == 0, true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(mbox.String(), "\n>From body\n>>From another\n") {
		t.Fatal("missing mboxrd escaping")
	}
	var messages [][]byte
	if err := readMbox(t.Context(), bytes.NewReader(mbox.Bytes()), func(b []byte) error { messages = append(messages, bytes.Clone(b)); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || !bytes.Contains(messages[0], []byte("\nFrom body\n>From another\n")) {
		t.Fatal("mboxrd unescaping or splitting failed")
	}
	path := write(t, "in.mbox", mbox.Bytes())
	var counts []string
	for range 2 {
		if err := Import(t.Context(), a, f, path, ".mbox", DefaultUpload*10, func(x string) { counts = append(counts, x) }); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(counts, ",") != "success,success,skipped,skipped" {
		t.Fatalf("unexpected outcomes: %v", counts)
	}
	var zipped bytes.Buffer
	if err := Export(t.Context(), a, []Folder{f}, &zipped, nil); err != nil {
		t.Fatal(err)
	}
	if err := Import(t.Context(), b, f, write(t, "out.zip", zipped.Bytes()), ".zip", DefaultUpload*10, nil); err != nil {
		t.Fatal(err)
	}
	var states []struct {
		Subject       string
		IsRead        int
		IsStarred     bool
		AuthUnchecked bool
	}
	if err := db.Instance.SQL(`SELECT e.subject,ue.is_read,ue.is_starred,e.auth_unchecked FROM email e JOIN user_email ue ON ue.email_id=e.id WHERE ue.user_id=? ORDER BY e.subject`, b).Find(&states); err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0].Subject != "Alpha" || states[0].IsRead != 1 || !states[0].IsStarred || !states[0].AuthUnchecked || states[1].Subject != "Beta" || states[1].IsRead != 0 {
		t.Fatalf("round trip flags: %+v", states)
	}
}

func makeZip(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestZIPTraversalAndExpansionRejectedBeforeWriting(t *testing.T) {
	a, _ := fixture(t)
	for _, tc := range []struct {
		name   string
		bytes  []byte
		budget int64
		want   error
	}{
		{"../x.eml", raw("x", "a@test", "a@test", "O"), 2000, ErrInvalid},
		{"C:\\escape.eml", []byte("x"), 2000, ErrInvalid},
		{"large.eml", bytes.Repeat([]byte("a"), 65536), 8192, ErrTooLarge},
	} {
		p := write(t, "mail.zip", makeZip(t, tc.name, tc.bytes))
		err := Import(t.Context(), a, builtins[0], p, ".zip", tc.budget, nil)
		if !errors.Is(err, tc.want) && !(tc.want == ErrInvalid && errors.Is(err, zip.ErrInsecurePath)) {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	if c, err := db.Instance.Count(&models.Email{}); c != 0 || err != nil {
		t.Fatalf("zip failure wrote mail: %d %v", c, err)
	}
}

func TestImportDoesNotDeliverOrRunMailboxRules(t *testing.T) {
	a, _ := fixture(t)
	var notify, deliver atomic.Int32
	inbound.SetNotifier(func(int, int, *inbound.Analysis) { notify.Add(1) })
	t.Cleanup(func() { inbound.SetNotifier(nil) })
	send.SetLocalDelivery(func(*pctx.Context, *parsemail.Email, []*parsemail.User) (map[string]error, error) {
		deliver.Add(1)
		return nil, nil
	})
	t.Cleanup(func() { send.SetLocalDelivery(nil) })
	r := dto.Rule{UserId: a, Name: "fixture delete if inbox engine is used", Rules: []*dto.Value{{Field: "Subject", Type: "equal", Rule: "Do not deliver"}}, Action: dto.DELETE}
	if _, err := db.Instance.Insert(r.Encode(), &models.ForwardSetting{UserID: a, Targets: `["out@example.test"]`, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	p := write(t, "mail.eml", raw("Do not deliver", "sideeffects@example.test", "from@example.test", "O"))
	if e := Import(t.Context(), a, builtins[0], p, ".eml", DefaultUpload*10, nil); e != nil {
		t.Fatal(e)
	}
	if notify.Load() != 0 || deliver.Load() != 0 {
		t.Fatal("import invoked delivery/Telegram hooks")
	}
	if n, err := db.Instance.Count(&models.OutboundQueue{}); err != nil || n != 0 {
		t.Fatalf("triggered forwarding: %d %v", n, err)
	}
	var link models.UserEmail
	if ok, err := db.Instance.Where("user_id=?", a).Get(&link); err != nil || !ok || link.Status != 0 || link.IsRead != 0 {
		t.Fatalf("rule was run or wrong flags: %v", err)
	}
}

func TestExportJobsOwnerIsolationAndExpiration(t *testing.T) {
	a, b := fixture(t)
	root := t.TempDir()
	var m Manager
	j, e := m.StartExport(root, a, []Folder{builtins[0]})
	if e != nil {
		t.Fatal(e)
	}
	for range 300 {
		got, err := m.Status(root, a, j.ID, "export")
		if err != nil {
			t.Fatal(err)
		}
		if got.State == "failed" {
			t.Fatalf("export: %s", got.Error)
		}
		if got.State == "done" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := m.OpenExport(root, b, j.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign download: %v", err)
	}
	var fresh Manager
	file, err := fresh.OpenExport(root, a, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(file)
	file.Close()
	if _, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatal(err)
	}
	m.Cleanup(root, time.Now().Add(25*time.Hour))
	if _, err := fresh.OpenExport(root, a, j.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired export: %v", err)
	}
}

func TestImportedSentClassificationAndQuota(t *testing.T) {
	a, _ := fixture(t)
	if _, err := db.Instance.Insert(&models.Domain{ID: 1, Name: "example.test", IsDefault: true}, &models.MailboxAddress{UserID: a, DomainID: 1, LocalPart: "a", Status: models.AddressActive}); err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		folder   Folder
		id, from string
		want     int8
	}{{builtins[0], "in@example.test", "a@example.test", 0}, {builtins[1], "sent@example.test", "a@example.test", 1}, {builtins[1], "foreign@example.test", "b@example.test", 0}} {
		state, err := importMessage(t.Context(), a, x.folder, raw("fixture", x.id, x.from, "O"))
		if state != "success" || err != nil {
			t.Fatalf("import: %s %v", state, err)
		}
		var e models.Email
		if ok, err := db.Instance.Where("msg_id=?", x.id).Get(&e); err != nil || !ok || e.Type != x.want {
			t.Fatalf("wrong type for %s: %d %v", x.id, e.Type, err)
		}
	}
	if _, err := db.Instance.Table(&models.User{}).ID(a).Update(map[string]any{"quota_mb": 1, "used_bytes": int64(1 << 20)}); err != nil {
		t.Fatal(err)
	}
	_, err := importMessage(context.Background(), a, builtins[0], raw("over quota", "quota@example.test", "b@example.test", "O"))
	if !errors.Is(err, ErrQuota) {
		t.Fatalf("quota: %v", err)
	}
}
