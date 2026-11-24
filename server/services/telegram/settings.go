package telegram

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/netproxy"
	"github.com/Jinnrry/pmail/utils/isotime"
	secrets "github.com/Jinnrry/pmail/utils/seal"
	"xorm.io/xorm"
)

var configMu sync.Mutex
var bindMu sync.Mutex

type AdminStatus struct {
	Configured  bool           `json:"configured"`
	Username    *string        `json:"bot_username"`
	ProxySet    bool           `json:"proxy_set"`
	Proxy       *netproxy.View `json:"proxy"`
	ProxySource string         `json:"proxy_source"`
	Mode        string         `json:"webhook_mode"`
}
type TestResult struct {
	OK       bool   `json:"ok"`
	Username string `json:"bot_username"`
	Latency  int64  `json:"latency_ms"`
}
type QuietHours struct {
	Start string `json:"start"`
	End   string `json:"end"`
	TZ    string `json:"tz"`
}
type Filters struct {
	Groups           []int       `json:"group_ids"`
	OnlyVerification bool        `json:"only_verification"`
	IncludeSpam      bool        `json:"include_spam"`
	IncludePreview   bool        `json:"include_preview"`
	Quiet            *QuietHours `json:"quiet_hours"`
}
type UserStatus struct {
	Available bool       `json:"available"`
	Bound     bool       `json:"bound"`
	Title     *string    `json:"chat_title"`
	BoundAt   *time.Time `json:"bound_at"`
	Filters   Filters    `json:"filters"`
}
type Code struct {
	Code   string    `json:"code"`
	Expiry time.Time `json:"expires_at"`
	Link   string    `json:"deep_link"`
}

func seal(s, field string) (string, error) {
	return secrets.Encode("pmail/telegram/v1", field, s)
}
func unseal(s, field string) (string, error) {
	return secrets.Decode("pmail/telegram/v1", field, s)
}

