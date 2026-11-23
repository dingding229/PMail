// Package mailauth provides test fixtures for mail protocol authentication.
package mailauth

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/password"
)

// MailCredentialMatrix exercises protocol-specific authentication with a real
// email address and both kinds of credentials. TOTP setup itself is tested by auth.
func MailCredentialMatrix(t *testing.T, u models.User, authenticate func(email, secret string) bool) {
	t.Helper()
	if err := domain.Initialize(&config.Config{Domain: "mail-credentials.test"}); err != nil {
		t.Fatal(err)
	}
	d := domain.Default()
	mailbox := models.MailboxAddress{UserID: u.ID, DomainID: d.ID, LocalPart: "client", IsPrimary: true, CanSend: true}
	if _, err := db.Instance.Insert(&mailbox); err != nil {
		t.Fatal(err)
	}
	loginPassword := rand.Text()
	hash, err := password.Hash(loginPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Instance.ID(u.ID).Cols("password").Update(&models.User{Password: hash}); err != nil {
		t.Fatal(err)
	}
	app, err := auth.CreateAppPassword(context.Background(), u, "mail fixture")
	if err != nil {
		t.Fatal(err)
	}

	for _, factor := range []bool{false, true} {
		if factor {
			if _, err := db.Instance.ID(u.ID).Cols("totp_enabled", "factor_version").Update(&models.User{TOTPEnabled: true, FactorVersion: 1}); err != nil {
				t.Fatal(err)
			}
		}
		for _, tc := range []struct {
			name, secret string
			want         bool
		}{
			{"login", loginPassword, !factor}, {"app", app.Password, true},
		} {
			if got := authenticate("client@"+d.Name, tc.secret); got != tc.want {
				t.Fatalf("2FA=%t %s password accepted=%t, want %t", factor, tc.name, got, tc.want)
			}
		}
	}
}
