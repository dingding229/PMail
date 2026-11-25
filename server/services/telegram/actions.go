package telegram

import (
	"context"
	"errors"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/maintenance"
	"github.com/Jinnrry/pmail/services/translation"
	pctx "github.com/Jinnrry/pmail/utils/context"
)

const (
	callbackCooldown     = 30 * time.Second
	callbackQueueSize    = 16
	callbackSeenMax      = 4096
	callbackWorkers      = 2
	autoTranslateTimeout = 8 * time.Second
)

type mailActions struct {
	available func(context.Context, int, int, string, models.Email) translation.Availability
	translate func(context.Context, int, string, translation.TranslateRequest) (translation.Translation, error)
	analyze   func(context.Context, int, string, translation.AIRequest) (translation.AIResult, error)
}

type worker struct {
	actions mailActions
	jobs    chan callbackJob
	// Only the poll loop accesses seen. A bounded table avoids retaining a
	// lifetime history; active cooldowns aren't evicted to make new space.
	seen map[callbackKey]time.Time
	now  func() time.Time
}

func newWorker() *worker {
	return &worker{
		actions: mailActions{translation.Available, translation.Default.Translate, translation.Default.Analyze},
		jobs:    make(chan callbackJob, callbackQueueSize), seen: make(map[callbackKey]time.Time), now: time.Now,
	}
}

func mailKeyboard(ctx *pctx.Context, id int, a *inbound.Analysis, available translation.Availability) *replyMarkup {
	var rows [][]inlineButton
	if a != nil && a.VerificationCode != nil {
		if code := a.VerificationCode.Code; code != "" && code == truncate(code, 32) {
			rows = append(rows, []inlineButton{{Text: i18n.T(ctx, "telegram_copy_code", code), CopyText: &copyTextButton{Text: code}}})
		}
	}
	if id > 0 {
		var row []inlineButton
		arg := strconv.Itoa(id)
		// Mail already in the reader's language gets no translate button.
		if available.TranslationAvailable && available.NeedsTranslation {
			row = append(row, inlineButton{Text: i18n.T(ctx, "telegram_translate"), CallbackData: "tr:" + arg})
		}
		if available.AIAvailable {
			row = append(row, inlineButton{Text: i18n.T(ctx, "telegram_ai_summary"), CallbackData: "ai:" + arg})
		}
		if len(row) != 0 {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return &replyMarkup{Keyboard: rows}
}

// telegramText preserves readable line breaks but strips controls including
// bidi overrides. Limits apply to rendered characters, before HTML escaping.
func telegramText(s string, n int) string {
	var out strings.Builder
	for _, r := range s {
		if n <= 0 {
			break
		}
		if r == '\t' {
			r = ' '
		}
		if r != '\n' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf)) {
			continue
		}
		out.WriteRune(r)
		n--
	}
	return out.String()
}

// previewText tidies a plain-text body for a chat preview. Text parts made
// from HTML layouts are full of padding spaces and empty lines, which used up
// the whole preview before any content showed. Runs of white space collapse
// to one space, blank lines and image placeholders are dropped, and the
// result is cut at a word or line boundary with an ellipsis.
func previewText(s string, n int) string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		line = strings.Join(strings.FieldsFunc(line, func(r rune) bool {
			return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.In(r, unicode.Cf)
		}), " ")
		if line == "" || imagePlaceholder.MatchString(line) {
			continue
		}
		lines = append(lines, line)
	}
	text := strings.Join(lines, "\n")
	if n <= 0 || utf8.RuneCountInString(text) <= n {
		return text
	}
	r := []rune(text)[:n]
	cut := len(r)
	for i := len(r) - 1; i > n*3/4; i-- {
		if r[i] == ' ' || r[i] == '\n' {
			cut = i
			break
		}
	}
	return strings.TrimRightFunc(string(r[:cut]), unicode.IsSpace) + "…"
}

var imagePlaceholder = regexp.MustCompile(`(?i)^\[\s*(image|img|图片)[^\]]{0,200}\]$`)

