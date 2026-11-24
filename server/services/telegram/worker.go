package telegram

import (
	"context"
	"errors"
	"html"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/maintenance"
	"github.com/Jinnrry/pmail/services/translation"
	pctx "github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func truncate(s string, n int) string {
	var b strings.Builder
	for _, r := range s {
		if n == 0 {
			break
		}
		if !unicode.IsControl(r) && !unicode.In(r, unicode.Cf) {
			b.WriteRune(r)
			n--
		} else if r == '\n' || r == '\t' {
			b.WriteByte(' ')
			n--
		}
	}
	return b.String()
}
func quiet(q *QuietHours, now time.Time) bool {
	if q == nil {
		return false
	}
	loc, err := time.LoadLocation(q.TZ)
	if err != nil {
		return false
	}
	clock := now.In(loc).Format("15:04")
	if q.Start < q.End {
		return clock >= q.Start && clock < q.End
	}
	return clock >= q.Start || clock < q.End
}
func matches(f Filters, l models.UserEmail, a *inbound.Analysis, now time.Time) bool {
	if quiet(f.Quiet, now) || f.OnlyVerification && (a == nil || a.VerificationCode == nil) {
		return false
	}
	if l.Status == consts.EmailStatusDel || l.Status == consts.EmailStatusDrafts {
		return false
	}
	spam := l.Status == consts.EmailStatusJunk || l.GroupId == models.Junk || (a != nil && a.Recipient != nil && a.Recipient.Verdict == "spam")
	if spam && !f.IncludeSpam {
		return false
	}
	if f.Groups == nil {
		return true
	}
	g := l.GroupId
	if g == 0 {
		if spam {
			g = models.Junk
		} else {
			g = models.INBOX
		}
	}
	return slices.Contains(f.Groups, g) || (g == models.INBOX && slices.Contains(f.Groups, 0))
}
func publicLink(id int) string {
	u, err := url.Parse(strings.TrimSpace(os.Getenv("PMAIL_PUBLIC_URL")))
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/mail/inbox"
	}
	u.RawQuery = ""
	u.Fragment = ""
	q := u.Query()
	q.Set("mail", strconv.Itoa(id))
	u.RawQuery = q.Encode()
	return u.String()
}
func formatMail(ctx *pctx.Context, row models.Email, l models.UserEmail, a *inbound.Analysis, preview bool, tr *translation.Translation) string {
	esc := func(s string, n int) string { return html.EscapeString(truncate(s, n)) }
	label := func(k string) string { return esc(i18n.T(ctx, k), 100) }
	from := row.FromAddress
	if row.FromName != "" {
		from = row.FromName + " <" + from + ">"
	}
	text := "<b>" + label("telegram_mail_from") + "</b> " + esc(from, 220) + "\n<b>" + label("telegram_mail_subject") + "</b> " + esc(row.Subject, 240)
	if a != nil && a.Risk != nil && a.Risk.Level != "none" {
		text += "\n⚠ " + label("telegram_mail_risk") + " " + esc(a.Risk.Level, 16)
	}
	if l.Status == consts.EmailStatusJunk || l.GroupId == models.Junk || (a != nil && a.Recipient != nil && a.Recipient.Verdict == "spam") {
		text += "\n⚑ " + label("telegram_mail_junk")
	}
	if a != nil && a.VerificationCode != nil {
		text += "\n" + label("telegram_mail_code") + " <code>" + esc(a.VerificationCode.Code, 8) + "</code>"
	}
	if canPreview(preview, row, a) {
		text += "\n\n" + esc(strings.TrimSpace(row.Text.String), 300)
	}
	text += autoPreview(ctx, tr, canPreview(preview, row, a))
	if href := publicLink(row.Id); href != "" {
		text += "\n<a href=\"" + html.EscapeString(href) + "\">" + label("telegram_open_mail") + "</a>"
	}
	return text
}
func loadLink(ctx context.Context, uid, id int) (models.UserEmail, bool, error) {
	var l models.UserEmail
	ok, err := db.Instance.Context(ctx).Where("user_id=? AND email_id=?", uid, id).Desc("id").Cols("status", "group_id", "inbound_spam").Get(&l)
	return l, ok, err
}

// Notify runs post-commit. The durable queue stores only IDs so a future
// filter change cannot leak a queued preview or reuse a replaced binding.
func Notify(uid, emailID int, a *inbound.Analysis) {
	if emailID <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	c, err := getConfig(ctx)
	if err != nil || c.Token == "" {
		return
	}
	b, ok, err := binding(ctx, c, uid)
	if err != nil || !ok {
		return
	}
	f, err := filters(ctx, uid)
	if err != nil {
		return
	}
	l, ok, err := loadLink(ctx, uid, emailID)
	if err != nil || !ok || !matches(f, l, a, time.Now()) {
		return
	}
	enqueuePush(ctx, b, emailID)
}

