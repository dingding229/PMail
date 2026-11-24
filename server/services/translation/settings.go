package translation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/netproxy"
	"github.com/Jinnrry/pmail/utils/seal"
	"xorm.io/xorm"
)

var (
	ErrInvalid      = errors.New("translation_invalid")
	ErrDisabled     = errors.New("translation_disabled")
	ErrNotOwned     = errors.New("translation_mail_not_found")
	ErrLimit        = errors.New("translation_daily_limit")
	ErrUpstreamRate = errors.New("translation_upstream_rate_limited")
	ErrConnect      = errors.New("translation_connection_failed")
	ErrTimeout      = errors.New("translation_timeout")
	ErrAuth         = errors.New("translation_upstream_auth")
	ErrFormat       = errors.New("translation_response_invalid")
	ErrProvider     = errors.New("translation_provider_failed")
	ErrAIConfig     = errors.New("ai_not_configured")
	ErrAIDisabled   = errors.New("ai_config_disabled")
	ErrBusy         = errors.New("ai_busy")
	ErrNetwork      = errors.New("ai_network_blocked")
)

const namespace = "pmail/translation/v1"
const webProxyField = "translation/web"
const DefaultTimeout = 30

var settingsMu sync.Mutex

type Policy struct {
	WebEnabled            bool   `json:"web_enabled"`
	AIEnabled             bool   `json:"ai_enabled"` // AI translation switch; not the summary switch
	DefaultEngine         string `json:"default_engine"`
	Provider              string `json:"provider"`
	URL                   string `json:"url"`
	DeepLTier             string `json:"deepl_tier"`
	MyMemoryEmail         string `json:"mymemory_email"`
	ProxyMode             string `json:"proxy_mode"` // none, telegram, custom
	MaxChars              int    `json:"max_chars"`
	DailyLimit            int    `json:"daily_limit_per_user"`
	Timeout               int    `json:"timeout"` // seconds
	AIUserConfigEnabled   bool   `json:"ai_user_config_enabled"`
	AIAllowPrivateNetwork bool   `json:"ai_allow_private_network"`
}

type policyRecord struct {
	Policy
	Key   string          `json:"key_ciphertext,omitempty"`
	Proxy netproxy.Stored `json:"proxy"`
}
type PolicyView struct {
	Policy
	APIKeySet bool           `json:"api_key_set"`
	Proxy     *netproxy.View `json:"proxy"`
}
type PolicyPatch struct {
	WebEnabled            *bool           `json:"web_enabled"`
	AIEnabled             *bool           `json:"ai_enabled"`
	DefaultEngine         *string         `json:"default_engine"`
	Provider              *string         `json:"provider"`
	URL                   *string         `json:"url"`
	DeepLTier             *string         `json:"deepl_tier"`
	MyMemoryEmail         *string         `json:"mymemory_email"`
	ProxyMode             *string         `json:"proxy_mode"`
	Proxy                 *netproxy.Input `json:"proxy"`
	ClearProxy            bool            `json:"clear_proxy"`
	APIKey                *string         `json:"api_key"`
	MaxChars              *int            `json:"max_chars"`
	DailyLimit            *int            `json:"daily_limit_per_user"`
	Timeout               *int            `json:"timeout"`
	AIUserConfigEnabled   *bool           `json:"ai_user_config_enabled"`
	AIAllowPrivateNetwork *bool           `json:"ai_allow_private_network"`
}

