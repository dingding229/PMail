package smtp_server

import (
	"net"
	"testing"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema/mailauth"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestSMTPSubmissionSecondFactorCredentialMatrix(t *testing.T) {
	e, _ := newAuditTestEngine(t)
	var user models.User
	if ok, err := e.ID(1).Get(&user); err != nil || !ok {
		t.Fatalf("fixture: %v", err)
	}
	mailauth.MailCredentialMatrix(t, user, func(email, secret string) bool {
		s := &Session{Ctx: &context.Context{}, RemoteAddress: &net.TCPAddr{IP: net.ParseIP("192.0.2.246"), Port: 2525}}
		if err := s.AuthPlain(email, secret); err != nil {
			return false
		}
		if s.Ctx.UserID != user.ID || s.validateSubmission() != nil {
			t.Fatal("invalid SMTP session")
		}
		return true
	})
}