func riskWarning(ctx *pctx.Context, a *inbound.Analysis) string {
	if a == nil || a.Risk == nil || a.Risk.Level == "none" {
		return ""
	}
	return "⚠ " + i18n.T(ctx, "telegram_mail_risk") + " " + truncate(a.Risk.Level, 16) + "\n"
}

func translatedMail(ctx *pctx.Context, tr translation.Translation, bodyLimit int) string {
	text := i18n.T(ctx, "telegram_translated_subject") + " " + truncate(tr.Subject, 240)
	if bodyLimit > 0 && strings.TrimSpace(tr.Text) != "" {
		text += "\n\n" + previewText(tr.Text, bodyLimit)
	}
	return text
}

func canPreview(preview bool, row models.Email, a *inbound.Analysis) bool {
	return preview && a != nil && a.Risk != nil && a.Risk.Level == "none" && row.Text.Valid && strings.TrimSpace(row.Text.String) != ""
}

func autoPreview(ctx *pctx.Context, tr *translation.Translation, preview bool) string {
	if tr == nil {
		return ""
	}
	limit := 0
	if preview {
		limit = previewChars
	}
	return "\n\n" + html.EscapeString(translatedMail(ctx, *tr, limit))
}

type callbackKey struct {
	userID, emailID int
	action          string
}

type callbackJob struct {
	api       *WorkerClient
	key       callbackKey
	binding   models.TelegramBinding
	messageID int
}

func callbackTarget(data string) (string, int, bool) {
	if len(data) > 64 {
		return "", 0, false
	}
	action, arg, ok := strings.Cut(data, ":")
	if !ok || (action != "tr" && action != "ai") {
		return "", 0, false
	}
	id, err := strconv.Atoi(arg)
	return action, id, err == nil && id > 0 && strconv.Itoa(id) == arg
}

// Verify the current bot generation, the enabled mailbox owner and the
// owner's live mail link, not just the generation that started the long poll.
func allowedCallback(ctx context.Context, chatID int64, generation string, mailID int) (models.TelegramBinding, string, bool, error) {
	if generation == "" {
		return models.TelegramBinding{}, "", false, nil
	}
	// Use one DB snapshot for generation, binding, owner and mail membership.
	// Separate reads could combine an old binding with a newly changed owner.
	var row struct {
		models.TelegramBinding `xorm:"extends"`
		Lang                   string `xorm:"lang"`
	}
	q := `SELECT b.*, u.lang FROM telegram_binding b JOIN ` + db.Instance.Quote("user") + ` u ON u.id=b.user_id
		WHERE b.chat_id=? AND b.bot_generation=? AND u.disabled=0
		AND EXISTS (SELECT 1 FROM telegram_config c WHERE c.id=1 AND c.generation=b.bot_generation AND c.token<>'')
		AND EXISTS (SELECT 1 FROM user_email ue WHERE ue.user_id=b.user_id AND ue.email_id=?)`
	ok, err := db.Instance.Context(ctx).SQL(q, chatID, generation, mailID).Get(&row)
	return row.TelegramBinding, row.Lang, ok && err == nil, err
}

func (w *worker) dispatchCallback(ctx context.Context, api *WorkerClient, generation string, cb *callbackQuery) {
	if cb.ID == "" {
		return
	} // No Telegram identifier to acknowledge.
	locale := &pctx.Context{Context: ctx, Lang: cb.From.LanguageCode}
	answer := func(key string) error { return api.answer(ctx, cb.ID, i18n.T(locale, key)) }
	action, id, ok := callbackTarget(cb.Data)
	if !ok || cb.From.IsBot || cb.From.ID <= 0 || cb.Message == nil || cb.Message.ID <= 0 || cb.Message.Chat.Type != "private" || cb.Message.Chat.ID != cb.From.ID {
		_ = answer("telegram_callback_denied")
		return
	}
	check, cancel := context.WithTimeout(ctx, 2*time.Second)
	b, lang, ok, _ := allowedCallback(check, cb.From.ID, generation, id)
	cancel()
	if !ok {
		_ = answer("telegram_callback_denied")
		return
	}
	locale.Lang = lang
	key := callbackKey{b.UserID, id, action}
	now := w.now()
	if now.Before(w.seen[key]) {
		_ = answer("telegram_callback_duplicate")
		return
	}
	if len(w.seen) >= callbackSeenMax {
		for k, expiry := range w.seen {
			if !now.Before(expiry) {
				delete(w.seen, k)
			}
		}
	}
	if len(w.jobs) == cap(w.jobs) || len(w.seen) >= callbackSeenMax {
		_ = answer("telegram_callback_busy")
		return
	}
	progress := "telegram_translating"
	if action == "ai" {
		progress = "telegram_summarizing"
	}
	// Answer before queuing anything that might call an external provider.
	if answer(progress) != nil || ctx.Err() != nil {
		return
	}
	w.seen[key] = w.now().Add(callbackCooldown)
	w.jobs <- callbackJob{api: api, key: key, binding: b, messageID: cb.Message.ID}
}