// MigrateProxy never changes bot generation or polling offset.
func MigrateProxy(ctx context.Context) error {
	configMu.Lock()
	defer configMu.Unlock()
	c, err := getConfig(ctx)
	if err != nil || c.Proxy == "" || c.ProxyFields != "" {
		return err
	}
	p, err := netproxy.TelegramDB(c)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = db.Instance.Context(ctx).ID(1).Where("proxy=? AND (proxy_fields IS NULL OR proxy_fields='')", c.Proxy).Cols("proxy", "proxy_fields").Update(&models.TelegramConfig{ProxyFields: string(raw)})
	return err
}
func randomCode(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func getConfig(ctx context.Context) (models.TelegramConfig, error) {
	var c models.TelegramConfig
	_, err := db.Instance.Context(ctx).ID(1).Get(&c)
	return c, err
}
func status(c models.TelegramConfig) AdminStatus {
	_, proxy, source, _ := netproxy.TelegramProxy(c)
	out := AdminStatus{Configured: c.Token != "", ProxySet: source != "none", Proxy: proxy, ProxySource: source, Mode: "polling"}
	if c.Username != "" {
		out.Username = &c.Username
	}
	return out
}
func AdminGet(ctx context.Context) (AdminStatus, error) {
	c, err := getConfig(ctx)
	return status(c), err
}
func AdminPut(ctx context.Context, token *string, proxy *netproxy.Input, clearProxy bool) (AdminStatus, error) {
	configMu.Lock()
	defer configMu.Unlock()
	c, err := getConfig(ctx)
	if err != nil {
		return AdminStatus{}, err
	}
	oldToken, err := unseal(c.Token, "token")
	if err != nil && token == nil {
		return AdminStatus{}, err
	}
	savedProxy, err := netproxy.TelegramDB(c)
	if err != nil && !clearProxy && (proxy == nil || proxy.Password == nil) {
		return AdminStatus{}, ErrProxy
	}
	if token != nil {
		oldToken = strings.TrimSpace(*token)
	}
	savedProxy, err = netproxy.Apply(savedProxy, proxy, clearProxy, netproxy.TelegramField)
	if err != nil {
		return AdminStatus{}, ErrProxy
	}
	raw, err := json.Marshal(savedProxy)
	if err != nil {
		return AdminStatus{}, ErrProxy
	}
	c.ProxyFields, c.Proxy = string(raw), ""
	oldGeneration := c.Generation
	if token != nil {
		// A deliberate replacement/clear invalidates old bindings and queued mail.
		c.Generation, err = randomCode(16)
		if err != nil {
			return AdminStatus{}, err
		}
		c.Offset = 0
	}
	if oldToken != "" {
		p, _, _, proxyErr := netproxy.TelegramProxy(c)
		if proxyErr != nil {
			return AdminStatus{}, ErrProxy
		}
		api, err := newClient(oldToken, p)
		if err != nil {
			return AdminStatus{}, ErrToken
		}
		check, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		c.Username, err = api.me(check)
		if err != nil {
			return AdminStatus{}, ErrToken
		}
	} else {
		c.Username = ""
	}
	if c.Token, err = seal(oldToken, "token"); err != nil {
		return AdminStatus{}, err
	}
	c.ID = 1
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		ok, err := tx.ID(1).Exist(&models.TelegramConfig{})
		if err != nil {
			return err
		}
		if ok {
			cols := []string{"token", "proxy", "proxy_fields", "username", "generation"}
			if c.Generation != oldGeneration {
				cols = append(cols, "next_offset")
			}
			_, err = tx.ID(1).Cols(cols...).Update(&c)
		} else {
			_, err = tx.Insert(&c)
		}
		if err != nil {
			return err
		}
		if oldGeneration != c.Generation {
			if _, err = tx.Where("bot_generation=?", oldGeneration).Delete(&models.TelegramBinding{}); err != nil {
				return err
			}
			if _, err = tx.Where("bot_generation=?", oldGeneration).Delete(&models.TelegramCode{}); err != nil {
				return err
			}
		}
		return nil
	})
	return status(c), err
}
func clientFor(c models.TelegramConfig) (*WorkerClient, error) {
	token, err := unseal(c.Token, "token")
	if err != nil {
		return nil, err
	}
	proxy, _, _, err := netproxy.TelegramProxy(c)
	if err != nil {
		return nil, err
	}
	return newClient(token, proxy)
}
func AdminTest(ctx context.Context) (TestResult, error) {
	c, err := getConfig(ctx)
	if err != nil {
		return TestResult{}, err
	}
	api, err := clientFor(c)
	if err != nil {
		return TestResult{}, err
	}
	t := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	name, err := api.me(ctx)
	if err != nil {
		return TestResult{}, err
	}
	return TestResult{true, name, time.Since(t).Milliseconds()}, nil
}
func filters(ctx context.Context, uid int) (Filters, error) {
	var row models.TelegramPreference
	ok, err := db.Instance.Context(ctx).ID(uid).Get(&row)
	var f Filters
	if err == nil && ok {
		err = json.Unmarshal([]byte(row.Filters), &f)
	}
	return f, err
}
func binding(ctx context.Context, c models.TelegramConfig, uid int) (models.TelegramBinding, bool, error) {
	var b models.TelegramBinding
	ok, err := db.Instance.Context(ctx).ID(uid).Where("bot_generation=?", c.Generation).Get(&b)
	return b, ok && c.Token != "", err
}
func UserGet(ctx context.Context, uid int) (UserStatus, error) {
	c, err := getConfig(ctx)
	if err != nil {
		return UserStatus{}, err
	}
	f, err := filters(ctx, uid)
	if err != nil {
		return UserStatus{}, err
	}
	b, ok, err := binding(ctx, c, uid)
	out := UserStatus{Available: c.Token != "", Bound: ok, Filters: f}
	if ok {
		out.Title = &b.Title
		out.BoundAt = isotime.UTC(b.BoundAt)
	}
	return out, err
}
func lockUser(tx *xorm.Session, uid int) error {
	q := tx.ID(uid).Where("disabled=0").Cols("id")
	if db.Instance.DriverName() != config.DBTypeSQLite {
		q.ForUpdate()
	}
	ok, err := q.Get(&models.User{})
	if !ok && err == nil {
		return ErrInvalid
	}
	return err
}
func SetFilters(ctx context.Context, uid int, f Filters) (UserStatus, error) {
	if len(f.Groups) > 100 {
		return UserStatus{}, ErrInvalid
	}
	if f.Groups != nil {
		f.Groups = slices.Clone(f.Groups)
		slices.Sort(f.Groups)
		f.Groups = slices.Compact(f.Groups)
	}
	if q := f.Quiet; q != nil {
		if _, err := time.Parse("15:04", q.Start); err != nil || len(q.Start) != 5 {
			return UserStatus{}, ErrInvalid
		}
		if _, err := time.Parse("15:04", q.End); err != nil || len(q.End) != 5 || q.Start == q.End {
			return UserStatus{}, ErrInvalid
		}
		if _, err := time.LoadLocation(q.TZ); err != nil || q.TZ == "" || q.TZ == "Local" || len(q.TZ) > 80 {
			return UserStatus{}, ErrInvalid
		}
	}
	data, _ := json.Marshal(f)
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if err := lockUser(tx, uid); err != nil {
			return err
		}
		for _, gid := range f.Groups {
			if _, ok := models.GroupCodeToName[gid]; ok {
				continue
			}
			if gid == 0 {
				continue
			}
			ok, err := tx.ID(gid).Where("user_id=?", uid).Exist(&models.Group{})
			if err != nil {
				return err
			}
			if !ok {
				return ErrInvalid
			}
		}
		ok, err := tx.ID(uid).Exist(&models.TelegramPreference{})
		if err != nil {
			return err
		}
		row := models.TelegramPreference{UserID: uid, Filters: string(data)}
		if ok {
			_, err = tx.ID(uid).Cols("filters").Update(&row)
		} else {
			_, err = tx.Insert(&row)
		}
		return err
	})
	if err != nil {
		return UserStatus{}, err
	}
	return UserGet(ctx, uid)
}
func NewCode(ctx context.Context, uid int) (Code, error) {
	c, err := getConfig(ctx)
	if err != nil {
		return Code{}, err
	}
	if c.Token == "" || c.Username == "" {
		return Code{}, ErrNotConfigured
	}
	value, err := randomCode(24)
	if err != nil {
		return Code{}, err
	}
	exp := time.Now().UTC().Add(10 * time.Minute)
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if err := lockUser(tx, uid); err != nil {
			return err
		}
		if _, err := tx.ID(uid).Delete(&models.TelegramCode{}); err != nil {
			return err
		}
		_, err := tx.Insert(&models.TelegramCode{UserID: uid, Hash: digest(value), Expires: exp.Unix(), Bot: c.Generation})
		return err
	})
	if err != nil {
		return Code{}, err
	}
	return Code{value, exp, "https://t.me/" + c.Username + "?start=" + value}, nil
}
func unbind(ctx context.Context, uid int, byChat int64, identity string) error {
	bindMu.Lock()
	defer bindMu.Unlock()
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		var b models.TelegramBinding
		var ok bool
		var err error
		switch {
		case byChat != 0:
			ok, err = tx.Where("chat_id=?", byChat).Get(&b)
		case uid != 0:
			ok, err = tx.ID(uid).Get(&b)
		default:
			return ErrInvalid
		}
		if err != nil {
			return err
		}
		if !ok {
			if uid != 0 && identity == "" {
				_, err = tx.ID(uid).Delete(&models.TelegramCode{})
			}
			return err
		}
		if identity != "" && identity != b.Identity {
			return nil
		}
		if _, err = tx.ID(b.UserID).Where("identity=?", b.Identity).Delete(&models.TelegramBinding{}); err != nil {
			return err
		}
		if _, err = tx.ID(b.UserID).Delete(&models.TelegramCode{}); err != nil {
			return err
		}
		_, err = tx.Where("user_id=? AND binding=?", b.UserID, b.Identity).Delete(&models.TelegramPush{})
		return err
	})
}
func Unbind(ctx context.Context, uid int) error { return unbind(ctx, uid, 0, "") }
func consume(ctx context.Context, value string, ch chat, generation string, now time.Time) error {
	if ch.Type != "private" || ch.ID <= 0 || len(value) != 32 {
		return ErrBindExpired
	}
	id, err := randomCode(16)
	if err != nil {
		return err
	}
	bindMu.Lock()
	defer bindMu.Unlock()
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		var c models.TelegramConfig
		ok, err := tx.ID(1).Where("generation=?", generation).Get(&c)
		if err != nil {
			return err
		}
		if !ok || c.Token == "" {
			return ErrBindExpired
		}
		var pending models.TelegramCode
		ok, err = tx.Where("code_hash=? AND bot_generation=? AND expires>?", digest(value), generation, now.Unix()).Get(&pending)
		if err != nil {
			return err
		}
		if !ok {
			return ErrBindExpired
		}
		if err = lockUser(tx, pending.UserID); err != nil {
			return err
		}
		// Never steal an existing private chat from another account.
		busy, err := tx.Where("chat_id=? AND user_id<>?", ch.ID, pending.UserID).Exist(&models.TelegramBinding{})
		if err != nil {
			return err
		}
		if busy {
			return ErrBindExpired
		}
		n, err := tx.ID(pending.UserID).Where("code_hash=? AND expires>?", digest(value), now.Unix()).Delete(&models.TelegramCode{})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrBindExpired
		}
		_, err = tx.ID(pending.UserID).Delete(&models.TelegramBinding{})
		if err != nil {
			return err
		}
		title := strings.TrimSpace(ch.First + " " + ch.Last)
		if title == "" {
			title = ch.Username
		}
		title = truncate(title, 100)
		_, err = tx.Insert(&models.TelegramBinding{UserID: pending.UserID, ChatID: ch.ID, Title: title, Identity: id, Bot: generation, BoundAt: now.UTC()})
		if db.IsUniqueViolation(err) {
			return ErrBindExpired
		}
		return err
	})
}
