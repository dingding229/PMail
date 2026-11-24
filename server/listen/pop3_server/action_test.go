package pop3_server

import (
	"bufio"
	stdcontext "context"
	"database/sql"
	"fmt"
	"net"
	stdmail "net/mail"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailevents"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/testschema"
	pctx "github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/password"
	"xorm.io/xorm"
)

const fixturePassword = "pop3-fixture-password"

func popFixture(t *testing.T) models.User {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	engine.SetMaxOpenConns(1)
	old := db.Instance
	db.Instance = engine
	t.Cleanup(func() { db.Instance = old; _ = engine.Close() })
	if err := testschema.Sync(engine); err != nil {
		t.Fatal(err)
	}
	hash, err := password.Hash(fixturePassword)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately test an admin: POP3 must not use Web's admin read override.
	owner := models.User{Account: "popowner", Name: "POP owner", Password: hash, IsAdmin: 1}
	other := models.User{Account: "other", Name: "Other", Password: hash}
	for _, u := range []*models.User{&owner, &other} {
		if _, err := engine.Insert(u); err != nil {
			t.Fatal(err)
		}
	}
	addMessage(t, owner.ID, 3, "older owned")
	addMessage(t, owner.ID, 8, "newer owned")
	addMessage(t, other.ID, 1, "foreign private")
	// IMAP can keep another link to an Inbox message in a custom folder.
	if _, err := engine.Insert(&models.UserEmail{UserID: owner.ID, EmailID: 8, GroupId: 22}); err != nil {
		t.Fatal(err)
	}
	return owner
}

func addMessage(t *testing.T, owner, id int, subject string) {
	t.Helper()
	mail := models.Email{Id: id, FromAddress: "sender@example.test", Subject: subject, MsgID: fmt.Sprintf("%d@example.test", id), SendDate: time.Date(2026, 11, 22, 15, 17, 0, 0, time.UTC), Text: sql.NullString{String: "body", Valid: true}}
	if _, err := db.Instance.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Instance.Insert(&models.UserEmail{UserID: owner, EmailID: id}); err != nil {
		t.Fatal(err)
	}
}

