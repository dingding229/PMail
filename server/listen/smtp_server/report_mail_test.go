package smtp_server

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"

	stdctx "context"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/services/group"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/reportmail"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestSMTPReportMailRulesForwardingAndNotification(t *testing.T) {
	e := newAuthenticationTestEngine(t)
	old, oldConfig := db.Instance, config.Instance
	db.Instance = e
	config.Instance = &config.Config{Domain: "example.test", Domains: []string{"example.test"}, SecretKey: "report-fixture-only"}
	t.Cleanup(func() { db.Instance, config.Instance = old, oldConfig })
	t.Setenv("PMAIL_DEFAULT_LANG", "en")
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	u := models.User{Account: "reports", Password: "fixture"}
	if _, err := e.Insert(&u); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	if _, err := forward.Set(u.ID, true, []string{"autoforward@remote.example"}, true); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	pctx := &context.Context{Context: ctx, UserID: u.ID}
	var notified []int
	inbound.SetNotifier(func(uid, id int, _ *inbound.Analysis) {
		if uid == u.ID {
			notified = append(notified, id)
		}
	})
	t.Cleanup(func() { inbound.SetNotifier(nil) })
	n := 0
	send := func(subject string, dsn bool) (models.UserEmail, models.Email) {
		t.Helper()
		n++
		body := "Content-Type: text/plain\r\n\r\nFixture\r\n"
		if dsn {
			body = "Content-Type: multipart/report; report-type=delivery-status; boundary=dsn\r\n\r\n--dsn\r\nContent-Type: text/plain\r\n\r\nFailure notice\r\n--dsn\r\nContent-Type: message/delivery-status\r\n\r\nReporting-MTA: dns; example.test\r\n\r\n--dsn--\r\n"
		}
		wire := fmt.Sprintf("From: reported@sender.example\r\nTo: reports@example.test\r\nSubject: %s\r\nMessage-ID: <report-fixture-%d@sender.example>\r\n", subject, n) + body
		// Like Backend.NewSession before TB25: no standard context inside.
		s := Session{Ctx: &context.Context{}, From: "reported@sender.example", To: []string{"reports@example.test"}, AuthenticateInbound: func(stdctx.Context, netip.Addr, string, string, []byte) models.MailAuthResults {
			return models.MailAuthResults{}
		}}
		if err := s.Data(strings.NewReader(wire)); err != nil {
			t.Fatal(err)
		}
		var mail models.Email
		if ok, err := e.Desc("id").Get(&mail); err != nil || !ok || mail.Subject != subject {
			t.Fatalf("not stored: %+v %v", mail, err)
		}
		var link models.UserEmail
		if ok, err := e.Where("email_id=? AND user_id=?", mail.Id, u.ID).Get(&link); err != nil || !ok {
			t.Fatalf("no link %v", err)
		}
		return link, mail
	}
	queueCount := func(id int) int64 {
		t.Helper()
		n, err := e.Where("email_id=?", id).Count(&models.OutboundQueue{})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	rule := func(action dto.RuleType, params string) *models.Rule {
		m := (&dto.Rule{Name: "fixture", Rules: []*dto.Value{{Field: "Subject", Type: "contains", Rule: "matching-rule"}}, Action: action, Params: params}).Encode()
		m.UserId = u.ID
		if _, err := e.Insert(m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	m, mail := send("DMARC Aggregate Report", false)
	var folder models.Group
	if ok, err := e.ID(m.GroupId).Get(&folder); err != nil || !ok || folder.Name != "Reports" || m.IsRead != 1 || len(notified) != 0 || queueCount(mail.Id) != 0 || mail.ReportMailState != string(reportmail.DMARCAggregate) {
		t.Fatalf("filed link=%+v folder=%+v notif=%v err=%v", m, folder, notified, err)
	}
	if ok, err := group.DelGroup(pctx, folder.ID); err != nil || !ok {
		t.Fatal(ok, err)
	}
	m, _ = send("DMARC Aggregate Report after recreation", false)
	if m.GroupId == 0 || m.GroupId == folder.ID || m.IsRead != 1 {
		t.Fatal(m)
	}
	to := models.Group{Name: "Rule", UserId: u.ID, FullPath: "Rule"}
	if _, err := e.Insert(&to); err != nil {
		t.Fatal(err)
	}
	mover := rule(dto.MOVE, fmt.Sprint(to.ID))
	m, _ = send("DMARC Aggregate Report matching-rule move", false)
	if m.GroupId != to.ID || m.IsRead != 0 {
		t.Fatalf("rule lost: %+v", m)
	}
	if _, err := e.ID(mover.Id).Delete(&models.Rule{}); err != nil {
		t.Fatal(err)
	}
	forwarder := rule(dto.FORWARD, "explicit@remote.example")
	m, mail = send("DMARC Aggregate Report matching-rule forward", false)
	if m.GroupId == 0 || queueCount(mail.Id) != 1 {
		t.Fatal("explicit rule forward not preserved")
	}
	var item models.OutboundQueue
	if ok, err := e.Where("email_id=?", mail.Id).Get(&item); err != nil || !ok || item.RcptTo != "explicit@remote.example" {
		t.Fatal(ok, err, item.RcptTo)
	}
	if _, err := e.ID(forwarder.Id).Delete(&models.Rule{}); err != nil {
		t.Fatal(err)
	}
	p := true
	if _, err := reportmail.UpdatePreference(ctx, u.ID, nil, nil, nil, &p); err != nil {
		t.Fatal(err)
	}
	m, mail = send("DMARC Aggregate Report opt-in forwarding", false)
	if m.GroupId == 0 || queueCount(mail.Id) != 1 {
		t.Fatal("forward opt-in not effective")
	}
	f := false
	last := len(notified)
	if _, err := reportmail.UpdatePreference(ctx, u.ID, &f, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	m, mail = send("DMARC Aggregate Report disabled", false)
	if m.GroupId != 0 || m.IsRead != 0 || len(notified) != last+1 || queueCount(mail.Id) != 1 {
		t.Fatal("disabled report behavior changed")
	}
	if _, err := reportmail.UpdatePreference(ctx, u.ID, &p, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	m, mail = send("DMARC Aggregate Report DSN stays in inbox", true)
	if m.GroupId != 0 || m.IsRead != 0 || mail.ReportMailState != ":" {
		t.Fatal("DSN should not be filed", m, mail.ReportMailState)
	}
	if _, err := inbound.AddRule(ctx, u.ID, models.SpamRule{Action: "block", Kind: "domain", Value: "sender.example"}); err != nil {
		t.Fatal(err)
	}
	m, mail = send("DMARC Aggregate Report spam", false)
	if m.GroupId != 0 || m.Status != 5 || queueCount(mail.Id) != 0 {
		t.Fatal("spam verdict must prevail", m)
	}
}
