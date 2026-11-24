// Package translation brokers mail text to administrator-selected web
// translation services and per-user AI endpoints.
package translation

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/netproxy"
	"xorm.io/xorm"
)

type Service struct {
	end            endpoints
	resolver       resolver
	dial           dialFunc
	systemTelegram func(context.Context) (*url.URL, error)
	now            func() time.Time
	edge           edgeCache
	mu             sync.Mutex
	running        map[int]int
}

func newService() *Service {
	d := net.Dialer{Timeout: 5 * time.Second}
	return &Service{end: endpoints{googleEndpoint, edgeAuthEndpoint, edgeEndpoint, deepLEndpoint, deepLFreeEndpoint}, resolver: net.DefaultResolver, dial: d.DialContext, systemTelegram: netproxy.SystemTelegram, now: time.Now, running: map[int]int{}}
}

var Default = newService()

func (s *Service) acquire(uid int) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[uid] >= 2 {
		return nil, ErrBusy
	}
	s.running[uid]++
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running[uid]--
		if s.running[uid] == 0 {
			delete(s.running, uid)
		}
	}, nil
}

var languages = map[string]string{"zh-cn": "zh-CN", "zh-tw": "zh-TW", "en": "en", "ja": "ja", "ko": "ko", "fr": "fr", "de": "de", "es": "es", "ru": "ru"}

func language(v string) (string, error) {
	if l, ok := languages[strings.ToLower(strings.TrimSpace(v))]; ok {
		return l, nil
	}
	return "", ErrInvalid
}
func targetLanguage(request, preference, ui string) (string, error) {
	if request != "" {
		return language(request)
	}
	if preference != "" {
		return language(preference)
	}
	if l, e := language(ui); e == nil {
		return l, nil
	}
	if strings.HasPrefix(strings.ToLower(ui), "zh") {
		return "zh-CN", nil
	}
	return "en", nil
}

type Availability struct {
	TranslationAvailable bool     `json:"translation_available"`
	NeedsTranslation     bool     `json:"needs_translation"`
	TranslationEngines   []string `json:"translation_engines"`
	AIAvailable          bool     `json:"ai_available"`
}

func Available(ctx context.Context, uid, id int, ui string, mail models.Email) Availability {
	a := Availability{TranslationEngines: []string{}}
	ok, err := owns(ctx, uid, id)
	if err != nil || !ok {
		return a
	}
	p, err := policy(ctx)
	if err != nil {
		return a
	}
	ai, err := getAI(ctx, uid)
	a.AIAvailable = err == nil && p.AIUserConfigEnabled && ai.BaseURL != "" && ai.Model != ""
	if p.AIEnabled && a.AIAvailable {
		a.TranslationEngines = append(a.TranslationEngines, "ai")
	}
	if p.WebEnabled {
		a.TranslationEngines = append(a.TranslationEngines, "web")
	}
	a.TranslationAvailable = len(a.TranslationEngines) != 0
	if a.TranslationAvailable {
		pref, _ := GetPreferences(ctx, uid)
		lang, _ := targetLanguage("", pref.TargetLang, ui)
		_, text, _ := mailText(mail, 4096)
		if strings.TrimSpace(text) == "" {
			text = mail.Subject
		}
		a.NeedsTranslation = differentScript(text, lang)
	}
	return a
}
func differentScript(text, lang string) bool {
	letters, matching, kana := 0, 0, 0
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		isKana := unicode.In(r, unicode.Hiragana, unicode.Katakana)
		if isKana {
			kana++
		}
		var match bool
		switch lang {
		case "zh-CN", "zh-TW":
			match = unicode.Is(unicode.Han, r)
		case "ja":
			match = isKana || unicode.Is(unicode.Han, r)
		case "ko":
			match = unicode.Is(unicode.Hangul, r)
		case "ru":
			match = unicode.Is(unicode.Cyrillic, r)
		default:
			match = unicode.Is(unicode.Latin, r)
		}
		if match {
			matching++
		}
	}
	return letters >= 4 && (matching*100 < letters*35 || ((lang == "zh-CN" || lang == "zh-TW") && kana*4 > letters))
}