func enqueuePush(ctx context.Context, b models.TelegramBinding, ref int) {
	uid := b.UserID
	n, err := db.Instance.Context(ctx).Where("user_id=?", uid).Count(&models.TelegramPush{})
	if err != nil || n >= 200 {
		return
	}
	now := time.Now()
	_, err = db.Instance.Context(ctx).Insert(&models.TelegramPush{UserID: uid, EmailID: ref, Binding: b.Identity, Due: now.UnixMilli(), Expires: now.Add(24 * time.Hour).UnixMilli()})
	if err != nil && !db.IsUniqueViolation(err) {
		log.Warnf("telegram enqueue unavailable user_id=%d", uid)
	}
}

type limiter struct {
	sync.Mutex
	next, pause time.Time
	chat        map[int64]time.Time
}

var messages = limiter{chat: map[int64]time.Time{}}

func maxTime(times ...time.Time) time.Time {
	var latest time.Time
	for _, t := range times {
		if t.After(latest) {
			latest = t
		}
	}
	return latest
}
func (l *limiter) readyAt(id int64) time.Time {
	return maxTime(l.chat[id], l.next, l.pause)
}
func (l *limiter) earliest(id int64) time.Time {
	l.Lock()
	defer l.Unlock()
	return l.readyAt(id)
}
func (l *limiter) allow(id int64, t time.Time) (bool, time.Time) {
	l.Lock()
	defer l.Unlock()
	if ready := l.readyAt(id); t.Before(ready) {
		return false, ready
	}
	if len(l.chat) >= 4096 {
		for k, v := range l.chat {
			if !t.Before(v) {
				delete(l.chat, k)
			}
		}
	}
	l.chat[id] = t.Add(time.Second)
	l.next = t.Add(40 * time.Millisecond)
	return true, t
}
func (l *limiter) backoff(until time.Time) {
	l.Lock()
	defer l.Unlock()
	if until.After(l.pause) {
		l.pause = until
	}
}
func providerDelay(e *botError) time.Duration {
	return time.Duration(min(86400, max(1, e.RetryAfter))) * time.Second
}
func UserTest(ctx context.Context, uid int, text string) error {
	c, err := getConfig(ctx)
	if err != nil {
		return err
	}
	if c.Token == "" {
		return ErrNotConfigured
	}
	b, ok, err := binding(ctx, c, uid)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotBound
	}
	api, err := clientFor(c)
	if err != nil {
		return err
	}
	if ok, _ := messages.allow(b.ChatID, time.Now()); !ok {
		return ErrRateLimited
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err = api.send(ctx, b.ChatID, html.EscapeString(text)); err != nil {
		var e *botError
		if errors.As(err, &e) {
			switch e.Status {
			case 403:
				_ = unbind(ctx, b.UserID, 0, b.Identity)
				return ErrNotBound
			case 429:
				messages.backoff(time.Now().Add(providerDelay(e)))
				return ErrRateLimited
			}
		}
		return ErrUnavailable
	}
	return nil
}
func wait(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
func (w *worker) pollOnce(ctx context.Context) error {
	c, err := getConfig(ctx)
	if err != nil {
		return err
	}
	api, err := clientFor(c)
	if err != nil {
		return err
	}
	var rows []update
	if err = api.call(ctx, "getUpdates", map[string]any{"offset": c.Offset, "timeout": 25, "limit": 50, "allowed_updates": []string{"message", "callback_query"}}, &rows); err != nil {
		return err
	}
	for _, upd := range rows {
		if upd.ID < c.Offset {
			continue
		}
		if m := upd.Message; m != nil && m.Chat.Type == "private" && m.Chat.ID > 0 {
			fields := strings.Fields(truncate(m.Text, 500))
			if len(fields) > 0 {
				cmd := fields[0]
				if before, after, ok := strings.Cut(cmd, "@"); ok && strings.EqualFold(after, c.Username) {
					cmd = before
				}
				switch {
				case cmd == "/start" && len(fields) == 2:
					err = consume(ctx, fields[1], m.Chat, c.Generation, time.Now())
					if err != nil && !errors.Is(err, ErrBindExpired) && !errors.Is(err, ErrInvalid) {
						return err
					}
				case cmd == "/stop" && len(fields) == 1:
					var b models.TelegramBinding
					ok, err := db.Instance.Context(ctx).Where("chat_id=? AND bot_generation=?", m.Chat.ID, c.Generation).Get(&b)
					if err != nil {
						return err
					}
					if ok {
						if err = unbind(ctx, b.UserID, 0, b.Identity); err != nil {
							return err
						}
					}
				}
			}
		}
		if q := upd.Callback; q != nil {
			w.dispatchCallback(ctx, api, c.Generation, q)
		}
		c.Offset = upd.ID + 1
		if _, err = db.Instance.Context(ctx).Table(&models.TelegramConfig{}).ID(1).Where("generation=? AND next_offset<?", c.Generation, c.Offset).Update(map[string]any{"next_offset": c.Offset}); err != nil {
			return err
		}
	}
	return nil
}
func drop(ctx context.Context, id int) {
	_, _ = db.Instance.Context(ctx).ID(id).Delete(&models.TelegramPush{})
}
func reschedule(ctx context.Context, row models.TelegramPush, attempt int, when time.Time) {
	_, _ = db.Instance.Context(ctx).Table(&models.TelegramPush{}).ID(row.ID).Update(map[string]any{"due": when.UnixMilli(), "attempts": attempt})
}

// A malformed preference or another row-specific failure must not leave the
// same due row in front of every other recipient. Retry with a bounded backoff;
// use the observed lease fields so we never reschedule another worker's claim.
func retryPushPreparation(ctx context.Context, row models.TelegramPush) bool {
	attempt := row.Attempts + 1
	due := time.Now().Add(time.Duration(1<<min(attempt, 5)) * time.Second)
	n, err := db.Instance.Context(ctx).Table(&models.TelegramPush{}).ID(row.ID).
		Where("due=? AND attempts=?", row.Due, row.Attempts).Update(map[string]any{"due": due.UnixMilli(), "attempts": attempt})
	if err == nil && n == 1 && attempt >= 5 {
		log.Warnf("telegram push preparation exhausted user_id=%d", row.UserID)
	}
	return err == nil && n == 1
}

func (w *worker) queueOnce(ctx context.Context) bool {
	now := time.Now()
	var row models.TelegramPush
	found, err := db.Instance.Context(ctx).Where("due<=?", now.UnixMilli()).Asc("due", "id").Get(&row)
	if err != nil || !found {
		return false
	}
	if row.Expires <= now.UnixMilli() || row.Attempts >= 5 {
		drop(ctx, row.ID)
		return true
	}
	c, err := getConfig(ctx)
	if err != nil {
		return false
	}
	b, ok, err := binding(ctx, c, row.UserID)
	if err != nil {
		return retryPushPreparation(ctx, row)
	}
	if !ok || b.Identity != row.Binding {
		drop(ctx, row.ID)
		return true
	}
	if row.EmailID < 0 {
		return w.queueAddressRequest(ctx, row, c, b)
	}
	f, err := filters(ctx, row.UserID)
	if err != nil {
		return retryPushPreparation(ctx, row)
	}
	if quiet(f.Quiet, now) {
		drop(ctx, row.ID)
		return true
	}
	l, ok, err := loadLink(ctx, row.UserID, row.EmailID)
	if err != nil {
		return retryPushPreparation(ctx, row)
	}
	if !ok {
		drop(ctx, row.ID)
		return true
	}
	var owner models.User
	if ok, err = db.Instance.Context(ctx).ID(row.UserID).Where("disabled=0").Cols("lang").Get(&owner); err != nil {
		return retryPushPreparation(ctx, row)
	}
	if !ok {
		drop(ctx, row.ID)
		return true
	}
	var e models.Email
	if ok, err = db.Instance.Context(ctx).ID(row.EmailID).Cols("id", "subject", "from_name", "from_address", "inbound_analysis").Get(&e); err != nil {
		return retryPushPreparation(ctx, row)
	}
	if !ok {
		drop(ctx, row.ID)
		return true
	}
	a := inbound.FromStorage(e.InboundAnalysis)
	if a != nil {
		a.Recipient = inbound.ReadDetail(e.InboundAnalysis, l.InboundSpam).Spam
	}
	if !matches(f, l, a, now) {
		drop(ctx, row.ID)
		return true
	}
	if f.IncludePreview && a != nil && a.Risk != nil && a.Risk.Level == "none" {
		var body models.Email
		_, err = db.Instance.Context(ctx).ID(row.EmailID).Select("SUBSTR(text,1,600) AS text").Get(&body)
		if err != nil {
			return retryPushPreparation(ctx, row)
		}
		e.Text = body.Text
	}
	return deliverPush(ctx, row, c, b, func() (*pushPayload, error) {
		locale := &pctx.Context{Context: ctx, Lang: owner.Lang}
		available := w.actions.available(ctx, row.UserID, e.Id, owner.Lang, e)
		var translated *translation.Translation
		if f.AutoTranslate && available.TranslationAvailable {
			trCtx, cancel := context.WithTimeout(ctx, autoTranslateTimeout)
			if tr, trErr := w.actions.translate(trCtx, row.UserID, owner.Lang, translation.TranslateRequest{ID: e.Id}); trErr == nil {
				translated = &tr
			}
			cancel()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Re-read preferences and mail link after an external call. A changed
			// preview/quiet/group setting must be honored even on a leased row.
			f, err = filters(ctx, row.UserID)
			if err != nil {
				return nil, err
			}
			l, ok, err = loadLink(ctx, row.UserID, row.EmailID)
			if err != nil {
				return nil, err
			}
			if a != nil {
				a.Recipient = inbound.ReadDetail(e.InboundAnalysis, l.InboundSpam).Spam
			}
			if !ok || !matches(f, l, a, time.Now()) {
				return nil, nil
			}
			if !f.AutoTranslate {
				translated = nil
			}
			available = w.actions.available(ctx, row.UserID, e.Id, owner.Lang, e)
			if !available.TranslationAvailable {
				translated = nil
			}
		}
		current, _, authorized, err := allowedCallback(ctx, b.ChatID, c.Generation, row.EmailID)
		if err != nil {
			return nil, err
		}
		if !authorized || current.Identity != row.Binding || current.UserID != row.UserID {
			return nil, nil
		}
		return &pushPayload{text: formatMail(locale, e, l, a, f.IncludePreview, translated), markup: mailKeyboard(locale, e.Id, a, available)}, nil
	})
}

type pushPayload struct {
	text   string
	markup *replyMarkup
}

func deliverPush(ctx context.Context, row models.TelegramPush, c models.TelegramConfig, b models.TelegramBinding, prepare func() (*pushPayload, error)) bool {
	now := time.Now()
	if next := messages.earliest(b.ChatID); time.Now().Before(next) {
		reschedule(ctx, row, row.Attempts, next)
		return true
	}
	// Persist the attempt and a lease before sending. Restart may retry a
	// previously accepted message, but cannot exceed the attempt limit.
	attempt := row.Attempts + 1
	n, err := db.Instance.Context(ctx).Table(&models.TelegramPush{}).ID(row.ID).
		Where("due=? AND attempts=?", row.Due, row.Attempts).Update(map[string]any{"due": now.Add(30 * time.Second).UnixMilli(), "attempts": attempt})
	if err != nil || n != 1 {
		return false
	}
	payload, err := prepare()
	if err != nil {
		return false
	}
	if payload == nil {
		drop(ctx, row.ID)
		return true
	}
	if ok, next := messages.allow(b.ChatID, time.Now()); !ok {
		// No Bot API send was attempted; throttling doesn't consume a retry.
		reschedule(ctx, row, row.Attempts, next)
		return true
	}
	api, err := clientFor(c)
	if err == nil {
		sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = api.sendMessage(sendCtx, b.ChatID, payload.text, payload.markup, 0)
		cancel()
	}
	if ctx.Err() != nil {
		return false
	}
	if err == nil {
		drop(ctx, row.ID)
		return true
	}
	var failure *botError
	_ = errors.As(err, &failure)
	if failure != nil && failure.Status == 403 {
		_ = unbind(ctx, b.UserID, 0, b.Identity)
		drop(ctx, row.ID)
		return true
	}
	due := time.Now().Add(time.Duration(1<<min(attempt, 5)) * time.Second)
	if failure != nil && failure.Status == 429 {
		due = time.Now().Add(providerDelay(failure))
		messages.backoff(due)
	}
	if attempt >= 5 {
		drop(ctx, row.ID)
		log.Warnf("telegram push exhausted user_id=%d", row.UserID)
	} else {
		reschedule(ctx, row, attempt, due)
	}
	return true
}
func Start(ctx context.Context) <-chan struct{} {
	w := newWorker()
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Go(func() {
		for maintenance.Wait(ctx) {
			var err error
			if !maintenance.Work(ctx, func(ctx context.Context) { err = w.pollOnce(ctx) }) {
				continue
			}
			if err != nil {
				delay := 3 * time.Second
				var api *botError
				if errors.As(err, &api) && api.Status == 429 {
					delay = providerDelay(api)
				}
				wait(ctx, delay)
			} else {
				wait(ctx, 150*time.Millisecond)
			}
		}
	})
	wg.Go(func() {
		var last time.Time
		for maintenance.Wait(ctx) {
			var worked bool
			maintenance.Work(ctx, func(ctx context.Context) {
				if time.Since(last) > time.Hour {
					now := time.Now()
					_, _ = db.Instance.Context(ctx).Where("expires<=?", now.UnixMilli()).Delete(&models.TelegramPush{})
					_, _ = db.Instance.Context(ctx).Where("expires<=?", now.Unix()).Delete(&models.TelegramCode{})
					last = now
				}
				worked = w.queueOnce(ctx)
			})
			if worked {
				wait(ctx, 40*time.Millisecond)
			} else {
				wait(ctx, 500*time.Millisecond)
			}
		}
	})
	for range callbackWorkers {
		wg.Go(func() { w.callbackLoop(ctx) })
	}
	go func() { wg.Wait(); close(done) }()
	return done
}
