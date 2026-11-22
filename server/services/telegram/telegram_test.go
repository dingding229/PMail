package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/inbound"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestFakeBotBindingQueueRetryAndStop(t *testing.T) {
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	engine.SetMaxOpenConns(1)
	old, oldCfg := db.Instance, config.Instance
	db.Instance, config.Instance = engine, &config.Config{SecretKey: "test-only-not-for-a-deployment"}
	defer func() { engine.Close(); db.Instance, config.Instance = old, oldCfg }()
	if err = db.SyncSchema(engine); err != nil {
		t.Fatal(err)
	}
	uid := models.User{Account: "tg", Name: "fixture", Password: "fixture"}
	other := models.User{Account: "tg2", Name: "fixture", Password: "fixture"}
	if _, err = engine.Insert(&uid, &other); err != nil {
		t.Fatal(err)
	}
	response := `[]`
	behavior := 200
	var sent string
	var receivedOffset int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			fmt.Fprint(w, `{"ok":true,"result":{"is_bot":true,"username":"fixture_bot"}}`)
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			var body struct {
				Offset int64 `json:"offset"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			receivedOffset = body.Offset
			fmt.Fprintf(w, `{"ok":true,"result":%s}`, response)
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var body struct {
				Text string `json:"text"`
				Mode string `json:"parse_mode"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			sent = body.Text
			if body.Mode != "HTML" {
				t.Errorf("parse_mode=%q", body.Mode)
			}
			w.WriteHeader(behavior)
			switch behavior {
			case 429:
				fmt.Fprint(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":8}}`)
			case 403:
				fmt.Fprint(w, `{"ok":false,"error_code":403}`)
			default:
				fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	t.Setenv("PMAIL_TELEGRAM_API_BASE", s.URL)
	t.Setenv("PMAIL_TELEGRAM_PROXY", "")
	t.Setenv("PMAIL_PUBLIC_URL", "https://mail.example.test/")
	ctx := context.Background()
	token := "123:fixtureOnly_token_for_tests"
	if a, err := AdminPut(ctx, &token, nil, false); err != nil || !a.Configured || a.Username == nil {
		t.Fatalf("configure: %+v %v", a, err)
	}
	c, err := getConfig(ctx)
	if err != nil || c.Token == token || strings.Contains(c.Token, token) {
		t.Fatalf("unencrypted settings: %v", err)
	}
	b, err := NewCode(ctx, uid.ID)
	if err != nil {
		t.Fatal(err)
	}
	var saved models.TelegramCode
	if ok, err := engine.ID(uid.ID).Get(&saved); err != nil || !ok || saved.Hash == b.Code || saved.Hash != digest(b.Code) {
		t.Fatal("binding code is not hashed")
	}
	response = fmt.Sprintf(`[{"update_id":1,"message":{"chat":{"id":123,"type":"group"},"text":"/start %s"}},{"update_id":2,"message":{"chat":{"id":123,"type":"private","first_name":"Example"},"text":"/start %s"}}]`, b.Code, b.Code)
	if err = pollOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if u, err := UserGet(ctx, uid.ID); err != nil || !u.Bound {
		t.Fatalf("not bound %+v %v", u, err)
	}
	if err = consume(ctx, b.Code, chat{ID: 456, Type: "private"}, c.Generation, time.Now()); !errors.Is(err, ErrBindExpired) {
		t.Fatal("code replay accepted")
	}
	b2, err := NewCode(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = consume(ctx, b2.Code, chat{ID: 123, Type: "private"}, c.Generation, time.Now()); !errors.Is(err, ErrBindExpired) {
		t.Fatal("chat reassigned")
	}
	if err = consume(ctx, b2.Code, chat{ID: 456, Type: "private"}, c.Generation, time.Now().Add(11*time.Minute)); !errors.Is(err, ErrBindExpired) {
		t.Fatal("expired code used")
	}
	mail := models.Email{FromName: "<sender>", Subject: "<subject>", InboundAnalysis: inbound.Storage(&inbound.Analysis{Risk: &inbound.Risk{Level: "high"}, VerificationCode: &inbound.VerificationCode{Code: "412983"}})}
	if _, err = engine.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Insert(&models.UserEmail{UserID: uid.ID, EmailID: mail.Id}); err != nil {
		t.Fatal(err)
	}
	Notify(uid.ID, mail.Id, inbound.FromStorage(mail.InboundAnalysis))
	behavior = 429
	if !queueOnce(ctx) {
		t.Fatal("no queued delivery")
	}
	if !strings.Contains(sent, "<code>412983</code>") || strings.Contains(sent, "<subject>") || !strings.Contains(sent, "&lt;subject&gt;") {
		t.Fatalf("bad markup %q", sent)
	}
	var row models.TelegramPush
	if ok, err := engine.Get(&row); err != nil || !ok || row.Attempts != 1 || row.Due < time.Now().Add(7*time.Second).UnixMilli() {
		t.Fatalf("retry not persisted %+v %v", row, err)
	}
	messages.Lock()
	messages.next = time.Time{}
	messages.pause = time.Time{}
	clear(messages.chat)
	messages.Unlock()
	if _, err = engine.Table(&models.TelegramPush{}).ID(row.ID).Update(map[string]any{"due": time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	behavior = 403
	if !queueOnce(ctx) {
		t.Fatal("blocked push not processed")
	}
	if u, err := UserGet(ctx, uid.ID); err != nil || u.Bound {
		t.Fatal("403 did not unlink")
	}
	b, err = NewCode(ctx, uid.ID)
	if err != nil {
		t.Fatal(err)
	}
	response = fmt.Sprintf(`[{"update_id":3,"message":{"chat":{"id":123,"type":"private"},"text":"/start %s"}},{"update_id":4,"message":{"chat":{"id":123,"type":"private"},"text":"/stop"}}]`, b.Code)
	if err = pollOnce(ctx); err != nil || receivedOffset != 3 {
		t.Fatalf("durable polling offset=%d %v", receivedOffset, err)
	}
	if u, err := UserGet(ctx, uid.ID); err != nil || u.Bound {
		t.Fatal("/stop did not unlink")
	}
}
func TestFilterWallClockAndLimiter(t *testing.T) {
	now := time.Date(2026, 11, 23, 16, 0, 0, 0, time.UTC)
	if !quiet(&QuietHours{Start: "23:00", End: "06:00", TZ: "Asia/Shanghai"}, now) {
		t.Fatal("midnight window")
	}
	if matches(Filters{Groups: []int{}, OnlyVerification: true}, models.UserEmail{}, &inbound.Analysis{VerificationCode: &inbound.VerificationCode{Code: "482913"}}, now) {
		t.Fatal("empty group filter accepted mail")
	}
	l := limiter{chat: map[int64]time.Time{}}
	if ok, _ := l.allow(1, now); !ok {
		t.Fatal("first refused")
	}
	if ok, _ := l.allow(2, now.Add(39*time.Millisecond)); ok {
		t.Fatal("global rate not enforced")
	}
	if ok, _ := l.allow(1, now.Add(999*time.Millisecond)); ok {
		t.Fatal("chat rate not enforced")
	}
}
