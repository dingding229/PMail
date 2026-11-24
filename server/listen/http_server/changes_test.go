package http_server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/listen/smtp_server"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/mailevents"
	"github.com/Jinnrry/pmail/services/token"
	pctx "github.com/Jinnrry/pmail/utils/context"
)

type liveReply struct {
	Cursor   string            `json:"cursor"`
	Changed  bool              `json:"changed"`
	NewCount int64             `json:"new_count"`
	NewItems []mailevents.Item `json:"new_items"`
}

type liveCall struct {
	reply  liveReply
	status int
	cache  string
	err    error
}

func pollChanges(ctx context.Context, client *http.Client, base, cursor string, wait int, bearer string) liveCall {
	q := url.Values{"cursor": {cursor}, "wait": {strconv.Itoa(wait)}}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/email/changes?"+q.Encode(), nil)
	if err != nil {
		return liveCall{err: err}
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := client.Do(r)
	if err != nil {
		return liveCall{err: err}
	}
	defer res.Body.Close()
	out := liveCall{status: res.StatusCode, cache: res.Header.Get("Cache-Control")}
	if res.StatusCode == 200 {
		out.err = json.NewDecoder(res.Body).Decode(&out.reply)
	}
	return out
}
func liveOK(t *testing.T, c liveCall) liveReply {
	t.Helper()
	if c.err != nil || c.status != 200 || c.cache != "no-store" || c.reply.Cursor == "" || c.reply.NewItems == nil {
		t.Fatalf("unexpected live update: %+v", c)
	}
	return c.reply
}
func startLongPoll(t *testing.T, client *http.Client, base, cursor string) <-chan liveCall {
	t.Helper()
	pending := make(chan liveCall, 1)
	go func() { pending <- pollChanges(t.Context(), client, base, cursor, 5, "") }()
	stillWaiting(t, pending)
	return pending
}
func stillWaiting(t *testing.T, pending <-chan liveCall) {
	t.Helper()
	select {
	case response := <-pending:
		t.Fatalf("poll returned before eligible change: %+v", response)
	case <-time.After(65 * time.Millisecond):
	}
}
func wakeWithinSecond(t *testing.T, pending <-chan liveCall) liveReply {
	t.Helper()
	select {
	case reply := <-pending:
		return liveOK(t, reply)
	case <-time.After(time.Second):
		t.Fatal("mailbox change did not wake long poll in one second")
		return liveReply{}
	}
}

func liveFixture(t *testing.T) (*authFixture, *http.Client) {
	t.Helper()
	f := newAuthFixture(t)
	if err := db.SyncSchema(f.db); err != nil {
		t.Fatal(err)
	}
	return f, f.login(t, "member", "initial-password")
}
func liveMail(t *testing.T, f *authFixture, owner int, typ, status int8, folder int, subject string) models.Email {
	t.Helper()
	m := models.Email{Type: typ, Subject: subject, FromName: "Fixture", FromAddress: "sender@external.test", To: `[{"Name":"Public","EmailAddress":"member@poll.test"}]`}
	if _, err := f.db.Insert(&m); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Insert(&models.UserEmail{UserID: owner, EmailID: m.Id, Status: status, GroupId: folder}); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMailChangesHandshakeAuthenticationAndTimeout(t *testing.T) {
	f, client := liveFixture(t)
	liveMail(t, f, f.member.ID, 0, 0, 0, "historical")
	if got := pollChanges(t.Context(), http.DefaultClient, f.server.URL, "", 25, ""); got.status != 401 || got.cache != "no-store" || got.err != nil {
		t.Fatalf("unauthenticated status/cache: %+v", got)
	}
	start := time.Now()
	initial := liveOK(t, pollChanges(t.Context(), client, f.server.URL, "", 25, ""))
	if !initial.Changed || initial.NewCount != 0 || len(initial.NewItems) != 0 || time.Since(start) >= time.Second {
		t.Fatal("first poll should return without notifications or a wait")
	}
	start = time.Now()
	reset := liveOK(t, pollChanges(t.Context(), client, f.server.URL, "staleEpoch.500.200", 25, ""))
	if !reset.Changed || reset.NewCount != 0 || reset.Cursor != initial.Cursor || time.Since(start) >= time.Second {
		t.Fatal("epoch mismatch was not an immediate reset")
	}
	quiet := liveOK(t, pollChanges(t.Context(), client, f.server.URL, initial.Cursor, 1, ""))
	if quiet.Changed || quiet.NewCount != 0 || quiet.Cursor != initial.Cursor {
		t.Fatal("quiet mailbox reported a change")
	}
	for _, scope := range []string{token.Read, token.Send} {
		_, key, err := token.Create(f.member.ID, "poll-scope", scope, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if result := liveOK(t, pollChanges(t.Context(), http.DefaultClient, f.server.URL, "", 0, key)); result.Cursor != initial.Cursor {
			t.Fatal("read-capable token opened a different mailbox")
		}
	}
}

func TestMailChangesWakesOnSMTPButNotAnotherUser(t *testing.T) {
	f, client := liveFixture(t)
	old := config.Instance
	config.Instance = &config.Config{Domain: "poll.test"}
	t.Cleanup(func() { config.Instance = old })
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	before := liveOK(t, pollChanges(t.Context(), client, f.server.URL, "", 0, ""))
	pending := startLongPoll(t, client, f.server.URL, before.Cursor)
	submit := func(recipient string) {
		t.Helper()
		s := &smtp_server.Session{Ctx: &pctx.Context{Context: t.Context()}, From: "sender@external.test", AuthenticateInbound: func(context.Context, netip.Addr, string, string, []byte) models.MailAuthResults {
			return models.MailAuthResults{}
		}}
		if err := s.Rcpt(recipient, nil); err != nil {
			t.Fatal(err)
		}
		text := "From: Fixture <sender@external.test>\r\nTo: " + recipient + "\r\nSubject: SMTP update\r\n\r\nThis body should not be returned.\r\n"
		if err := s.Data(strings.NewReader(text)); err != nil {
			t.Fatal(err)
		}
	}
	submit("admin@poll.test")
	stillWaiting(t, pending)
	start := time.Now()
	submit("member@poll.test")
	reply := wakeWithinSecond(t, pending)
	if time.Since(start) > time.Second || !reply.Changed || reply.NewCount != 1 || len(reply.NewItems) != 1 {
		t.Fatalf("notified too late or leaked another mailbox: %+v", reply)
	}
	item := reply.NewItems[0]
	if item.ID <= 0 || item.Subject != "SMTP update" || item.FromName != "Fixture" || item.FromAddress != "sender@external.test" || item.ToAddress != "member@poll.test" || item.GroupID != 0 || item.Created < start.Add(-2*time.Second).UnixMilli() {
		t.Fatalf("wrong SMTP preview: %+v", item)
	}
	if !strings.HasSuffix(reply.Cursor, fmt.Sprint(".", item.ID)) {
		t.Fatal("cursor did not include the latest visible mail")
	}
}

func TestMailChangesFiltersCountAndPreviewLimit(t *testing.T) {
	f, client := liveFixture(t)
	baseline := liveOK(t, pollChanges(t.Context(), client, f.server.URL, "", 0, ""))
	custom, foreign := models.Group{Name: "custom", UserId: f.member.ID}, models.Group{Name: "foreign", UserId: f.admin.ID}
	if _, err := f.db.Insert(&custom, &foreign); err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		typ, status int8
		folder      int
	}{
		{0, consts.EmailStatusDel, 0}, {0, consts.EmailStatusJunk, 0}, {0, 0, models.Junk}, {0, consts.EmailStatusDrafts, 0}, {0, 0, models.Drafts}, {1, 0, 0}, {0, 0, foreign.ID}, {0, consts.EmailStatusSent, custom.ID},
	} {
		liveMail(t, f, f.member.ID, pair.typ, pair.status, pair.folder, "excluded")
	}
	liveMail(t, f, f.admin.ID, 0, 0, 0, "other owner")
	var expected []models.Email
	for i := range 7 {
		gid := custom.ID
		if i == 0 {
			gid = models.INBOX
		}
		expected = append(expected, liveMail(t, f, f.member.ID, 0, 0, gid, "include"))
	}
	// Multiple links through IMAP COPY are still one newly received message.
	if _, err := f.db.Insert(&models.UserEmail{UserID: f.member.ID, EmailID: expected[6].Id, GroupId: models.INBOX}); err != nil {
		t.Fatal(err)
	}
	last := liveMail(t, f, f.member.ID, 1, consts.EmailStatusSent, 0, "new outbound cursor high water mark")
	liveMail(t, f, f.admin.ID, 0, 0, 0, "newer but invisible")
	a := liveOK(t, pollChanges(t.Context(), client, f.server.URL, baseline.Cursor, 0, ""))
	if !a.Changed || a.NewCount != 7 || len(a.NewItems) != 5 || !strings.HasSuffix(a.Cursor, fmt.Sprint(".", last.Id)) {
		t.Fatalf("bad count/limit/cursor: %+v", a)
	}
	for i, item := range a.NewItems {
		if item.ID != expected[6-i].Id || item.GroupID != custom.ID || item.Subject != "include" {
			t.Fatalf("wrong preview ordering or placement: %+v", item)
		}
	}
}

func TestWebReadDeleteAndRestoreWakeOtherTabOncePerBatch(t *testing.T) {
	f, client := liveFixture(t)
	one := liveMail(t, f, f.member.ID, 0, 0, 0, "one")
	two := liveMail(t, f, f.member.ID, 0, 0, 0, "two")
	initial := liveOK(t, pollChanges(t.Context(), client, f.server.URL, "", 0, ""))
	before := mailevents.Default.Version(f.member.ID)
	ids := []int{one.Id, two.Id}
	for _, path := range []string{"/api/email/read", "/api/email/del", "/api/email/restore"} {
		pending := startLongPoll(t, client, f.server.URL, initial.Cursor)
		if r := f.call(t, client, path, map[string]any{"ids": ids}); r.ErrorNo != 0 {
			t.Fatalf("%s: %+v", path, r)
		}
		a := wakeWithinSecond(t, pending)
		before++
		if !a.Changed || a.NewCount != 0 || !strings.Contains(a.Cursor, fmt.Sprintf(".%d.", before)) || mailevents.Default.Version(f.member.ID) != before {
			t.Fatalf("%s had no change or published multiple times", path)
		}
		initial = a
	}
}

func TestAddressReviewNoticeWakesOnlyApplicant(t *testing.T) {
	f, client := liveFixture(t)
	old := config.Instance
	config.Instance = &config.Config{Domain: "approval-notice.test"}
	t.Cleanup(func() { config.Instance = old })
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	cursor := liveOK(t, pollChanges(t.Context(), client, f.server.URL, "", 0, "")).Cursor
	adminVersion := mailevents.Default.Version(f.admin.ID)
	for _, status := range []string{models.AddressActive, models.AddressRejected} {
		pending := startLongPoll(t, client, f.server.URL, cursor)
		view := address.View{MailboxAddress: models.MailboxAddress{UserID: f.member.ID, Status: status}, Address: "new@approval-notice.test"}
		if err := smtp_server.DeliverAddressReview(t.Context(), view); err != nil {
			t.Fatal(err)
		}
		got := wakeWithinSecond(t, pending)
		if !got.Changed || got.NewCount != 1 || len(got.NewItems) != 1 || got.NewItems[0].ToAddress != "member@approval-notice.test" || mailevents.Default.Version(f.admin.ID) != adminVersion {
			t.Fatalf("review notice not confined to applicant: %+v", got)
		}
		cursor = got.Cursor
	}
}
