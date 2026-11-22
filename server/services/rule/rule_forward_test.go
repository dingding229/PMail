package rule

import (
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/testschema"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/dkimkey"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestForwardToLocalUser(t *testing.T) {
	oldConfig := config.Instance
	oldDB := db.Instance
	defer func() {
		config.Instance = oldConfig
		db.Instance = oldDB
	}()

	config.Instance = &config.Config{
		Domain:  "example.com",
		Domains: []string{"example.com"},
	}

	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	db.Instance = engine

	if err := testschema.Sync(engine); err != nil {
		t.Fatal(err)
	}

	sender := &models.User{ID: 1, Account: "sender", Name: "sender"}
	recipient := &models.User{ID: 2, Account: "recipient", Name: "recipient", IsAdmin: 1}
	key, err := dkimkey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Insert(&models.Domain{ID: 1, Name: "example.com", IsDefault: true, DkimSelector: "default", DkimPrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(sender, recipient); err != nil {
		t.Fatal(err)
	}
	if err := domain.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}

	if _, err := engine.Insert(&models.Email{Id: 123, Subject: "forwarded fixture", Size: 10}); err != nil {
		t.Fatal(err)
	}

	email := &parsemail.Email{
		MessageId: 123,
		Status:    0,
		From:      &parsemail.User{EmailAddress: "sender@example.com"},
		RawData:   []byte("From: sender@example.com\r\nTo: recipient@example.com\r\n\r\nbody"),
	}
	if err := doForward(&context.Context{}, email, "recipient@example.com", sender, nil); err != nil {
		t.Fatal(err)
	}

	var links []models.UserEmail
	if err := engine.Table(&models.UserEmail{}).Where("email_id=?", email.MessageId).Find(&links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].UserID != recipient.ID {
		t.Fatalf("expected one user_email link for recipient, got %+v", links)
	}

	if err := doForward(&context.Context{}, email, "recipient@example.com", sender, nil); err != nil {
		t.Fatal(err)
	}
	var count int64
	count, err = engine.Table(&models.UserEmail{}).Where("email_id=? and user_id=?", email.MessageId, recipient.ID).Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected idempotent local forward link, got %d links", count)
	}
}

func TestForwardToLocalUserRejectsSelfLoop(t *testing.T) {
	oldConfig := config.Instance
	oldDB := db.Instance
	defer func() { config.Instance = oldConfig; db.Instance = oldDB }()
	config.Instance = &config.Config{Domain: "example.com", Domains: []string{"example.com"}}
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := testschema.Sync(engine); err != nil {
		t.Fatal(err)
	}
	db.Instance = engine
	key, err := dkimkey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&models.Domain{ID: 1, Name: "example.com", IsDefault: true, DkimSelector: "default", DkimPrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&models.User{ID: 1, Account: "sender", Name: "sender"}); err != nil {
		t.Fatal(err)
	}
	if err := domain.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	err = doForward(&context.Context{}, &parsemail.Email{MessageId: 1, From: &parsemail.User{EmailAddress: "sender@example.com"}}, "sender@example.com", &models.User{ID: 1, Account: "sender"}, []byte("From: sender@example.com\r\n\r\nbody"))
	if err == nil || !strings.Contains(err.Error(), "loop forwarding to self") {
		t.Fatalf("expected self-loop error, got %v", err)
	}
}
