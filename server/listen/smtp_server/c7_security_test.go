package smtp_server

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/emersion/go-smtp"
)

func TestC7SMTPRejectsRevokedSubmission(t *testing.T) {
	engine := newAuthenticationTestEngine(t)
	old := db.Instance
	db.Instance = engine
	t.Cleanup(func() { db.Instance = old })
	user := models.User{Account: "smtp-user", Name: "SMTP user", AuthVersion: 10, IsAdmin: 1, Password: "dummy-test-hash"}
	if _, err := engine.Insert(&user); err != nil {
		t.Fatal(err)
	}
	s := &Session{Ctx: &context.Context{UserID: user.ID, AuthVersion: 10, IsAdmin: true}}
	if err := s.Mail("user@example.test", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ID(user.ID).Cols("is_admin").Update(&models.User{IsAdmin: 0}); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{s.Mail("user@example.test", nil), s.Data(strings.NewReader(""))} {
		var protocol *smtp.SMTPError
		if !errors.As(err, &protocol) || protocol.Code != 535 {
			t.Fatalf("revoked SMTP session error: %v", err)
		}
	}
}
