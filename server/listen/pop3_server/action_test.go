package pop3_server

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	stdmail "net/mail"
	"strings"
	"testing"

	"github.com/Jinnrry/gopop"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/emersion/go-message/mail"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func Test_action_Retr(t *testing.T) {
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
	user := &models.User{Account: "pop3fixture"}
	message := &models.Email{
		FromAddress: "sender@example.com", Subject: "POP3 fixture", MsgID: "pop3@example.com",
		Text: sql.NullString{String: "fixture body", Valid: true},
	}
	for _, row := range []any{user, message} {
		if _, err := engine.Insert(row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := engine.Insert(&models.UserEmail{UserID: user.ID, EmailID: message.Id}); err != nil {
		t.Fatal(err)
	}

	session := &gopop.Session{Ctx: &context.Context{UserID: user.ID, AuthVersion: user.AuthVersion}}
	raw, size, err := (action{}).Retr(session, int64(message.Id))
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(raw)) {
		t.Fatalf("RETR size = %d, want %d", size, len(raw))
	}
	parsed, err := stdmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if subject := parsed.Header.Get("Subject"); subject != message.Subject {
		t.Fatalf("RETR subject = %q, want %q", subject, message.Subject)
	}
}

func Test_email(t *testing.T) {
	var b bytes.Buffer

	// Create our mail header
	var h mail.Header

	// Create a new mail writer
	mw, _ := mail.CreateWriter(&b, h)

	// Create a text part
	tw, _ := mw.CreateInline()

	var html mail.InlineHeader

	html.Header.Set("Content-Transfer-Encoding", "base64")
	w, _ := tw.CreatePart(html)

	io.WriteString(w, "=")

	w.Close()

	tw.Close()

	fmt.Printf("%s", b.String())

}
