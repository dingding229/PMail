package smtp_server

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/utils/context"
	smtp "github.com/emersion/go-smtp"
)

func TestCatchAllSingleDeliveryAndEnvelopeAddresses(t *testing.T) {
	t.Setenv("PMAIL_CATCH_ALL", "")
	e := newAuthenticationTestEngine(t)
	oldDB, oldCfg := db.Instance, config.Instance
	db.Instance, config.Instance = e, &config.Config{Domain: "example.test", CatchAll: true}
	t.Cleanup(func() {
		db.Instance, config.Instance = oldDB, oldCfg
		if oldDB != nil {
			_ = domain.Refresh()
		}
	})
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	a := models.User{Account: "first", Password: "fixture", IsAdmin: 1}
	b := models.User{Account: "second", Password: "fixture", IsAdmin: 1}
	owner := models.User{Account: "chosen", Password: "fixture"}
	if _, err := e.Insert(&a, &b, &owner); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	ctx := &context.Context{Context: t.Context()}
	newSession := func() *Session {
		return &Session{Ctx: ctx, From: "sender@outside.test", AuthenticateInbound: func(stdctx.Context, netip.Addr, string, string, []byte) models.MailAuthResults {
			return models.MailAuthResults{}
		}}
	}
	unknown := []string{"Odd@EXAMPLE.test", "blind@example.test", "odd@example.test"}
	deliver := func(subject string, expected int, recipients ...string) {
		t.Helper()
		s := newSession()
		for _, rcpt := range recipients {
			if err := s.Rcpt(rcpt, nil); err != nil {
				t.Fatal(err)
			}
		}
		wire := "From: sender@outside.test\r\nTo: unrelated@outside.test\r\nSubject: " + subject + "\r\n\r\nA message\r\n"
		if err := s.Data(strings.NewReader(wire)); err != nil {
			t.Fatal(err)
		}
		var mail models.Email
		if ok, err := e.Where("subject = ?", subject).Get(&mail); err != nil || !ok {
			t.Fatalf("mail exists=%t err=%v", ok, err)
		}
		var links []models.UserEmail
		if err := e.Where("email_id = ?", mail.Id).Find(&links); err != nil || len(links) != 1 || links[0].UserID != expected {
			t.Fatalf("want exactly one link for %d: %+v %v", expected, links, err)
		}
		var actual []string
		if err := json.Unmarshal([]byte(links[0].CatchAllAddresses.String), &actual); err != nil || !links[0].CatchAllAddresses.Valid || !slices.Equal(actual, []string{"odd@example.test", "blind@example.test"}) {
			t.Fatalf("unexpected catch-all envelope addresses %v %v", actual, err)
		}
	}
	assertRejection := func(err error, enhanced smtp.EnhancedCode) {
		t.Helper()
		var smtpErr *smtp.SMTPError
		if !errors.As(err, &smtpErr) || smtpErr.Code != 550 || smtpErr.EnhancedCode != enhanced {
			t.Fatalf("want 550 %v, got %v", enhanced, err)
		}
	}
	assertRejection(newSession().Rcpt("odd@outside.test", nil), smtp.EnhancedCode{5, 7, 1})
	deliver("default one inbox", a.ID, append(slices.Clone(unknown), "first@example.test")...)
	settings := inbound.Settings{DefaultSpamThreshold: 6, Zones: []string{}}
	if _, err := inbound.PutSettings(t.Context(), settings, inbound.CatchAllUpdate{RecipientID: &owner.ID}); err != nil {
		t.Fatal(err)
	}
	deliver("configured user", owner.ID, unknown...)
	if _, err := e.ID(owner.ID).Cols("disabled").Update(&models.User{Disabled: 1}); err != nil {
		t.Fatal(err)
	}
	deliver("disabled user fallback", a.ID, unknown...)
	if _, err := e.ID(owner.ID).Delete(&models.User{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ID(a.ID).Cols("disabled").Update(&models.User{Disabled: 1}); err != nil {
		t.Fatal(err)
	}
	deliver("deleted user and disabled administrator fallback", b.ID, unknown...)
	off := false
	if _, err := inbound.PutSettings(t.Context(), settings, inbound.CatchAllUpdate{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	assertRejection(newSession().Rcpt(unknown[0], nil), smtp.EnhancedCode{5, 1, 1})
	if err := newSession().Rcpt("second@example.test", nil); err != nil {
		t.Fatalf("known recipient rejected: %v", err)
	}
	t.Setenv("PMAIL_CATCH_ALL", "true")
	deliver("override", b.ID, unknown...)
	t.Setenv("PMAIL_CATCH_ALL", "")
	assertRejection(newSession().Rcpt(unknown[0], nil), smtp.EnhancedCode{5, 1, 1})
}
