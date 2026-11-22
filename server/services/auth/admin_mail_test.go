package auth

import (
	"testing"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestC7AdminMailOverrideRequiresExplicitOptIn(t *testing.T) {
	admin := &context.Context{UserID: 4, IsAdmin: true}
	mail := &models.Email{Id: 17}
	t.Setenv("PMAIL_ADMIN_MAIL_ACCESS", "")
	if AdminMailOverride(admin, mail) {
		t.Fatal("admin access defaulted on")
	}
	t.Setenv("PMAIL_ADMIN_MAIL_ACCESS", "1")
	if !AdminMailOverride(admin, mail) || AdminMailOverride(&context.Context{UserID: 4}, mail) {
		t.Fatal("explicit override role check failed")
	}
	mail.Status = consts.EmailStatusDrafts
	if AdminMailOverride(admin, mail) {
		t.Fatal("admin accessed draft")
	}
}