func owns(ctx context.Context, uid, id int) (bool, error) {
	return db.Instance.Context(ctx).Where("user_id=? AND email_id=?", uid, id).Exist(&models.UserEmail{})
}
func ownedMail(ctx context.Context, uid, id int) (models.Email, error) {
	var row models.Email
	if uid <= 0 || id <= 0 {
		return row, ErrNotOwned
	}
	q := db.Instance.Context(ctx).ID(id).Cols("id", "subject", "text", "html", "inbound_analysis").Where("EXISTS (SELECT 1 FROM user_email WHERE email_id=? AND user_id=?) AND EXISTS (SELECT 1 FROM "+db.Instance.Quote("user")+" WHERE id=? AND disabled=0)", id, uid, uid)
	ok, err := q.Get(&row)
	if err != nil {
		return row, err
	}
	if !ok {
		return row, ErrNotOwned
	}
	return row, nil
}
func mailText(row models.Email, budget int) (string, string, bool) {
	text := row.Text.String
	if strings.TrimSpace(text) == "" && row.Html.String != "" {
		text = parsemail.HTMLText(row.Html.String)
	}
	subject := row.Subject
	n := utf8.RuneCountInString(subject)
	m := utf8.RuneCountInString(text)
	if n+m <= budget {
		return subject, text, false
	}
	subject = clipped(subject, budget)
	return subject, clipped(text, max(0, budget-n)), true
}

type TranslateRequest struct {
	ID         int    `json:"id"`
	TargetLang string `json:"target_lang"`
	Engine     string `json:"engine"`
	Force      bool   `json:"force"`
}
type Translation struct {
	Subject    string `json:"subject"`
	Text       string `json:"text"`
	SourceLang string `json:"source_lang,omitempty"`
	TargetLang string `json:"target_lang"`
	Provider   string `json:"provider"`
	Engine     string `json:"engine"`
	Cached     bool   `json:"cached"`
	Truncated  bool   `json:"truncated"`
}

func (s *Service) Translate(ctx context.Context, uid int, ui string, in TranslateRequest) (Translation, error) {
	row, err := ownedMail(ctx, uid, in.ID)
	if err != nil {
		return Translation{}, err
	}
	p, err := policy(ctx)
	if err != nil {
		return Translation{}, err
	}
	pref, err := GetPreferences(ctx, uid)
	if err != nil {
		return Translation{}, err
	}
	lang, err := targetLanguage(in.TargetLang, pref.TargetLang, ui)
	if err != nil {
		return Translation{}, err
	}
	engine := in.Engine
	if engine == "" {
		engine = pref.TranslateEngine
	}
	if engine == "" {
		engine = p.DefaultEngine
	}
	if !engineValid(engine) {
		return Translation{}, ErrInvalid
	}
	if engine == "web" && !p.WebEnabled || engine == "ai" && !p.AIEnabled {
		return Translation{}, ErrDisabled
	}
	sub, txt, truncated := mailText(row, p.MaxChars)
	if strings.TrimSpace(sub) == "" && strings.TrimSpace(txt) == "" {
		return Translation{}, ErrInvalid
	}
	provider, cacheOwner := p.Provider, 0
	var ai aiRecord
	if engine == "ai" {
		ai, err = checkAI(ctx, p, uid)
		if err != nil {
			return Translation{}, err
		}
		provider, cacheOwner = "openai_compatible", uid
	}
	fingerprint := cacheKey(sub, txt, provider, p.URL, p.DeepLTier, p.Key)
	if engine == "ai" {
		fingerprint = cacheKey(sub, txt, ai)
	}
	result := Translation{TargetLang: lang, Provider: provider, Engine: engine, Truncated: truncated}
	key := models.EmailTranslation{EmailID: row.Id, UserID: cacheOwner, TargetLang: lang, Engine: engine}
	if !in.Force {
		var cached models.EmailTranslation
		found, err := db.Instance.Context(ctx).Where("email_id=? AND user_id=? AND target_lang=? AND engine=? AND cache_key=?", key.EmailID, key.UserID, lang, engine, fingerprint).Get(&cached)
		if err != nil {
			return Translation{}, err
		}
		if found {
			result.Subject, result.Text, result.SourceLang, result.Cached = cached.Subject, cached.Text, cached.SourceLang, true
			return result, nil
		}
	}
	if engine == "web" {
		if err := s.reserve(ctx, uid, p.DailyLimit); err != nil {
			return Translation{}, err
		}
		result.Subject, result.Text, result.SourceLang, err = s.translateWeb(ctx, p, sub, txt, lang)
	} else {
		release, e := s.acquire(uid)
		if e != nil {
			return Translation{}, e
		}
		defer release()
		err = s.askAI(ctx, uid, p, ai, translatePrompt, sub, txt, lang, func(v string) bool {
			var a struct {
				Subject *string `json:"subject"`
				Text    *string `json:"text"`
			}
			if !strictJSON(v, &a) || a.Subject == nil || a.Text == nil || (*a.Subject == "" && *a.Text == "") {
				return false
			}
			result.Subject, result.Text = *a.Subject, *a.Text
			return true
		})
	}
	if err != nil {
		return Translation{}, err
	}
	if len(result.Subject) > 8192 || len(result.Text)+len(result.Subject) > responseLimit {
		return Translation{}, ErrFormat
	}
	key.Subject, key.Text, key.SourceLang, key.Provider, key.CacheKey = result.Subject, result.Text, result.SourceLang, provider, fingerprint
	err = storeCache(ctx, uid, row.Id, engine == "ai", ai, func(tx *xorm.Session) error {
		_, err := tx.Where("email_id=? AND user_id=? AND target_lang=? AND engine=?", key.EmailID, key.UserID, key.TargetLang, key.Engine).Delete(&models.EmailTranslation{})
		if err != nil {
			return err
		}
		_, err = tx.Insert(&key)
		return err
	})
	return result, err
}