func defaults() policyRecord {
	return policyRecord{Policy: Policy{Provider: "google_web", DeepLTier: "auto", ProxyMode: "none", DefaultEngine: "web", MaxChars: 8000, DailyLimit: 200, Timeout: DefaultTimeout, AIUserConfigEnabled: true}}
}
func policy(ctx context.Context) (policyRecord, error) {
	p := defaults()
	var row models.TranslationSettings
	found, err := db.Instance.Context(ctx).ID(1).Get(&row)
	if err != nil {
		return p, err
	}
	if found {
		err = json.Unmarshal([]byte(row.Settings), &p)
	}
	return p, err
}
func (p policyRecord) view() PolicyView                 { return PolicyView{p.Policy, p.Key != "", p.Proxy.View()} }
func GetPolicy(ctx context.Context) (PolicyView, error) { p, err := policy(ctx); return p.view(), err }
func assign[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func SetPolicy(ctx context.Context, u PolicyPatch) (PolicyView, error) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	p, err := policy(ctx)
	if err != nil {
		return PolicyView{}, err
	}
	oldProvider, oldURL := p.Provider, p.URL
	assign(&p.WebEnabled, u.WebEnabled)
	assign(&p.AIEnabled, u.AIEnabled)
	assign(&p.DefaultEngine, u.DefaultEngine)
	assign(&p.Provider, u.Provider)
	assign(&p.URL, u.URL)
	assign(&p.DeepLTier, u.DeepLTier)
	if u.MyMemoryEmail != nil {
		value := strings.TrimSpace(*u.MyMemoryEmail)
		if value != "" {
			addr, e := mail.ParseAddress(value)
			if e != nil || addr.Name != "" || addr.Address != value || !netproxy.Safe(value, 254) {
				return PolicyView{}, ErrInvalid
			}
		}
		p.MyMemoryEmail = value
	}
	assign(&p.ProxyMode, u.ProxyMode)
	assign(&p.MaxChars, u.MaxChars)
	assign(&p.DailyLimit, u.DailyLimit)
	assign(&p.Timeout, u.Timeout)
	assign(&p.AIUserConfigEnabled, u.AIUserConfigEnabled)
	assign(&p.AIAllowPrivateNetwork, u.AIAllowPrivateNetwork)
	if p.MaxChars < 500 || p.MaxChars > 50000 || p.DailyLimit < 0 || !validTimeout(p.Timeout) || !engineValid(p.DefaultEngine) {
		return PolicyView{}, ErrInvalid
	}
	switch p.Provider {
	case "google_web", "edge_web", "deepl", "libretranslate", "mymemory":
	default:
		return PolicyView{}, ErrInvalid
	}
	if p.DeepLTier != "auto" && p.DeepLTier != "free" && p.DeepLTier != "pro" {
		return PolicyView{}, ErrInvalid
	}
	if p.URL != "" {
		if _, err := baseURL(p.URL); err != nil {
			return PolicyView{}, err
		}
	} else if p.Provider == "libretranslate" && p.WebEnabled {
		return PolicyView{}, ErrInvalid
	}
	if oldProvider != p.Provider || (p.Provider == "libretranslate" && oldURL != p.URL) {
		p.Key = ""
	}
	if u.APIKey != nil {
		p.Key, err = protect("web/key", *u.APIKey)
		if err != nil {
			return PolicyView{}, err
		}
	}
	if p.Provider == "deepl" && p.WebEnabled && p.Key == "" {
		return PolicyView{}, ErrInvalid
	}
	if u.Proxy != nil && u.ClearProxy {
		return PolicyView{}, ErrInvalid
	}
	p.Proxy, err = netproxy.Apply(p.Proxy, u.Proxy, u.ClearProxy, webProxyField)
	if err != nil {
		return PolicyView{}, ErrInvalid
	}
	switch p.ProxyMode {
	case "none", "telegram":
	case "custom":
		if p.Proxy.Type == "" {
			return PolicyView{}, ErrInvalid
		}
	default:
		return PolicyView{}, ErrInvalid
	}
	if err := writeSettings(ctx, "translation_settings", "id", 1, p); err != nil {
		return PolicyView{}, err
	}
	return p.view(), nil
}

