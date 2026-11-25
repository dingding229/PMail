package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	pctx "github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

const testBotToken = "123456:fixture-token-not-real"

func signInitData(t *testing.T, token string, fields map[string]string) string {
	t.Helper()
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	v := url.Values{}
	for i, k := range keys {
		lines[i] = k + "=" + fields[k]
		v.Set(k, fields[k])
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	v.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return v.Encode()
}

func launchFields(userID int64, at time.Time) map[string]string {
	user, _ := json.Marshal(map[string]any{"id": userID, "first_name": "Fixture"})
	return map[string]string{"auth_date": strconv.FormatInt(at.Unix(), 10), "query_id": "AAF", "user": string(user), "signature": "sig"}
}

func TestVerifyInitData(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	good := signInitData(t, testBotToken, launchFields(42, now.Add(-time.Minute)))
	if id, err := verifyInitData(good, testBotToken, now); err != nil || id != 42 {
		t.Fatalf("valid data: %d %v", id, err)
	}
	for name, raw := range map[string]string{
		"other bot": signInitData(t, "999:other", launchFields(42, now)),
		"expired":   signInitData(t, testBotToken, launchFields(42, now.Add(-2*time.Hour))),
		"future":    signInitData(t, testBotToken, launchFields(42, now.Add(time.Hour))),
		"no user":   signInitData(t, testBotToken, map[string]string{"auth_date": strconv.FormatInt(now.Unix(), 10)}),
		"tampered":  strings.Replace(good, "42", "43", 1),
		"empty":     "",
		"duplicate": good + "&user=%7B%22id%22%3A7%7D",
	} {
		if _, err := verifyInitData(raw, testBotToken, now); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func webAppDB(t *testing.T) (*xorm.Engine, models.User, models.User, models.Email) {
	t.Helper()
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	old, oldCfg := db.Instance, config.Instance
	db.Instance, config.Instance = e, &config.Config{SecretKey: "test-only-not-for-a-deployment"}
	t.Cleanup(func() { e.Close(); db.Instance, config.Instance = old, oldCfg })
	if err = db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	a, b := models.User{Account: "a", Name: "a", Password: "x"}, models.User{Account: "b", Name: "b", Password: "x"}
	mail := models.Email{Subject: "hi"}
	if _, err = e.Insert(&a, &b, &mail); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Insert(&models.UserEmail{UserID: a.ID, EmailID: mail.Id}); err != nil {
		t.Fatal(err)
	}
	return e, a, b, mail
}

func TestVerifyWebAppBindingAndTicket(t *testing.T) {
	e, a, b, mail := webAppDB(t)
	sealed, err := seal(testBotToken, "token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Insert(&models.TelegramConfig{ID: 1, Token: sealed, Username: "bot", Generation: "gen1"},
		&models.TelegramBinding{UserID: a.ID, ChatID: 42, Identity: "id-a", Bot: "gen1"},
		&models.TelegramBinding{UserID: b.ID, ChatID: -100, Identity: "id-b", Bot: "gen1"}); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	now := time.Now()
	if uid, _, err := VerifyWebApp(ctx, signInitData(t, testBotToken, launchFields(42, now)), mail.Id); err != nil || uid != a.ID {
		t.Fatalf("bound user: %d %v", uid, err)
	}
	for name, tc := range map[string]struct {
		user int64
		mail int
	}{"unbound telegram user": {7, mail.Id}, "group chat id": {-100, mail.Id}, "mail not owned": {42, mail.Id + 99}} {
		if _, _, err := VerifyWebApp(ctx, signInitData(t, testBotToken, launchFields(tc.user, now)), tc.mail); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err = e.Exec("UPDATE telegram_config SET generation='gen2' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyWebApp(ctx, signInitData(t, testBotToken, launchFields(42, now)), mail.Id); err == nil {
		t.Fatal("binding of a replaced bot accepted")
	}

	ticket := Ticket(a.ID, mail.Id, now)
	if uid, err := CheckTicket(ctx, ticket, mail.Id, now); err != nil || uid != a.ID {
		t.Fatalf("ticket: %d %v", uid, err)
	}
	if _, err := CheckTicket(ctx, ticket, mail.Id+1, now); err == nil {
		t.Fatal("ticket for another mail accepted")
	}
	if _, err := CheckTicket(ctx, ticket, mail.Id, now.Add(31*time.Minute)); err == nil {
		t.Fatal("expired ticket accepted")
	}
	forged := strings.Replace(ticket, "."+strconv.Itoa(a.ID)+".", "."+strconv.Itoa(b.ID)+".", 1)
	if _, err := CheckTicket(ctx, forged, mail.Id, now); err == nil {
		t.Fatal("ticket for another user accepted")
	}
	if _, err = e.Exec("UPDATE "+db.Instance.Quote("user")+" SET disabled=1 WHERE id=?", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckTicket(ctx, ticket, mail.Id, now); err == nil {
		t.Fatal("ticket of a disabled user accepted")
	}
}

func TestWebAppButton(t *testing.T) {
	t.Setenv("PMAIL_PUBLIC_URL", "https://mail.example.com/mail/inbox")
	ctx := &pctx.Context{Context: t.Context(), Lang: "zh-CN"}
	m := withWebApp(ctx, &replyMarkup{Keyboard: [][]inlineButton{{{Text: "AI", CallbackData: "ai:5"}}}}, 42, 5)
	if len(m.Keyboard) != 2 || m.Keyboard[0][0].WebApp == nil || m.Keyboard[0][0].WebApp.URL != "https://mail.example.com/tg/mail?mail=5" {
		t.Fatalf("button: %+v", m)
	}
	if got := withWebApp(ctx, nil, -100, 5); got != nil {
		t.Fatal("group chats cannot open Mini Apps")
	}
	t.Setenv("PMAIL_PUBLIC_URL", "http://mail.example.com")
	if got := withWebApp(ctx, nil, 42, 5); got != nil {
		t.Fatal("Mini Apps need HTTPS")
	}
}
