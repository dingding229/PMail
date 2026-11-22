package smtp_server

import (
	stdctx "context"
	"net/netip"
	"strings"

	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

func TestSMTPAndExternalPersistSharedAndOwnerAnalysis(t *testing.T) {
	e := newAuthenticationTestEngine(t)
	old, oldConfig := db.Instance, config.Instance
	db.Instance = e
	config.Instance = &config.Config{Domain: "example.test", Domains: []string{"example.test"}, CatchAll: false}
	t.Cleanup(func() { db.Instance, config.Instance = old, oldConfig })
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	a, b := models.User{Account: "a", Name: "A", Password: "fixture"}, models.User{Account: "b", Name: "B", Password: "fixture"}
	if _, err := e.Insert(&a, &b); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	ctx := stdctx.Background()
	if _, err := inbound.AddRule(ctx, a.ID, models.SpamRule{Action: "block", Kind: "domain", Value: "sender.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := inbound.AddRule(ctx, b.ID, models.SpamRule{Action: "allow", Kind: "address", Value: "from@sender.example"}); err != nil {
		t.Fatal(err)
	}
	const wire = "From: from@sender.example\r\nTo: a@example.test, b@example.test\r\nSubject: Login code: 352719\r\nMessage-ID: <fixture@sender.example>\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nFixture\r\n"
	s := &Session{Ctx: &context.Context{Context: ctx}, From: "from@sender.example", To: []string{"a@example.test", "b@example.test"}, AuthenticateInbound: func(stdctx.Context, netip.Addr, string, string, []byte) models.MailAuthResults {
		return models.MailAuthResults{}
	}}
	if err := s.Data(strings.NewReader(wire)); err != nil {
		t.Fatal(err)
	}
	var mails []models.Email
	if err := e.Find(&mails); err != nil || len(mails) != 1 {
		t.Fatalf("SMTP rows=%d %v", len(mails), err)
	}
	check := func(id, owner int, status int8, verdict string) {
		t.Helper()
		var link models.UserEmail
		ok, err := e.Where("user_id=? AND email_id=?", owner, id).Get(&link)
		got := inbound.ReadDetail(mails[0].InboundAnalysis, link.InboundSpam)
		if err != nil || !ok || link.Status != status || got.Spam == nil || got.Spam.Verdict != verdict || got.VerificationCode == nil || got.VerificationCode.Code != "352719" || got.Risk == nil {
			t.Fatalf("mail=%d owner=%d link=%+v view=%+v %v", id, owner, link, got, err)
		}
	}
	check(mails[0].Id, a.ID, consts.EmailStatusJunk, "spam")
	check(mails[0].Id, b.ID, consts.EmailStatusWait, "ham")
	imported, err := ImportExternal(&context.Context{Context: ctx, UserID: a.ID}, []byte(wire), "a@example.test", 0, func(*xorm.Session) (bool, error) { return false, nil })
	if err != nil || imported == nil || !imported.InboundAnalysis.Valid {
		t.Fatalf("import: %+v %v", imported, err)
	}
	check(imported.Id, a.ID, consts.EmailStatusJunk, "spam")
	local, err := parsemail.ParseLimited([]string{"b@example.test"}, strings.NewReader(wire), len(wire))
	if err != nil {
		t.Fatal(err)
	}
	local.RawData = []byte(wire)
	if err, perAddress := LocalDelivery(&context.Context{Context: ctx, UserID: a.ID}, local, []*parsemail.User{{EmailAddress: "b@example.test"}}); err != nil || len(perAddress) > 0 {
		t.Fatalf("local: %v %v", err, perAddress)
	}
	var last models.Email
	if ok, err := e.Desc("id").Get(&last); err != nil || !ok || last.Id == imported.Id || !last.InboundAnalysis.Valid {
		t.Fatalf("local result id=%d %v", last.Id, err)
	}
	check(last.Id, b.ID, consts.EmailStatusWait, "ham")

}