func validTimeout(n int) bool   { return n >= 1 && n <= 120 }
func engineValid(s string) bool { return s == "ai" || s == "web" }
func protect(field, key string) (string, error) {
	if !netproxy.Safe(key, 1024) {
		return "", ErrInvalid
	}
	return seal.Encode(namespace, field, key)
}
func baseURL(raw string) (*url.URL, error) {
	if !netproxy.Safe(raw, 512) || strings.ContainsAny(raw, "?#") {
		return nil, ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Opaque != "" || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, ErrInvalid
	}
	if _, err := netproxy.NormalizeHost(u.Hostname()); err != nil {
		return nil, ErrInvalid
	}
	if u.Port() != "" {
		n, err := strconv.Atoi(u.Port())
		if err != nil || n < 1 || n > 65535 {
			return nil, ErrInvalid
		}
	}
	return u, nil
}

type Preferences struct {
	TargetLang      string `json:"target_lang"`
	AutoTranslate   bool   `json:"auto_translate"`
	TranslateEngine string `json:"translate_engine"`
	AIAutoSummary   bool   `json:"ai_auto_summary"`
}

func GetPreferences(ctx context.Context, uid int) (Preferences, error) {
	var p Preferences
	var row models.TranslationPreference
	ok, err := db.Instance.Context(ctx).ID(uid).Get(&row)
	if err == nil && ok {
		err = json.Unmarshal([]byte(row.Settings), &p)
	}
	return p, err
}
func SetPreferences(ctx context.Context, uid int, p Preferences) (Preferences, error) {
	if p.TargetLang != "" {
		v, err := language(p.TargetLang)
		if err != nil {
			return Preferences{}, err
		}
		p.TargetLang = v
	}
	if p.TranslateEngine != "" && !engineValid(p.TranslateEngine) {
		return Preferences{}, ErrInvalid
	}
	return p, writeSettings(ctx, "translation_preference", "user_id", uid, p)
}

type AIInput struct {
	BaseURL    string          `json:"base_url"`
	Model      string          `json:"model"`
	Timeout    int             `json:"timeout"`
	ProxyMode  string          `json:"proxy_mode"` // none, system, custom
	Proxy      *netproxy.Input `json:"proxy"`
	ClearProxy bool            `json:"clear_proxy"`
	APIKey     *string         `json:"api_key"`
}
type aiRecord struct {
	BaseURL   string          `json:"base_url"`
	Model     string          `json:"model"`
	Timeout   int             `json:"timeout"`
	ProxyMode string          `json:"proxy_mode"`
	Proxy     netproxy.Stored `json:"proxy"`
	Key       string          `json:"key_ciphertext,omitempty"`
}
type AIView struct {
	Configured   bool           `json:"configured"`
	Allowed      bool           `json:"ai_user_config_enabled"`
	AllowPrivate bool           `json:"ai_allow_private_network"`
	BaseURL      string         `json:"base_url"`
	Model        string         `json:"model"`
	Timeout      int            `json:"timeout"`
	ProxyMode    string         `json:"proxy_mode"`
	APIKeySet    bool           `json:"api_key_set"`
	Proxy        *netproxy.View `json:"proxy"`
}

func aiField(uid int) string { return fmt.Sprintf("user/%d", uid) }
func getAI(ctx context.Context, uid int) (aiRecord, error) {
	session := db.Instance.NewSession()
	defer session.Close()
	return readAI(session.Context(ctx), uid)
}