func (j callbackJob) current(ctx context.Context) (string, bool) {
	b, lang, ok, _ := allowedCallback(ctx, j.binding.ChatID, j.binding.Bot, j.key.emailID)
	return lang, ok && b.Identity == j.binding.Identity && b.UserID == j.key.userID
}

func (w *worker) runCallback(ctx context.Context, job callbackJob) {
	lang, ok := job.current(ctx)
	if !ok {
		return
	}
	locale := &pctx.Context{Context: ctx, Lang: lang}
	var mail models.Email
	ok, err := db.Instance.Context(ctx).ID(job.key.emailID).Cols("inbound_analysis").Get(&mail)
	if err != nil || !ok {
		return
	}
	prefix := riskWarning(locale, inbound.FromStorage(mail.InboundAnalysis))
	var text string
	if job.key.action == "tr" {
		// The empty Engine explicitly delegates selection to the C27 service:
		// user preference, then administrator default. No new quota or cache.
		var tr translation.Translation
		tr, err = w.actions.translate(ctx, job.key.userID, lang, translation.TranslateRequest{ID: job.key.emailID})
		if err == nil {
			text = prefix + translatedMail(locale, tr, 1000)
		}
	} else {
		var result translation.AIResult
		result, err = w.actions.analyze(ctx, job.key.userID, lang, translation.AIRequest{ID: job.key.emailID, Action: "summary"})
		if err == nil && result.Summary != nil {
			const limit = 1500
			title := prefix + i18n.T(locale, "telegram_ai_summary") + ":\n"
			var points strings.Builder
			for _, point := range result.Summary.KeyPoints[:min(6, len(result.Summary.KeyPoints))] {
				points.WriteString("\n• " + telegramText(point, 120))
			}
			summaryLimit := limit - utf8.RuneCountInString(title) - utf8.RuneCountInString(points.String())
			text = title + telegramText(result.Summary.Summary, summaryLimit) + points.String()
		} else if err == nil {
			err = translation.ErrFormat
		}
	}
	if err != nil {
		key := translation.ErrorCode(err)
		if key == "internal_error" || errors.Is(err, translation.ErrNotOwned) {
			key = "telegram_action_failed"
		}
		text = prefix + i18n.T(locale, key)
	}
	// Recheck after upstream work and any rate-limit wait: an unbind, bot
	// replacement, account disable or permanent removal must stop the reply.
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for sendCtx.Err() == nil {
		if _, ok := job.current(sendCtx); !ok {
			return
		}
		allowed, next := messages.allow(job.binding.ChatID, time.Now())
		if !allowed {
			wait(sendCtx, time.Until(next))
			continue
		}
		err := job.api.sendMessage(sendCtx, job.binding.ChatID, html.EscapeString(text), nil, job.messageID)
		var failure *botError
		if errors.As(err, &failure) {
			if failure.Status == 429 {
				messages.backoff(time.Now().Add(providerDelay(failure)))
			}
			if failure.Status == 403 {
				_ = unbind(ctx, job.binding.UserID, 0, job.binding.Identity)
			}
		}
		return
	}
}

func (w *worker) callbackLoop(ctx context.Context) {
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case job := <-w.jobs:
			if ctx.Err() == nil {
				maintenance.Work(ctx, func(ctx context.Context) { w.runCallback(ctx, job) })
			}
		}
	}
}
