package smtp_server

import (
	stdctx "context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/utils/context"
	smtp "github.com/emersion/go-smtp"
)

func TestPendingAddressSMTPUsesCatchAllOrRejectsAndCannotSubmit(t *testing.T) {
	t.Setenv("PMAIL_CATCH_ALL", "true")
	e := newAuthenticationTestEngine(t)
	oldDB, oldCfg := db.Instance, config.Instance
	db.Instance, config.Instance = e, &config.Config{Domain: "approval-smtp.test", CatchAll: true}
	t.Cleanup(func() {
		db.Instance, config.Instance = oldDB, oldCfg
		if oldDB != nil {
			_ = domain.Refresh()
		}
	})
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	admin, member := models.User{Account: "admin", IsAdmin: 1}, models.User{Account: "member"}
	if _, err := e.Insert(&admin, &member); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	pending, err := address.Create(t.Context(), member.ID, domain.Default().ID, "pending", true, false)
	if err != nil {
		t.Fatal(err)
	}
	newSession := func(userID int) *Session {
		return &Session{Ctx: &context.Context{Context: t.Context(), UserID: userID, IsAdmin: userID == admin.ID}, From: "sender@else.test", AuthenticateInbound: func(stdctx.Context, netip.Addr, string, string, []byte) models.MailAuthResults {
			return models.MailAuthResults{}
		}}
	}
	for _, uid := range []int{member.ID, admin.ID} {
		var failure *smtp.SMTPError
		if err := newSession(uid).Mail(pending.Address, nil); !errors.As(err, &failure) || failure.Code != 550 || failure.EnhancedCode != (smtp.EnhancedCode{5, 7, 1}) {
			t.Fatalf("submission did not reject pending: %v", err)
		}
	}
	s := newSession(0)
	if err := s.Rcpt(pending.Address, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Data(strings.NewReader("From: sender@else.test\r\nTo: " + pending.Address + "\r\nSubject: Pending catch-all\r\n\r\nbody\r\n")); err != nil {
		t.Fatal(err)
	}
	var links []models.UserEmail
	if err := e.Find(&links); err != nil || len(links) != 1 || links[0].UserID != admin.ID || links[0].CatchAllAddresses.String != `["`+pending.Address+`"]` {
		t.Fatal("pending message did not go only into the catch-all inbox")
	}
	if n, err := e.Count(&models.UserEmailAddress{}); err != nil || n != 0 {
		t.Fatal("catch-all mail got an alias link")
	}
	counts, err := mailaddress.Unread(member.ID)
	if err != nil || len(counts.Addresses) != 1 || counts.Addresses[0].ID == pending.ID {
		t.Fatal("pending address appeared in switcher counts")
	}
	t.Setenv("PMAIL_CATCH_ALL", "false")
	var reject *smtp.SMTPError
	if err := newSession(0).Rcpt(pending.Address, nil); !errors.As(err, &reject) || reject.Code != 550 || reject.EnhancedCode != (smtp.EnhancedCode{5, 1, 1}) {
		t.Fatalf("pending recipient without catch-all: %v", err)
	}
	if _, err := address.Review(t.Context(), pending.ID, admin.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := newSession(0).Rcpt(pending.Address, nil); err != nil {
		t.Fatalf("approved recipient refused: %v", err)
	}
	if err := newSession(member.ID).Mail(pending.Address, nil); err != nil {
		t.Fatalf("approved sender refused: %v", err)
	}
}
