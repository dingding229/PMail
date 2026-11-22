package inbound

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/mail"
	"net/netip"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

var (
	ErrInvalid      = errors.New("invalid inbound parameters")
	ErrNotFound     = errors.New("inbound resource not found")
	ErrRuleConflict = errors.New("spam rule already exists")
	ErrRuleLimit    = errors.New("spam rule limit reached")
)

const RuleLimit = 500

type Settings struct {
	DNSBLEnabled         bool      `json:"dnsbl_enabled"`
	Zones                []string  `json:"dnsbl_zones"`
	DefaultSpamThreshold float64   `json:"default_spam_threshold"`
	ClamdEnabled         bool      `json:"clamd_enabled"`
	ClamdAddr            string    `json:"clamd_addr"`
	RiskAudit            bool      `json:"risk_block_dangerous_attachments"`
	Warnings             []Warning `json:"warnings,omitempty"`
}
type Warning struct {
	Code    string `json:"code"`
	Zone    string `json:"zone,omitempty"`
	Message string `json:"message,omitempty"`
}

var settingsWrite sync.Mutex // dedicated singleton; readers always observe committed DB settings
var hostLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func hostName(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for label := range strings.SplitSeq(s, ".") {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return true
}
func normalizeDomain(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}
func addressHost(s string) string {
	_, host, ok := strings.Cut(s, "@")
	if !ok {
		return ""
	}
	return normalizeDomain(host)
}
func validThreshold(n float64) bool { return !math.IsNaN(n) && n >= 1 && n <= 20 }
func safeString(s string, maxLen int) bool {
	if !utf8.ValidString(s) || len(s) == 0 || utf8.RuneCountInString(s) > maxLen {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) || unicode.In(c, unicode.Cf) {
			return false
		}
	}
	return true
}
func safeClamd(addr string) bool {
	if len(addr) > 255 || !safeString(addr, 255) {
		return false
	}
	if path, ok := strings.CutPrefix(addr, "unix:"); ok {
		return filepath.IsAbs(path) && filepath.Clean(path) == path && path != "/"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return allowedScannerIP(ip)
	}
	return hostName(normalizeDomain(host))
}
func allowedScannerIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.Zone() == "" && (ip.IsLoopback() || ip.IsPrivate() || ip.IsGlobalUnicast()) && !ip.IsLinkLocalUnicast()
}
func normalizeSettings(s Settings) (Settings, error) {
	if !validThreshold(s.DefaultSpamThreshold) || len(s.Zones) > 8 || (s.DNSBLEnabled && len(s.Zones) == 0) {
		return s, ErrInvalid
	}
	zones := make([]string, 0, len(s.Zones))
	seen := map[string]bool{}
	for _, raw := range s.Zones {
		z := normalizeDomain(raw)
		if len(z) > 175 || !hostName(z) || !strings.Contains(z, ".") {
			return s, ErrInvalid
		}
		if _, err := netip.ParseAddr(z); err == nil {
			return s, ErrInvalid
		}
		if !seen[z] {
			zones = append(zones, z)
			seen[z] = true
		}
	}
	s.Zones = zones
	s.ClamdAddr = strings.TrimSpace(s.ClamdAddr)
	s.Warnings = nil
	if (s.ClamdAddr != "" && !safeClamd(s.ClamdAddr)) || (s.ClamdEnabled && s.ClamdAddr == "") {
		return s, ErrInvalid
	}
	return s, nil
}
func GetSettings(ctx context.Context) (Settings, error) { return getSettings(ctx, db.Instance) }

func getSettings(ctx context.Context, engine *xorm.Engine) (Settings, error) {
	s := Settings{DefaultSpamThreshold: 6, Zones: []string{}}
	if engine == nil {
		return s, nil
	}
	var row models.InboundSettings
	found, err := engine.Context(ctx).ID(1).Get(&row)
	if err != nil || !found {
		return s, err
	}
	if err = json.Unmarshal([]byte(row.ZonesJSON), &s.Zones); err != nil {
		return s, err
	}
	s.DNSBLEnabled = row.DNSBLEnabled
	s.ClamdEnabled = row.ClamdEnabled
	s.ClamdAddr = row.ClamdAddr
	s.DefaultSpamThreshold = row.DefaultSpamThreshold
	s.RiskAudit = row.RiskAudit
	return s, nil
}
func PutSettings(ctx context.Context, s Settings) (Settings, error) {
	s, err := normalizeSettings(s)
	if err != nil {
		return s, err
	}
	zones, _ := json.Marshal(s.Zones)
	settingsWrite.Lock()
	defer settingsWrite.Unlock()
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		vals := []any{1, s.DNSBLEnabled, string(zones), s.DefaultSpamThreshold, s.ClamdEnabled, s.ClamdAddr, s.RiskAudit}
		_, err := tx.Exec(append([]any{settingsUpsert(db.Instance.DriverName())}, vals...)...)
		return err
	})
	return s, err
}
func settingsUpsert(driver string) string {
	const base = "INSERT INTO inbound_settings (id,dnsbl_enabled,dnsbl_zones,default_spam_threshold,clamd_enabled,clamd_addr,risk_block_dangerous_attachments) VALUES (?,?,?,?,?,?,?)"
	names := []string{"dnsbl_enabled", "dnsbl_zones", "default_spam_threshold", "clamd_enabled", "clamd_addr", "risk_block_dangerous_attachments"}
	var updates []string
	for _, c := range names {
		if driver == config.DBTypeMySQL {
			updates = append(updates, c+"=VALUES("+c+")")
		} else {
			updates = append(updates, c+"=excluded."+c)
		}
	}
	if driver == config.DBTypeMySQL {
		return base + " ON DUPLICATE KEY UPDATE " + strings.Join(updates, ",")
	}
	return base + " ON CONFLICT (id) DO UPDATE SET " + strings.Join(updates, ",")
}