func TestMaildropSequenceSnapshotAndStableUIDL(t *testing.T) {
	owner := popFixture(t)
	d, err := openMaildrop(owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	addMessage(t, owner.ID, 20, "delivered after login")
	list, err := d.list(0, false)
	if err != nil || len(list) != 2 || !strings.HasPrefix(list[0], "1 ") || !strings.HasPrefix(list[1], "2 ") {
		t.Fatalf("LIST: %v, %v", list, err)
	}
	raw, err := d.retr(1)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := stdmail.ReadMessage(strings.NewReader(raw))
	if err != nil || parsed.Header.Get("Subject") != "newer owned" {
		t.Fatalf("RETR 1 selected wrong mailbox entry: %v", err)
	}
	if list[0] != "1 "+strconv.Itoa(len(raw)) {
		t.Fatalf("LIST size %q, RETR octets %d", list[0], len(raw))
	}
	if _, err := d.retr(3); err != errNoSuchMessage {
		t.Fatalf("out-of-range RETR = %v", err)
	}
	uid, err := d.list(0, true)
	if err != nil || strings.Join(uid, ",") != "1 8,2 3" {
		t.Fatalf("UIDL: %v, %v", uid, err)
	}
	if err := d.mark(1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.retr(1); err != errNoSuchMessage {
		t.Fatalf("marked RETR = %v", err)
	}
	if n, _, err := d.stat(); err != nil || n != 1 {
		t.Fatalf("marked STAT = %d, %v", n, err)
	}
	d.reset()
	if uid, err := d.list(1, true); err != nil || strings.Join(uid, ",") != "1 8" {
		t.Fatalf("RSET UIDL: %v, %v", uid, err)
	}
	// Remove the ownership link while retaining the underlying email.
	if _, err := db.Instance.Where("email_id=? AND user_id=?", 8, owner.ID).Delete(new(models.UserEmail)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.retr(1); err != errNoSuchMessage {
		t.Fatalf("revoked message link remained readable: %v", err)
	}
}

func TestPOP3WireSequenceAndUpdate(t *testing.T) {
	owner := popFixture(t)
	server, client := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer server.Close()
		serveConnection(server, "mail.example.test", true, nil)
	}()
	reader := bufio.NewReader(client)
	read := func() string {
		t.Helper()
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSuffix(line, "\r\n")
	}
	command := func(msg, want string) {
		t.Helper()
		if _, err := fmt.Fprintf(client, "%s\r\n", msg); err != nil {
			t.Fatal(err)
		}
		if line := read(); !strings.HasPrefix(line, want) {
			t.Fatalf("POP3 %s: %q, want prefix %q", strings.Fields(msg)[0], line, want)
		}
	}
	if got := read(); !strings.HasPrefix(got, "+OK") {
		t.Fatal(got)
	}
	command("NOOP", "-ERR sign in first")
	command("USER "+owner.Account, "+OK")
	command("PASS "+fixturePassword, "+OK")
	command("STAT", "+OK 2 ")
	command("UIDL 1", "+OK 1 8")
	command("UIDL", "+OK")
	if got := []string{read(), read(), read()}; strings.Join(got, ",") != "1 8,2 3,." {
		t.Fatalf("UIDL form: %v", got)
	}
	command("DELE 1", "+OK")
	command("LIST 1", "-ERR no such message")
	command("RSET", "+OK")
	command("UIDL 1", "+OK 1 8")
	command("RETR 99", "-ERR no such message")
	command("TOP 1 0", "+OK")
	for read() != "." {
	} // header block, including blank separator
	command("DELE 1", "+OK")
	var link models.UserEmail
	if _, err := db.Instance.Where("email_id=? AND user_id=? AND group_id=0", 8, owner.ID).Get(&link); err != nil || link.Status != consts.EmailStatusWait {
		t.Fatalf("DELE committed before QUIT: %+v, %v", link, err)
	}
	command("QUIT", "+OK")
	<-done
	var trashed, filed models.UserEmail
	if found, err := db.Instance.ID(link.ID).Get(&trashed); err != nil || !found || trashed.Status != consts.EmailStatusDel {
		t.Fatalf("QUIT did not apply mark: %+v, %v", trashed, err)
	}
	if trashed.DeletedAt == nil || trashed.PrevStatus == nil || *trashed.PrevStatus != consts.EmailStatusWait || trashed.PrevGroupID == nil || *trashed.PrevGroupID != 0 {
		t.Fatalf("QUIT did not preserve the original placement: %+v", trashed)
	}
	if found, err := db.Instance.Where("email_id=? AND user_id=? AND group_id=22", 8, owner.ID).Get(&filed); err != nil || !found || filed.Status != consts.EmailStatusWait {
		t.Fatalf("QUIT removed a link from a different folder: %+v, %v", filed, err)
	}
	fresh, err := openMaildrop(owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.close()
	if uid, err := fresh.list(0, true); err != nil || strings.Join(uid, ",") != "1 3" {
		t.Fatalf("relogin changed permanent UIDL or retained deleted mail: %v, %v", uid, err)
	}
}

func TestPOP3DisconnectDoesNotCommit(t *testing.T) {
	owner := popFixture(t)
	server, client := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer server.Close()
		serveConnection(server, "mail.example.test", true, nil)
	}()
	reader := bufio.NewReader(client)
	for _, command := range []string{"", "USER " + owner.Account, "PASS " + fixturePassword, "DELE 1"} {
		if command != "" {
			if _, err := fmt.Fprintf(client, "%s\r\n", command); err != nil {
				t.Fatal(err)
			}
		}
		if line, err := reader.ReadString('\n'); err != nil || !strings.HasPrefix(line, "+OK") {
			t.Fatalf("unexpected POP3 response %q, %v", line, err)
		}
	}
	_ = client.Close()
	<-done
	drop, err := openMaildrop(owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer drop.close()
	if uid, err := drop.list(0, true); err != nil || strings.Join(uid, ",") != "1 8,2 3" {
		t.Fatalf("disconnect committed DELE: %v, %v", uid, err)
	}
}

func TestCachedPOPOctetsMatchRETR(t *testing.T) {
	owner := popFixture(t)
	var row models.Email
	if _, err := db.Instance.ID(8).Get(&row); err != nil {
		t.Fatal(err)
	}
	mailstore.SetPOPSize(&pctx.Context{Context: stdcontext.Background()}, &row)
	if row.POPOctets <= 0 {
		t.Fatal("octet count not computed")
	}
	if _, err := db.Instance.ID(8).Cols("pop_octets").Update(&row); err != nil {
		t.Fatal(err)
	}
	d, err := openMaildrop(owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	if d.messages[0].size != row.POPOctets {
		t.Fatal("snapshot did not use cached octets")
	}
	raw, err := d.retr(1)
	if err != nil || int64(len(raw)) != row.POPOctets {
		t.Fatalf("RETR bytes=%d %v", len(raw), err)
	}
}

func TestPOP3DeletionPublishesOnCommit(t *testing.T) {
	owner := popFixture(t)
	d, err := openMaildrop(owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	before := mailevents.Default.Version(owner.ID)
	if err := d.mark(1); err != nil {
		t.Fatal(err)
	}
	if err := d.mark(2); err != nil {
		t.Fatal(err)
	}
	if mailevents.Default.Version(owner.ID) != before {
		t.Fatal("DELE published before QUIT")
	}
	if err := d.commit(); err != nil {
		t.Fatal(err)
	}
	if mailevents.Default.Version(owner.ID) != before+1 {
		t.Fatal("POP3 QUIT didn't publish exactly once")
	}
}