func readAI(session *xorm.Session, uid int) (aiRecord, error) {
	p := aiRecord{Timeout: DefaultTimeout, ProxyMode: "none"}
	var row models.AIConfig
	found, err := session.ID(uid).Get(&row)
	if found && err == nil {
		err = json.Unmarshal([]byte(row.Settings), &p)
	}
	return p, err
}
func (a aiRecord) view(p policyRecord) AIView {
	return AIView{a.BaseURL != "" && a.Model != "", p.AIUserConfigEnabled, p.AIAllowPrivateNetwork, a.BaseURL, a.Model, a.Timeout, a.ProxyMode, a.Key != "", a.Proxy.View()}
}
func GetAI(ctx context.Context, uid int) (AIView, error) {
	p, err := policy(ctx)
	if err != nil {
		return AIView{}, err
	}
	a, err := getAI(ctx, uid)
	return a.view(p), err
}
func SetAI(ctx context.Context, uid int, in AIInput) (AIView, error) {
	p, err := policy(ctx)
	if err != nil {
		return AIView{}, err
	}
	if !p.AIUserConfigEnabled {
		return AIView{}, ErrAIDisabled
	}
	if _, err := baseURL(in.BaseURL); err != nil || strings.TrimSpace(in.Model) == "" || !netproxy.Safe(in.Model, 200) {
		return AIView{}, ErrInvalid
	}
	if in.Timeout == 0 {
		in.Timeout = DefaultTimeout
	}
	if !validTimeout(in.Timeout) || in.Proxy != nil && in.ClearProxy {
		return AIView{}, ErrInvalid
	}
	if in.ProxyMode == "" {
		in.ProxyMode = "none"
	}
	if in.ProxyMode != "none" && in.ProxyMode != "system" && in.ProxyMode != "custom" {
		return AIView{}, ErrInvalid
	}
	var a aiRecord
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if err := lockUser(tx, uid); err != nil {
			return err
		}
		a, err = readAI(tx, uid)
		if err != nil {
			return err
		}
		a.Proxy, err = netproxy.Apply(a.Proxy, in.Proxy, in.ClearProxy, aiField(uid))
		if err != nil || in.ProxyMode == "custom" && a.Proxy.Type == "" {
			return ErrInvalid
		}
		// An omitted key stays on the existing endpoint, not on a newly chosen endpoint.
		if a.BaseURL != in.BaseURL {
			a.Key = ""
		}
		if in.APIKey != nil {
			a.Key, err = protect(aiField(uid)+"/key", *in.APIKey)
			if err != nil {
				return err
			}
		}
		a.BaseURL, a.Model, a.Timeout, a.ProxyMode = in.BaseURL, in.Model, in.Timeout, in.ProxyMode
		return saveSettings(tx, "user_ai_config", "user_id", uid, a)
	})
	return a.view(p), err
}

// DeleteUserAI can be used inside the account deletion transaction.
func DeleteUserAI(tx *xorm.Session, uid int) error {
	for _, row := range []any{&models.AIConfig{}, &models.EmailAIResult{}, &models.EmailTranslation{}} {
		q := tx.Where("user_id=?", uid)
		if _, err := q.Delete(row); err != nil {
			return err
		}
	}
	return nil
}
func RemoveAI(ctx context.Context, uid int) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if err := lockUser(tx, uid); err != nil {
			return err
		}
		return DeleteUserAI(tx, uid)
	})
}

func writeSettings(ctx context.Context, table, col string, id int, value any) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error { tx.Context(ctx); return saveSettings(tx, table, col, id, value) })
}
func saveSettings(tx *xorm.Session, table, col string, id int, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	query := "INSERT INTO " + table + " (" + col + ",settings) VALUES (?,?)"
	if tx.Engine().DriverName() == "mysql" {
		query += " ON DUPLICATE KEY UPDATE settings=VALUES(settings)"
	} else {
		query += " ON CONFLICT (" + col + ") DO UPDATE SET settings=excluded.settings"
	}
	_, err = tx.Exec(query, id, string(raw))
	return err
}
func lockUser(tx *xorm.Session, uid int) error {
	var u models.User
	q := tx.ID(uid).Cols("id", "disabled")
	if tx.Engine().DriverName() != "sqlite" {
		q.ForUpdate()
	}
	found, err := q.Get(&u)
	if err != nil {
		return err
	}
	if !found || u.Disabled != 0 {
		return ErrNotOwned
	}
	return nil
}
