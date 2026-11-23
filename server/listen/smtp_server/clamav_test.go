package smtp_server

import (
	"bytes"
	stdctx "context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/antivirus"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/utils/context"
	smtp "github.com/emersion/go-smtp"
	log "github.com/sirupsen/logrus"
)

func TestDATAClamAVRejectsInfectedAndPersistsFailOpen(t *testing.T) {
	logger := log.StandardLogger()
	original, formatter := logger.Out, logger.Formatter
	var audit bytes.Buffer
	logger.SetOutput(&audit)
	logger.SetFormatter(&log.TextFormatter{DisableQuote: true})
	t.Cleanup(func() { logger.SetOutput(original); logger.SetFormatter(formatter) })
	e := newAuthenticationTestEngine(t)
	oldDB, oldCfg := db.Instance, config.Instance
	db.Instance, config.Instance = e, &config.Config{Domain: "clamav-smtp.test"}
	t.Cleanup(func() {
		db.Instance, config.Instance = oldDB, oldCfg
		if oldDB != nil {
			_ = domain.Refresh()
		}
	})
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.User{Account: "recipient", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	for _, result := range []antivirus.Result{{Status: "infected", Virus: "Test.Signature"}, {Status: "not_scanned", Reason: "scanner_unavailable"}, {Status: "clean"}} {
		t.Run(result.Status, func(t *testing.T) {
			s := &Session{Ctx: &context.Context{Context: t.Context()}, From: "sender@outside.test", AuthenticateInbound: func(stdctx.Context, netip.Addr, string, string, []byte) models.MailAuthResults {
				return models.MailAuthResults{}
			}}
			if err := s.Rcpt("recipient@clamav-smtp.test", nil); err != nil {
				t.Fatal(err)
			}
			raw := "From: sender@outside.test\r\nTo: recipient@clamav-smtp.test\r\nSubject: C22 " + result.Status + "\r\n\r\nMessage fixture\r\n"
			called := 0
			s.scanInbound = func(_ stdctx.Context, content []byte) *antivirus.Result {
				called++
				if string(content) != raw {
					t.Fatal("raw email changed before scan")
				}
				return &result
			}
			err := s.Data(strings.NewReader(raw))
			if called != 1 {
				t.Fatalf("scanned %d times", called)
			}
			var row models.Email
			exists, dbErr := e.Where("subject = ?", "C22 "+result.Status).Get(&row)
			if dbErr != nil {
				t.Fatal(dbErr)
			}
			if result.Status == "infected" {
				var rejected *smtp.SMTPError
				if !errors.As(err, &rejected) || rejected.Code != 554 || rejected.EnhancedCode != (smtp.EnhancedCode{5, 7, 1}) || exists {
					t.Fatalf("rejected=%v inserted=%t", err, exists)
				}
				for _, entry := range []string{`audit_event=inbound_virus_rejected`, `virus="Test.Signature"`, `mail_from="sender@outside.test"`, `rcpt_to=["recipient@clamav-smtp.test"]`} {
					if !strings.Contains(audit.String(), entry) {
						t.Errorf("missing audit field %q", entry)
					}
				}
				return
			}
			if err != nil || !exists {
				t.Fatalf("mail was not delivered: %v", err)
			}
			a := inbound.FromStorage(row.InboundAnalysis)
			if a == nil || a.Antivirus == nil || *a.Antivirus != result {
				t.Fatalf("scan result not persisted: %+v", a)
			}
		})
	}
}

func TestSubmissionDoesNotCallClamAV(t *testing.T) {
	s := &Session{Ctx: &context.Context{Context: t.Context(), UserID: 1}, scanInbound: func(stdctx.Context, []byte) *antivirus.Result { t.Fatal("outbound SMTP was scanned"); return nil }}
	e := newAuthenticationTestEngine(t)
	oldDB := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = oldDB })
	if _, err := e.Insert(&models.User{ID: 1, Account: "outbound", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	// An intentionally malformed envelope exits before outbound transport.
	if err := s.Data(strings.NewReader("invalid MIME\r\n\r\n")); err == nil {
		t.Fatal("malformed fixture accepted")
	}
}