func checkAI(ctx context.Context, p policyRecord, uid int) (aiRecord, error) {
	if !p.AIUserConfigEnabled {
		return aiRecord{}, ErrAIDisabled
	}
	a, err := getAI(ctx, uid)
	if err != nil {
		return a, err
	}
	if a.BaseURL == "" || a.Model == "" {
		return a, ErrAIConfig
	}
	return a, nil
}
func configuredAI(ctx context.Context, uid int) (policyRecord, aiRecord, error) {
	p, e := policy(ctx)
	if e != nil {
		return p, aiRecord{}, e
	}
	a, e := checkAI(ctx, p, uid)
	return p, a, e
}

type AIRequest struct {
	ID     int    `json:"id"`
	Action string `json:"action"`
	Lang   string `json:"lang"`
	Force  bool   `json:"force"`
}

func (s *Service) Analyze(ctx context.Context, uid int, ui string, in AIRequest) (AIResult, error) {
	row, err := ownedMail(ctx, uid, in.ID)
	if err != nil {
		return AIResult{}, err
	}
	if in.Action != "summary" && in.Action != "extract" {
		return AIResult{}, ErrInvalid
	}
	p, ai, err := configuredAI(ctx, uid)
	if err != nil {
		return AIResult{}, err
	}
	pref, err := GetPreferences(ctx, uid)
	if err != nil {
		return AIResult{}, err
	}
	lang, err := targetLanguage(in.Lang, pref.TargetLang, ui)
	if err != nil {
		return AIResult{}, err
	}
	sub, txt, truncated := mailText(row, p.MaxChars)
	if strings.TrimSpace(sub) == "" && strings.TrimSpace(txt) == "" {
		return AIResult{}, ErrInvalid
	}
	fingerprint := cacheKey(sub, txt, ai)
	stored := models.EmailAIResult{UserID: uid, EmailID: row.Id, Action: in.Action, Lang: lang}
	result := AIResult{Action: in.Action, Lang: lang, Model: ai.Model, Truncated: truncated}
	if !in.Force {
		var cached models.EmailAIResult
		found, err := db.Instance.Context(ctx).Where("user_id=? AND email_id=? AND action=? AND lang=? AND cache_key=?", uid, row.Id, in.Action, lang, fingerprint).Get(&cached)
		if err != nil {
			return AIResult{}, err
		}
		if found && json.Unmarshal([]byte(cached.ResultJSON), &result) == nil {
			result.Cached = true
			result.Truncated = truncated
			return result, nil
		}
	}
	release, err := s.acquire(uid)
	if err != nil {
		return AIResult{}, err
	}
	defer release()
	prompt := summaryPrompt
	if in.Action == "extract" {
		prompt = extractPrompt
	}
	err = s.askAI(ctx, uid, p, ai, prompt, sub, txt, lang, func(v string) bool {
		if in.Action == "summary" {
			var a Summary
			if !strictJSON(v, &a) || strings.TrimSpace(a.Summary) == "" || a.KeyPoints == nil {
				return false
			}
			a.Summary = clipped(a.Summary, 2500)
			if n := summarySentences(a.Summary); n < 3 || n > 5 {
				return false
			}
			a.KeyPoints = limited(a.KeyPoints, 6)
			for i := range a.KeyPoints {
				a.KeyPoints[i] = clipped(a.KeyPoints[i], 300)
			}
			result.Summary = &a
			return true
		}
		var a Extraction
		if !strictJSON(v, &a) || a.ActionItems == nil || a.Dates == nil || a.Amounts == nil || a.Links == nil || a.Codes == nil || a.Contacts == nil {
			return false
		}
		cleanExtraction(&a, sub+"\n"+txt)
		result.Extraction = &a
		return true
	})
	if err != nil {
		return AIResult{}, err
	}
	stored.ResultJSON, stored.Model, stored.CacheKey = marshalResult(result), ai.Model, fingerprint
	err = storeCache(ctx, uid, row.Id, true, ai, func(tx *xorm.Session) error {
		_, err := tx.Where("user_id=? AND email_id=? AND action=? AND lang=?", uid, row.Id, in.Action, lang).Delete(&models.EmailAIResult{})
		if err != nil {
			return err
		}
		_, err = tx.Insert(&stored)
		return err
	})
	return result, err
}

