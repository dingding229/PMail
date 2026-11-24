package telegram

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/translation"
	pctx "github.com/Jinnrry/pmail/utils/context"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestInlineKeyboardJSONCombinations(t *testing.T) {
	ctx := &pctx.Context{Context: t.Context(), Lang: "zh-CN"}
	id := math.MaxInt
	for n := range 8 {
		withCode, web, ai := n&1 != 0, n&2 != 0, n&4 != 0
		t.Run(fmt.Sprintf("code=%t/web=%t/ai=%t", withCode, web, ai), func(t *testing.T) {
			var analysis *inbound.Analysis
			if withCode {
				analysis = &inbound.Analysis{VerificationCode: &inbound.VerificationCode{Code: "123456"}}
			}
			markup := mailKeyboard(ctx, id, analysis, translation.Availability{TranslationAvailable: web, AIAvailable: ai})
			wire, err := json.Marshal(outgoingMessage{Markup: markup})
			if err != nil {
				t.Fatal(err)
			}
			var out struct {
				Markup *struct {
					Rows [][]map[string]any `json:"inline_keyboard"`
				} `json:"reply_markup"`
			}
			if err = json.Unmarshal(wire, &out); err != nil {
				t.Fatal(err)
			}
			if !withCode && !web && !ai {
				if strings.Contains(string(wire), `"reply_markup"`) {
					t.Fatal("empty keyboard was not omitted")
				}
				return
			}
			rowCount := 1
			if withCode && (web || ai) {
				rowCount = 2
			}
			if out.Markup == nil || len(out.Markup.Rows) != rowCount {
				t.Fatal("wrong keyboard row count")
			}
			if withCode {
				row := out.Markup.Rows[0]
				if len(row) != 1 || len(row[0]) != 2 || row[0]["text"] != "复制验证码 123456" {
					t.Fatal("invalid copy button")
				}
				value, ok := row[0]["copy_text"].(map[string]any)
				if !ok || value["text"] != "123456" || len(value) != 1 {
					t.Fatal("wrong copy_text payload")
				}
			}
			if web || ai {
				row := out.Markup.Rows[rowCount-1]
				var data []string
				if web {
					data = append(data, fmt.Sprintf("tr:%d", id))
				}
				if ai {
					data = append(data, fmt.Sprintf("ai:%d", id))
				}
				if len(row) != len(data) {
					t.Fatal("wrong action row size")
				}
				for idx, b := range row {
					text := "翻译"
					if strings.HasPrefix(data[idx], "ai:") {
						text = "AI 总结"
					}
					if len(b) != 2 || b["callback_data"] != data[idx] || b["text"] != text || len(data[idx]) > 64 {
						t.Fatal("wrong action button payload")
					}
				}
			}
		})
	}
}

const testChatID int64 = 91711

type recordedAnswer struct {
	ID   string `json:"callback_query_id"`
	Text string `json:"text"`
}
type testBot struct {
	mu            sync.Mutex
	updates       []update
	allowed       []string
	answers       []recordedAnswer
	messages      []outgoingMessage
	beforeUpdates func()
}

