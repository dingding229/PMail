package inbound

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func inboundFixture(t testing.TB) (*xorm.Engine, int, int) {
	t.Helper()
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	old := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = old; _ = e.Close() })
	if err = testschema.Sync(e); err != nil {
		t.Fatal(err)
	}
	first, second := models.User{Account: "inbound_a", Name: "A", Password: "fixture"}, models.User{Account: "inbound_b", Name: "B", Password: "fixture"}
	if _, err = e.Insert(&first, &second); err != nil {
		t.Fatal(err)
	}
	return e, first.ID, second.ID
}

func TestSettingsRulesAndPerRecipientScore(t *testing.T) {
	e, a, b := inboundFixture(t)
	ctx := context.Background()
	if _, err := PutSettings(ctx, Settings{DefaultSpamThreshold: 9, DNSBLEnabled: true, Zones: []string{"bad.test", "BAD.TEST."}, RiskAudit: true}, CatchAllUpdate{}); err != nil {
		t.Fatal(err)
	}
	s, err := GetSettings(ctx)
	if err != nil || len(s.Zones) != 1 || s.DefaultSpamThreshold != 9 || !s.RiskAudit {
		t.Fatalf("settings %+v %v", s, err)
	}
	if _, err = PutSettings(ctx, Settings{DefaultSpamThreshold: 7, Zones: []string{}}, CatchAllUpdate{}); err != nil {
		t.Fatal(err)
	}
	aRule, err := AddRule(ctx, a, models.SpamRule{Action: "block", Kind: "domain", Value: "Sender.Example."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AddRule(ctx, a, models.SpamRule{Action: "block", Kind: "domain", Value: "sender.example"}); !errors.Is(err, ErrRuleConflict) {
		t.Fatal(err)
	}
	if err = DeleteRule(ctx, b, aRule.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err = AddRule(ctx, b, models.SpamRule{Action: "allow", Kind: "address", Value: "FROM@SUB.SENDER.EXAMPLE"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PMAIL_SPAM_KEYWORDS", "PROMO SAMPLE,虚构促销")
	msg := &parsemail.Email{Subject: "PROMO SAMPLE", From: &parsemail.User{EmailAddress: "from@sub.sender.example"}, ReplyTo: []*parsemail.User{{EmailAddress: "reply@other.example"}}, Text: []byte("虚构促销")}
	auth := &models.MailAuthResults{SPF: models.MailAuthCheck{Result: "fail"}, DKIM: models.MailAuthCheck{Result: "fail"}, DMARC: models.MailAuthCheck{Result: "fail"}}
	shared, perOwner := AnalyzeDelivery(ctx, msg, Envelope{}, auth, []int{a, b})
	if shared == nil || len(shared.Base) < 5 || perOwner[a] == nil || perOwner[b] == nil || perOwner[a].Verdict != "spam" || perOwner[b].Verdict != "ham" || perOwner[b].Score != 0 {
		t.Fatalf("classification %+v %#v", shared, perOwner)
	}
	if _, err = PutPreference(ctx, a, models.SpamPreference{Enabled: false, Threshold: 2, BayesEnabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, perOwner = AnalyzeDelivery(ctx, msg, Envelope{}, auth, []int{a}); perOwner[a] == nil || perOwner[a].Verdict != "unknown" {
		t.Fatalf("disabled: %+v", perOwner[a])
	}
	if _, err = PutPreference(ctx, a, models.SpamPreference{Enabled: true, Threshold: 2, BayesEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = PutPreference(ctx, a, models.SpamPreference{Enabled: false, Threshold: 2, BayesEnabled: false}); err != nil {
		t.Fatal(err)
	}
	pref, err := Preference(ctx, a, 7)
	if err != nil || pref.Enabled || pref.BayesEnabled {
		t.Fatalf("bool update: %+v %v", pref, err)
	}
	for i := 0; i < RuleLimit-1; i++ {
		value := fmt.Sprintf("fixture %d", i)
		if _, err = e.Insert(&models.SpamRule{UserID: a, Action: "allow", Kind: "keyword", Value: value, ValueHash: hashValue(value)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = AddRule(ctx, a, models.SpamRule{Action: "block", Kind: "keyword", Value: "new rule"}); !errors.Is(err, ErrRuleLimit) {
		t.Fatal(err)
	}
}

func TestBayesWarmupIdempotenceUndoAndOwnerIsolation(t *testing.T) {
	e, a, b := inboundFixture(t)
	ctx := context.Background()
	add := func(text string) int {
		t.Helper()
		row := models.Email{Subject: text, Text: sql.NullString{String: text, Valid: true}}
		if _, err := e.Insert(&row); err != nil {
			t.Fatal(err)
		}
		for _, u := range []int{a, b} {
			if _, err := e.Insert(&models.UserEmail{UserID: u, EmailID: row.Id}); err != nil {
				t.Fatal(err)
			}
		}
		return row.Id
	}
	spam, ham := []int{}, []int{}
	for i := 0; i < 20; i++ {
		spam = append(spam, add("fake jackpot 试玩彩金"))
		ham = append(ham, add("project meeting 项目会议"))
	}
	if err := MarkSpam(ctx, a, spam, true); err != nil {
		t.Fatal(err)
	}
	if err := MarkSpam(ctx, a, spam, true); err != nil {
		t.Fatal(err)
	}
	if err := MarkSpam(ctx, a, ham[:19], false); err != nil {
		t.Fatal(err)
	}
	if w, err := bayesScore(ctx, e, a, "", "fake jackpot 试玩彩金"); err != nil || w != 0 {
		t.Fatalf("warmup: %v %v", w, err)
	}
	if err := MarkSpam(ctx, a, ham[19:], false); err != nil {
		t.Fatal(err)
	}
	if w, err := bayesScore(ctx, e, a, "", "fake jackpot 试玩彩金"); err != nil || w < 1 {
		t.Fatalf("spam score %v %v", w, err)
	}
	if w, err := bayesScore(ctx, e, a, "", "project meeting 项目会议"); err != nil || w > -1 {
		t.Fatalf("ham score %v %v", w, err)
	}
	if w, err := bayesScore(ctx, e, b, "", "fake jackpot 试玩彩金"); err != nil || w != 0 {
		t.Fatalf("another owner's score %v %v", w, err)
	}
	if err := MarkSpam(ctx, a, []int{spam[0], 99999}, false); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	var totals models.SpamTotals
	if _, err := e.ID(a).Get(&totals); err != nil || totals.Spam != 20 || totals.Ham != 20 {
		t.Fatalf("idempotence/atomic %+v %v", totals, err)
	}
	if err := MarkSpam(ctx, a, spam[:1], false); err != nil {
		t.Fatal(err)
	}
	totals = models.SpamTotals{}
	if _, err := e.ID(a).Get(&totals); err != nil || totals.Spam != 19 || totals.Ham != 21 {
		t.Fatalf("undo %+v %v", totals, err)
	}
	var counts models.SpamToken
	if _, err := e.Where("user_id=? AND token_key=?", a, hashValue("jackpot")).Get(&counts); err != nil || counts.Spam != 19 || counts.Ham != 1 {
		t.Fatalf("token undo %+v %v", counts, err)
	}
	var other models.UserEmail
	if _, err := e.Where("user_id=? AND email_id=?", b, spam[1]).Get(&other); err != nil || other.Status != 0 || other.InboundSpam.Valid {
		t.Fatalf("cross owner modified %+v %v", other, err)
	}
	if len(tokens("", strings.Repeat("benign ", 2048)+"消息")) != 2 {
		t.Fatal("dedup or Chinese bigram changed")
	}
	var big strings.Builder
	for i := 0; i < 2100; i++ {
		fmt.Fprintf(&big, "word%04d ", i)
	}
	if n := len(tokens("", big.String())); n != tokenLimit {
		t.Fatalf("tokens cap = %d", n)
	}
}

func BenchmarkInbound100KiB(b *testing.B) {
	_, uid, _ := inboundFixture(b)
	const header = "From: fixture@example.test\r\nTo: local@example.test\r\nSubject: Example verification code: 438719\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n"
	text := strings.Repeat("This is generated benign content for measurement.\r\n", 2300)
	wire := []byte(header + text[:(100<<10)-len(header)])
	for _, tc := range []struct {
		name string
		scan bool
	}{{"ParseOnly", false}, {"ParseAndAnalyze", true}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				e, err := parsemail.ParseLimited([]string{"local@example.test"}, bytes.NewReader(wire), len(wire))
				if err != nil {
					b.Fatal(err)
				}
				if tc.scan {
					if a, perOwner := AnalyzeDelivery(context.Background(), e, Envelope{}, nil, []int{uid}); a == nil || perOwner[uid] == nil {
						b.Fatal("analysis unexpectedly unavailable")
					}
				}
			}
		})
	}
}

func TestAnalysisDoesNotWaitPastDeadlineForStorage(t *testing.T) {
	e, owner, _ := inboundFixture(t)
	connection, err := e.DB().DB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	// The only SQLite connection remains occupied. A classification read must
	// not keep delivery waiting after the caller's analysis deadline expires.
	deadline, stop := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer stop()
	started := time.Now()
	a, owners := AnalyzeDelivery(deadline, &parsemail.Email{Subject: "OTP: 234567"}, Envelope{}, nil, []int{owner})
	if a != nil || owners != nil || time.Since(started) > deadlineTestCeiling {
		t.Fatalf("analysis outlived deadline: %v %+v %v", time.Since(started), a, owners)
	}
}

const deadlineTestCeiling = 500 * time.Millisecond

func TestCatchAllOverridePreservesStoredChoice(t *testing.T) {
	t.Setenv("PMAIL_CATCH_ALL", "")
	_, owner, _ := inboundFixture(t)
	ctx := t.Context()
	s := Settings{DefaultSpamThreshold: 6, Zones: []string{}}
	off := false
	if _, err := PutSettings(ctx, s, CatchAllUpdate{Enabled: &off, RecipientID: &owner}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PMAIL_CATCH_ALL", "true")
	on := true
	got, err := PutSettings(ctx, s, CatchAllUpdate{Enabled: &on})
	if err != nil || !got.CatchAll.EnvControlled || !got.CatchAll.Enabled || got.CatchAll.RecipientID != owner {
		t.Fatalf("effective settings %+v, %v", got.CatchAll, err)
	}
	t.Setenv("PMAIL_CATCH_ALL", "")
	got, err = GetSettings(ctx)
	if err != nil || got.CatchAll.EnvControlled || got.CatchAll.Enabled || got.CatchAll.RecipientID != owner {
		t.Fatalf("stored settings %+v, %v", got.CatchAll, err)
	}
	missing := owner + 1000
	if _, err := PutSettings(ctx, s, CatchAllUpdate{RecipientID: &missing}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepted missing recipient: %v", err)
	}
}
