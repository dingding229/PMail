package imap_server

import (
	"testing"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema/mailauth"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestIMAPSecondFactorCredentialMatrix(t *testing.T) {
	e, fixture, _ := imapFixture(t)
	var user models.User
	if ok, err := e.ID(fixture.ctx.UserID).Get(&user); err != nil || !ok {
		t.Fatalf("fixture: %v", err)
	}
	mailauth.MailCredentialMatrix(t, user, func(email, secret string) bool {
		s := &serverSession{ctx: &context.Context{}, remoteAddr: "192.0.2.245:11000"}
		if err := s.Login(email, secret); err != nil {
			return false
		}
		if s.ctx.UserID != user.ID || s.requireSession() != nil {
			t.Fatal("invalid IMAP session")
		}
		return true
	})
}