func Preference(ctx context.Context, userID int, threshold float64) (models.SpamPreference, error) {
	return preference(ctx, db.Instance, userID, threshold)
}

func preference(ctx context.Context, engine *xorm.Engine, userID int, threshold float64) (models.SpamPreference, error) {
	p := models.SpamPreference{UserID: userID, Enabled: true, Threshold: threshold, BayesEnabled: true}
	var stored models.SpamPreference
	found, err := engine.Context(ctx).ID(userID).Get(&stored)
	if err == nil && found {
		return stored, nil
	}
	return p, err
}
func lockOwner(tx *xorm.Session, id int) error {
	q := tx.ID(id).Cols("id")
	if db.Instance.DriverName() != config.DBTypeSQLite {
		q.ForUpdate()
	}
	found, err := q.Get(&models.User{})
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}
func PutPreference(ctx context.Context, userID int, p models.SpamPreference) (models.SpamPreference, error) {
	p.UserID = userID
	if !validThreshold(p.Threshold) {
		return p, ErrInvalid
	}
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if err := lockOwner(tx, userID); err != nil {
			return err
		}
		found, err := tx.ID(userID).Exist(&models.SpamPreference{})
		if err != nil {
			return err
		}
		if !found {
			_, err = tx.Insert(&p)
		} else {
			_, err = tx.ID(userID).Cols("enabled", "threshold", "bayes_enabled").Update(&p)
		}
		return err
	})
	return p, err
}
func Rules(ctx context.Context, userID int) ([]models.SpamRule, error) {
	return rules(ctx, db.Instance, userID)
}

func rules(ctx context.Context, engine *xorm.Engine, userID int) ([]models.SpamRule, error) {
	out := []models.SpamRule{}
	if err := engine.Context(ctx).Where("user_id=?", userID).Asc("id").Limit(RuleLimit).Find(&out); err != nil {
		return nil, err
	}
	return out, nil
}
func hashValue(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func normalizeRule(r models.SpamRule) (models.SpamRule, error) {
	r.ID = 0
	r.Value = strings.TrimSpace(r.Value)
	if (r.Action != "block" && r.Action != "allow") || !safeString(r.Value, 254) {
		return r, ErrInvalid
	}
	switch r.Kind {
	case "address":
		r.Value = strings.ToLower(r.Value)
		a, err := mail.ParseAddress(r.Value)
		if err != nil || a.Address != r.Value || !hostName(addressHost(r.Value)) || strings.HasSuffix(r.Value, ".") {
			return r, ErrInvalid
		}
	case "domain":
		r.Value = normalizeDomain(r.Value)
		if !hostName(r.Value) {
			return r, ErrInvalid
		}
	case "keyword":
		r.Value = strings.Join(strings.Fields(strings.ToLower(r.Value)), " ")
	default:
		return r, ErrInvalid
	}
	if len(r.Value) > 1016 {
		return r, ErrInvalid
	}
	r.ValueHash = hashValue(r.Value)
	return r, nil
}
func AddRule(ctx context.Context, userID int, r models.SpamRule) (models.SpamRule, error) {
	r, err := normalizeRule(r)
	if err != nil {
		return r, err
	}
	r.UserID = userID
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if err := lockOwner(tx, userID); err != nil {
			return err
		}
		exists, err := tx.Where("user_id=? AND action=? AND kind=? AND value_hash=?", userID, r.Action, r.Kind, r.ValueHash).Exist(&models.SpamRule{})
		if err != nil {
			return err
		}
		if exists {
			return ErrRuleConflict
		}
		n, err := tx.Where("user_id=?", userID).Count(&models.SpamRule{})
		if err != nil {
			return err
		}
		if n >= RuleLimit {
			return ErrRuleLimit
		}
		r.ID = 0
		_, err = tx.Insert(&r)
		if db.IsUniqueViolation(err) {
			return ErrRuleConflict
		}
		return err
	})
	return r, err
}
func DeleteRule(ctx context.Context, userID, id int) error {
	if id <= 0 {
		return ErrInvalid
	}
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		if err := lockOwner(tx, userID); err != nil {
			return err
		}
		n, err := tx.ID(id).Where("user_id=?", userID).Delete(&models.SpamRule{})
		if err == nil && n == 0 {
			return ErrNotFound
		}
		return err
	})
}