func storeCache(ctx context.Context, owner, id int, ai bool, snapshot aiRecord, write func(*xorm.Session) error) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		// Same lock order as permanent unlink: owner first, then the mail row.
		if err := lockUser(tx, owner); err != nil {
			return err
		}
		if ai {
			var cfg models.AIConfig
			ok, err := tx.ID(owner).Get(&cfg)
			if err != nil {
				return err
			}
			var current aiRecord
			if !ok || json.Unmarshal([]byte(cfg.Settings), &current) != nil || current != snapshot {
				return ErrAIConfig
			}
		}
		q := tx.ID(id).Cols("id")
		if tx.Engine().DriverName() != "sqlite" {
			q.ForUpdate()
		}
		ok, err := q.Get(&models.Email{})
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotOwned
		}
		ok, err = tx.Where("email_id=? AND user_id=?", id, owner).Exist(&models.UserEmail{})
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotOwned
		}
		return write(tx)
	})
}

// One row per user bounds storage. The owner lock serializes reservations
// across listeners and processes. Cache misses charge before sending text.
func (s *Service) reserve(ctx context.Context, uid, limit int) error {
	if limit == 0 {
		return nil
	}
	day := s.now().UTC().Unix() / 86400
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if err := lockUser(tx, uid); err != nil {
			return err
		}
		row := models.TranslationUsage{UserID: uid}
		ok, err := tx.ID(uid).Get(&row)
		if err != nil {
			return err
		}
		if !ok {
			row.Day, row.Requests = day, 1
			_, err = tx.Insert(&row)
			return err
		}
		if row.Day != day {
			row.Day, row.Requests = day, 0
		}
		if row.Requests >= limit {
			return ErrLimit
		}
		row.Requests++
		_, err = tx.ID(uid).Cols("day", "requests").Update(&row)
		return err
	})
}

// ErrorCode deliberately never falls back to err.Error() for unknown errors.
func ErrorCode(err error) string {
	sentinels := []error{ErrInvalid, ErrDisabled, ErrNotOwned, ErrLimit, ErrUpstreamRate, ErrConnect, ErrTimeout, ErrAuth, ErrFormat, ErrProvider, ErrAIConfig, ErrAIDisabled, ErrBusy, ErrNetwork}
	if i := slices.IndexFunc(sentinels, func(e error) bool { return errors.Is(err, e) }); i >= 0 {
		return sentinels[i].Error()
	}
	return "internal_error"
}