func (b *testBot) serve(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var result any = true
	switch {
	case strings.HasSuffix(r.URL.Path, "/getUpdates"):
		var req struct {
			Allowed []string `json:"allowed_updates"`
			Offset  int64    `json:"offset"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(400)
			return
		}
		b.allowed = req.Allowed
		if b.beforeUpdates != nil {
			b.beforeUpdates()
		}
		rows := []update{}
		for _, u := range b.updates {
			if u.ID >= req.Offset {
				rows = append(rows, u)
			}
		}
		result = rows
	case strings.HasSuffix(r.URL.Path, "/answerCallbackQuery"):
		var a recordedAnswer
		if json.NewDecoder(r.Body).Decode(&a) != nil {
			w.WriteHeader(400)
			return
		}
		b.answers = append(b.answers, a)
	case strings.HasSuffix(r.URL.Path, "/sendMessage"):
		var m outgoingMessage
		if json.NewDecoder(r.Body).Decode(&m) != nil {
			w.WriteHeader(400)
			return
		}
		b.messages = append(b.messages, m)
	default:
		w.WriteHeader(404)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}
func (b *testBot) snapshot() ([]recordedAnswer, []outgoingMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.answers), slices.Clone(b.messages)
}
func resetRate() {
	messages.Lock()
	defer messages.Unlock()
	messages.next, messages.pause = time.Time{}, time.Time{}
	clear(messages.chat)
}

func callbackFixture(t *testing.T) (*worker, *xorm.Engine, *testBot, int, int, models.TelegramConfig) {
	t.Helper()
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	oldDB, oldConfig := db.Instance, config.Instance
	db.Instance, config.Instance = e, &config.Config{SecretKey: "telegram-callback-fixture"}
	t.Cleanup(func() { db.Instance, config.Instance = oldDB, oldConfig; e.Close() })
	resetRate()
	t.Cleanup(resetRate)
	b := &testBot{}
	server := httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(server.Close)
	t.Setenv("PMAIL_TELEGRAM_API_BASE", server.URL)
	t.Setenv("PMAIL_TELEGRAM_PROXY", "")
	t.Setenv("PMAIL_PUBLIC_URL", "")
	if err = db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	owner, other := models.User{Account: "tg-test-owner", Lang: "zh-CN"}, models.User{Account: "tg-test-other", Lang: "en"}
	if _, err = e.Insert(&owner, &other); err != nil {
		t.Fatal(err)
	}
	token, err := seal("123456:abcdefghijklmnopqrstuvwxyz", "token")
	if err != nil {
		t.Fatal(err)
	}
	c := models.TelegramConfig{ID: 1, Generation: "new-generation", Token: token, Username: "Fixture_bot"}
	bound := models.TelegramBinding{UserID: owner.ID, ChatID: testChatID, Bot: c.Generation, Identity: "current-binding"}
	if _, err = e.Insert(&c, &bound); err != nil {
		t.Fatal(err)
	}
	return newWorker(), e, b, owner.ID, other.ID, c
}

func callbackMail(t *testing.T, e *xorm.Engine, uid int, risk, body string) models.Email {
	t.Helper()
	var a *inbound.Analysis
	if risk != "" {
		a = &inbound.Analysis{Risk: &inbound.Risk{Level: risk}, VerificationCode: &inbound.VerificationCode{Code: "918273"}}
	}
	m := models.Email{Subject: "original <subject>", Text: sql.NullString{String: body, Valid: body != ""}, InboundAnalysis: inbound.Storage(a)}
	if _, err := e.Insert(&m); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.UserEmail{EmailID: m.Id, UserID: uid}); err != nil {
		t.Fatal(err)
	}
	return m
}

func testCallback(id string, chatID int64, data string) *callbackQuery {
	return &callbackQuery{ID: id, From: user{ID: chatID, LanguageCode: "zh-CN"}, Message: &message{ID: 42, Chat: chat{ID: chatID, Type: "private"}}, Data: data}
}

func TestPollRejectsUnauthorizedCallbacksAndAnswersEveryID(t *testing.T) {
	w, e, b, owner, other, c := callbackFixture(t)
	own := callbackMail(t, e, owner, "none", "own content")
	foreign := callbackMail(t, e, other, "none", "other content")
	data := fmt.Sprintf("tr:%d", own.Id)
	mismatch := testCallback("mismatch", testChatID, data)
	mismatch.From.ID++
	group := testCallback("group", testChatID, data)
	group.Message.Chat.Type = "group"
	noMessage := testCallback("inline", testChatID, data)
	noMessage.Message = nil
	if _, err := e.Insert(&models.TelegramBinding{UserID: other, ChatID: testChatID + 5, Identity: "stale", Bot: "previous-generation"}); err != nil {
		t.Fatal(err)
	}
	queries := []*callbackQuery{
		testCallback("unbound", testChatID+1, data),
		testCallback("foreign", testChatID, fmt.Sprintf("ai:%d", foreign.Id)),
		testCallback("old-binding", testChatID+5, fmt.Sprintf("tr:%d", foreign.Id)),
		mismatch, group, noMessage, testCallback("malformed", testChatID, "tr:+1"),
	}
	for j, cb := range queries {
		b.updates = append(b.updates, update{ID: int64(j + 1), Callback: cb})
	}
	if err := w.pollOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	subscribed := slices.Contains(b.allowed, "callback_query") && slices.Contains(b.allowed, "message")
	b.mu.Unlock()
	if !subscribed {
		t.Fatal("getUpdates did not subscribe to messages and callback queries")
	}
	a, sent := b.snapshot()
	if len(a) != len(queries) || len(sent) != 0 || len(w.jobs) != 0 {
		t.Fatal("unauthorized callback produced work or did not get an answer")
	}
	for j, cb := range queries {
		if a[j].ID != cb.ID || a[j].Text != i18n.Text("zh-CN", "telegram_callback_denied") {
			t.Fatal("unauthorized callback response is not uniform")
		}
	}
	// Rotate the config during a live getUpdates response. The old client and
	// even its still-present binding must not authorize the returned query.
	var rotateErr error
	b.mu.Lock()
	b.updates = append(b.updates, update{ID: 8, Callback: testCallback("old-poll", testChatID, data)})
	b.beforeUpdates = func() {
		_, rotateErr = e.ID(c.ID).Cols("generation").Update(&models.TelegramConfig{Generation: "rotated"})
	}
	b.mu.Unlock()
	if err := w.pollOnce(t.Context()); err != nil || rotateErr != nil {
		t.Fatalf("rotate poll: %v / %v", err, rotateErr)
	}
	a, sent = b.snapshot()
	if len(a) != 8 || a[7].ID != "old-poll" || a[7].Text != i18n.Text("zh-CN", "telegram_callback_denied") || len(w.jobs) != 0 || len(sent) != 0 {
		t.Fatal("stale poll was not denied")
	}
}

func TestCallbackAcknowledgesBeforeWorkDeduplicatesAndRepliesSafely(t *testing.T) {
	w, e, b, owner, _, c := callbackFixture(t)
	m := callbackMail(t, e, owner, "high", "risky original")
	api, err := clientFor(c)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	w.now = func() time.Time { return now }
	translationCalls, aiCalls := 0, 0
	w.actions.translate = func(_ context.Context, uid int, lang string, in translation.TranslateRequest) (translation.Translation, error) {
		translationCalls++
		a, _ := b.snapshot()
		if len(a) == 0 || a[0].Text != i18n.Text("zh-CN", "telegram_translating") || uid != owner || lang != "zh-CN" || in.ID != m.Id || in.Engine != "" || in.Force {
			t.Error("translation was not acknowledged first, or settings not delegated")
		}
		if translationCalls > 1 {
			return translation.Translation{}, translation.ErrLimit
		}
		return translation.Translation{Subject: "<subject> &", Text: "<unsafe>\n" + strings.Repeat("界", 991) + "END"}, nil
	}
	w.actions.analyze = func(_ context.Context, uid int, lang string, in translation.AIRequest) (translation.AIResult, error) {
		aiCalls++
		a, _ := b.snapshot()
		if a[len(a)-1].Text != i18n.Text("zh-CN", "telegram_summarizing") || uid != owner || lang != "zh-CN" || in.ID != m.Id || in.Action != "summary" {
			t.Error("summary wasn't acknowledged first or used the wrong identity")
		}
		return translation.AIResult{Summary: &translation.Summary{Summary: "<ai summary>" + strings.Repeat("点", 1800), KeyPoints: []string{"<first> & more"}}}, nil
	}
	tr := fmt.Sprintf("tr:%d", m.Id)
	ai := fmt.Sprintf("ai:%d", m.Id)
	w.dispatchCallback(t.Context(), api, c.Generation, testCallback("first", testChatID, tr))
	now = now.Add(29 * time.Second)
	w.dispatchCallback(t.Context(), api, c.Generation, testCallback("duplicate", testChatID, tr))
	a, sent := b.snapshot()
	if len(a) != 2 || a[1].Text != i18n.Text("zh-CN", "telegram_callback_duplicate") || len(w.jobs) != 1 || len(sent) != 0 || translationCalls != 0 {
		t.Fatal("duplicate generated work or missing acknowledgement")
	}
	w.runCallback(t.Context(), <-w.jobs)
	w.dispatchCallback(t.Context(), api, c.Generation, testCallback("summary", testChatID, ai))
	if len(w.jobs) != 1 {
		t.Fatal("AI action should have its own cooldown")
	}
	resetRate()
	w.runCallback(t.Context(), <-w.jobs)
	now = now.Add(time.Second)
	w.dispatchCallback(t.Context(), api, c.Generation, testCallback("expired", testChatID, tr))
	if len(w.jobs) != 1 {
		t.Fatal("dedup did not expire with 30 seconds")
	}
	resetRate()
	w.runCallback(t.Context(), <-w.jobs)
	a, sent = b.snapshot()
	if len(a) != 4 || translationCalls != 2 || aiCalls != 1 || len(sent) != 3 {
		t.Fatal("incorrect acknowledgement or reply count")
	}
	for _, s := range sent {
		if s.ChatID != testChatID || s.Reply == nil || s.Reply.MessageID != 42 || s.ParseMode != "HTML" || !s.PreviewOptions["is_disabled"] || !strings.HasPrefix(s.Text, "⚠ ") {
			t.Fatal("incorrect risk warning or reply parameters")
		}
	}
	if !strings.Contains(sent[0].Text, "&lt;subject&gt; &amp;") || strings.Contains(sent[0].Text, "<unsafe>") || strings.Contains(sent[0].Text, "END") {
		t.Fatal("translation not escaped or bounded")
	}
	_, body, ok := strings.Cut(html.UnescapeString(sent[0].Text), "\n\n")
	if !ok || utf8.RuneCountInString(body) != 1000 {
		t.Fatal("wrong translation body length")
	}
	if utf8.RuneCountInString(html.UnescapeString(sent[1].Text)) > 1500 || !strings.Contains(sent[1].Text, "• &lt;first&gt; &amp; more") || strings.Contains(sent[1].Text, "<ai summary>") {
		t.Fatal("summary not escaped, lacks points, or too long")
	}
	if !strings.Contains(sent[2].Text, i18n.Text("zh-CN", "translation_daily_limit")) {
		t.Fatal("provider error did not produce a localized reply")
	}
}

func TestCallbackRevalidatesBindingBeforeSending(t *testing.T) {
	w, e, b, owner, _, c := callbackFixture(t)
	m := callbackMail(t, e, owner, "none", "body")
	api, err := clientFor(c)
	if err != nil {
		t.Fatal(err)
	}
	w.actions.translate = func(ctx context.Context, _ int, _ string, _ translation.TranslateRequest) (translation.Translation, error) {
		if _, err := e.Context(ctx).ID(owner).Delete(&models.TelegramBinding{}); err != nil {
			t.Fatal(err)
		}
		return translation.Translation{Text: "translated result"}, nil
	}
	w.dispatchCallback(t.Context(), api, c.Generation, testCallback("pending", testChatID, fmt.Sprintf("tr:%d", m.Id)))
	if len(w.jobs) != 1 {
		t.Fatal("valid callback was not scheduled")
	}
	w.runCallback(t.Context(), <-w.jobs)
	a, sent := b.snapshot()
	if len(a) != 1 || len(sent) != 0 {
		t.Fatal("reply was emitted after unbind or acknowledgement missing")
	}
}

func TestAutoTranslatePushPreviewAndFailureFallback(t *testing.T) {
	for _, tc := range []struct {
		name, risk, body                           string
		auto, preview, fail, wantSubject, wantBody bool
	}{
		{"disabled", "none", "original preview", false, true, false, false, false},
		{"failure", "none", "original preview", true, true, true, false, false},
		{"safe", "none", "original preview", true, true, false, true, true},
		{"risky", "high", "original preview", true, true, false, true, false},
		{"opted out of preview", "none", "original preview", true, false, false, true, false},
		{"no original preview", "none", "", true, true, false, true, false},
		{"unclassified", "", "original preview", true, true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, e, b, owner, _, _ := callbackFixture(t)
			m := callbackMail(t, e, owner, tc.risk, tc.body)
			settings, err := SetFilters(t.Context(), owner, Filters{AutoTranslate: tc.auto, IncludePreview: tc.preview})
			if err != nil || settings.Filters.AutoTranslate != tc.auto {
				t.Fatalf("preference was not persisted: %v", err)
			}
			w.actions.available = func(context.Context, int, int, string, models.Email) translation.Availability {
				return translation.Availability{TranslationAvailable: true}
			}
			calls := 0
			w.actions.translate = func(ctx context.Context, _ int, _ string, _ translation.TranslateRequest) (translation.Translation, error) {
				calls++
				if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > autoTranslateTimeout {
					t.Error("auto translation was not time limited")
				}
				if tc.fail {
					return translation.Translation{}, errors.New("private provider error")
				}
				return translation.Translation{Subject: "<translated subject>", Text: "<translated body>"}, nil
			}
			Notify(owner, m.Id, inbound.FromStorage(m.InboundAnalysis))
			if calls != 0 {
				t.Fatal("Notify ran an external action")
			}
			if !w.queueOnce(t.Context()) {
				t.Fatal("queue did not process push")
			}
			_, sent := b.snapshot()
			if len(sent) != 1 || (calls == 1) != tc.auto {
				t.Fatal("unexpected translation or send count")
			}
			text := sent[0].Text
			if !strings.Contains(text, "original &lt;subject&gt;") || strings.Contains(text, "private provider") || strings.Contains(text, "<translated subject>") || (strings.Contains(text, "&lt;translated subject&gt;")) != tc.wantSubject || (strings.Contains(text, "&lt;translated body&gt;")) != tc.wantBody {
				t.Fatal("fallback, escaping or translated preview incorrect")
			}
			if strings.Contains(text, "original preview") != (tc.preview && tc.risk == "none" && tc.body != "") {
				t.Fatal("original preview behavior changed")
			}
			if sent[0].Markup == nil || sent[0].Reply != nil {
				t.Fatal("push must include buttons but not be a reply")
			}
		})
	}
}

func TestRealTranslationServicePreferenceCacheAndPersonalAIAvailability(t *testing.T) {
	w, e, b, owner, other, c := callbackFixture(t)
	m := callbackMail(t, e, owner, "none", "original body")
	if _, err := e.Insert(&models.UserEmail{UserID: other, EmailID: m.Id}); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1/chat/completions" {
			t.Error("expected user's AI endpoint")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"subject":"translated","text":"body"}`}}}})
	}))
	t.Cleanup(s.Close)
	yes := true
	if _, err := translation.SetPolicy(t.Context(), translation.PolicyPatch{AIEnabled: &yes, AIAllowPrivateNetwork: &yes}); err != nil {
		t.Fatal(err)
	}
	if _, err := translation.SetAI(t.Context(), owner, translation.AIInput{BaseURL: s.URL + "/v1", Model: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if _, err := translation.SetPreferences(t.Context(), owner, translation.Preferences{TranslateEngine: "ai"}); err != nil {
		t.Fatal(err)
	}
	if available := w.actions.available(t.Context(), owner, m.Id, "zh-CN", m); !available.AIAvailable || !available.TranslationAvailable {
		t.Fatal("owner's own AI not available")
	}
	if available := w.actions.available(t.Context(), other, m.Id, "en", m); available.AIAvailable || available.TranslationAvailable {
		t.Fatal("another user's AI configuration enabled a button")
	}
	api, err := clientFor(c)
	if err != nil {
		t.Fatal(err)
	}
	w.dispatchCallback(t.Context(), api, c.Generation, testCallback("real", testChatID, fmt.Sprintf("tr:%d", m.Id)))
	if len(w.jobs) != 1 {
		t.Fatal("real-service test callback wasn't queued")
	}
	w.runCallback(t.Context(), <-w.jobs)
	if shared, err := translation.Default.Translate(t.Context(), owner, "zh-CN", translation.TranslateRequest{ID: m.Id, Engine: "ai"}); err != nil || !shared.Cached || requests.Load() != 1 {
		t.Fatalf("user translation engine or shared cache not used: %v", err)
	}
	_, sent := b.snapshot()
	if len(sent) != 1 || !strings.Contains(sent[0].Text, "translated") {
		t.Fatal("real-service callback missing reply")
	}
}
