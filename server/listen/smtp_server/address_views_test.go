package smtp_server

import (
	stdctx "context"
	"net/netip"
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestEnvelopeAliasViewsInboxSearchOtherAndOutbound(t *testing.T) {
	e := newAuthenticationTestEngine(t)
	oldDB, oldConfig := db.Instance, config.Instance
	db.Instance, config.Instance = e, &config.Config{Domain: "views.test", CatchAll: true}
	t.Cleanup(func() {
		db.Instance, config.Instance = oldDB, oldConfig
		if oldDB != nil {
			_ = domain.Refresh()
		}
	})
	t.Setenv("PMAIL_CATCH_ALL", "true")
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	u := models.User{Account: "main", IsAdmin: 1, Password: "fixture"}
	if _, err := e.Insert(&u); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	alias1, err := address.Create(t.Context(), u.ID, domain.Default().ID, "one", true, true)
	if err != nil {
		t.Fatal(err)
	}
	alias2, err := address.Create(t.Context(), u.ID, domain.Default().ID, "two", true, true)
	if err != nil {
		t.Fatal(err)
	}
	primary, err := address.Primary(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &context.Context{Context: t.Context(), UserID: u.ID}
	deliver := func(subject string, targets ...string) {
		t.Helper()
		s := &Session{Ctx: &context.Context{Context: t.Context()}, From: "sender@outside.test", AuthenticateInbound: func(stdctx.Context, netip.Addr, string, string, []byte) models.MailAuthResults {
			return models.MailAuthResults{}
		}}
		for _, target := range targets {
			if err := s.Rcpt(target, nil); err != nil {
				t.Fatal(err)
			}
		}
		// The header is intentionally the primary address, not the actual recipients.
		raw := "From: sender@outside.test\r\nTo: main@views.test\r\nSubject: " + subject + "\r\n\r\nsearchable body\r\n"
		if err := s.Data(strings.NewReader(raw)); err != nil {
			t.Fatal(err)
		}
	}
	deliver("two aliases", alias1.Address, alias2.Address)
	deliver("catchall only", "unknown@views.test")
	deliver("mixed", alias1.Address, "other@views.test")
	inbox := dto.SearchTag{Type: 0, Status: -1, GroupId: 0}
	get := func(tag dto.SearchTag, filter mailaddress.Filter, keyword string, want int) []int {
		t.Helper()
		rows, total, err := list.WebPage(ctx, tag, keyword, list.MailCursor{}, 0, 20, true, filter)
		if err != nil || len(rows) != want || total != int64(want) {
			t.Fatalf("filter=%+v keyword=%q got %d/%d want %d err=%v", filter, keyword, len(rows), total, want, err)
		}
		ids := []int{}
		for _, r := range rows {
			ids = append(ids, r.Id)
		}
		return ids
	}
	a := get(inbox, mailaddress.Filter{ID: alias1.ID}, "two aliases", 1)
	b := get(inbox, mailaddress.Filter{ID: alias2.ID}, "two aliases", 1)
	if a[0] != b[0] {
		t.Fatal("two aliases created different copies")
	}
	get(inbox, mailaddress.Filter{}, "two aliases", 1)
	get(inbox, mailaddress.Filter{ID: primary.ID}, "", 0)
	get(inbox, mailaddress.Filter{Other: true}, "", 2)
	get(inbox, mailaddress.Filter{Other: true}, "two aliases", 0)
	count, err := mailaddress.Unread(u.ID)
	if err != nil || count.AllUnread != 3 || count.Other.Unread != 2 || count.Addresses[1].Unread != 2 || count.Addresses[2].Unread != 1 {
		t.Fatalf("counts: %+v err=%v", count, err)
	}
	// Same grouping when a local submission has both aliases as targets.
	outgoing := &parsemail.Email{From: parsemail.BuilderUser(primary.Address), To: []*parsemail.User{parsemail.BuilderUser(alias1.Address), parsemail.BuilderUser(alias2.Address)}, Subject: "local copies", Text: []byte("test")}
	if errMap, err := LocalDelivery(ctx, outgoing, outgoing.To); err != nil || len(errMap) != 0 {
		t.Fatalf("local delivery: %v %v", errMap, err)
	}
	get(inbox, mailaddress.Filter{}, "local copies", 1)
	get(inbox, mailaddress.Filter{ID: alias2.ID}, "local copies", 1)
	if _, err := persistInitialAudit(ctx, models.Email{Type: consts.EmailTypeSend, FromAddress: alias2.Address, Subject: "alias sender", Status: consts.EmailStatusSent}, []int{u.ID}); err != nil {
		t.Fatal(err)
	}
	sent := dto.SearchTag{Type: 1, Status: -1, GroupId: -1}
	get(sent, mailaddress.Filter{ID: alias2.ID}, "alias sender", 1)
	get(sent, mailaddress.Filter{ID: primary.ID}, "alias sender", 0)
}
