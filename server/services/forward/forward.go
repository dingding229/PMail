package forward

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/queue"
	pcontext "github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

var (
	ErrInvalidTarget = errors.New("invalid forwarding target")
	ErrSelfTarget    = errors.New("loop forwarding to self: forwarding target cannot be one of your own addresses")
	ErrInvalidSRS    = errors.New("invalid SRS address")
)

func Get(userID int) (models.ForwardSetting, []string, error) {
	var row models.ForwardSetting
	found, err := db.Instance.ID(userID).Get(&row)
	if err != nil {
		return row, nil, err
	}
	if !found {
		return models.ForwardSetting{UserID: userID, KeepCopy: true}, nil, nil
	}
	return row, Targets(row.Targets), nil
}

func Set(userID int, enabled bool, targets []string, keepCopy bool) (models.ForwardSetting, error) {
	clean, err := ValidateTargets(userID, targets)
	if err != nil {
		return models.ForwardSetting{}, err
	}
	data, _ := json.Marshal(clean)
	row := models.ForwardSetting{UserID: userID, Enabled: enabled, Targets: string(data), KeepCopy: keepCopy, Updated: time.Now()}
	var user models.User
	found, err := db.Instance.ID(userID).Get(&user)
	if err != nil {
		return row, err
	}
	if !found {
		return row, address.ErrNotFound
	}
	updated, err := db.Instance.ID(userID).Cols("enabled", "targets", "keep_copy", "updated").Update(&row)
	if err != nil {
		return row, err
	}
	if updated == 0 {
		_, err = db.Instance.Insert(&row)
	}
	return row, err
}

func Targets(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var targets []string
	if strings.HasPrefix(raw, "[") {
		_ = json.Unmarshal([]byte(raw), &targets)
	} else {
		for _, item := range strings.Split(raw, ",") {
			if item = strings.TrimSpace(item); item != "" {
				targets = append(targets, item)
			}
		}
	}
	return unique(targets)
}

func ValidateTargets(userID int, targets []string) ([]string, error) {
	if len(targets) > 10 {
		return nil, fmt.Errorf("at most 10 forwarding targets are allowed")
	}
	self := map[string]bool{}
	addresses, err := ownAddresses(userID)
	if err != nil {
		return nil, err
	}
	for _, own := range addresses {
		self[strings.ToLower(own)] = true
	}
	clean := make([]string, 0, len(targets))
	for _, raw := range targets {
		raw = strings.TrimSpace(raw)
		addr, err := mail.ParseAddress(raw)
		if err != nil || addr.Address != raw {
			return nil, ErrInvalidTarget
		}
		local, host, err := address.Split(raw)
		if err != nil || local == "" || host == "" {
			return nil, ErrInvalidTarget
		}
		normalized := local + "@" + host
		if self[strings.ToLower(normalized)] {
			return nil, ErrSelfTarget
		}
		clean = append(clean, normalized)
	}
	return unique(clean), nil
}

func ownAddresses(userID int) ([]string, error) {
	if db.Instance == nil {
		return nil, errors.New("database is not initialized")
	}
	rows, err := address.List(userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Address)
	}
	return out, nil
}

func EnqueueForUser(ctx *pcontext.Context, userID int, email *parsemail.Email, raw []byte, target string) error {
	if email == nil || email.From == nil {
		return errors.New("forward source email is missing")
	}
	if len(raw) == 0 {
		raw = email.BuildBytes(ctx, false)
	}
	if parsemail.ShouldSkipForward(email, raw) {
		log.WithContext(ctx).Infof("skip forwarding automatic or daemon message to %s", target)
		return nil
	}
	node := nodeID()
	if shouldStop(email, raw, node, target) {
		return nil
	}
	local, host, err := address.Split(target)
	if err != nil || local == "" || host == "" {
		return ErrInvalidTarget
	}
	normalizedTarget := strings.ToLower(local + "@" + host)
	addresses, err := ownAddresses(userID)
	if err != nil {
		return err
	}
	for _, own := range addresses {
		if strings.EqualFold(own, normalizedTarget) {
			return ErrSelfTarget
		}
	}
	forwardedRaw := addForwardHeaders(raw, node, normalizedTarget)
	if domain.IsLocal(host) {
		forwarded := parsemail.NewEmailFromReader([]string{target}, bytes.NewReader(forwardedRaw), len(forwardedRaw))
		forwarded.Size = len(forwardedRaw)
		forwarded.RawData = append([]byte(nil), forwardedRaw...)
		if err := queue.DeliverLocal(ctx, forwarded, target); err != nil {
			if !errors.Is(err, queue.ErrLocalUnavailable) {
				return err
			}
			// Production always installs the unified handler. This narrow fallback
			// keeps offline rule fixtures usable without starting listeners.
			if forwarded.MessageId <= 0 {
				forwarded.MessageId = email.MessageId
			}
			user, found, resolveErr := address.Resolve(target)
			if resolveErr != nil || !found {
				return address.ErrNotFound
			}
			var relation models.UserEmail
			found, resolveErr = db.Instance.Where("email_id=? AND user_id=?", forwarded.MessageId, user.ID).Get(&relation)
			if resolveErr != nil || found {
				return resolveErr
			}
			_, resolveErr = db.Instance.Insert(&models.UserEmail{EmailID: int(forwarded.MessageId), UserID: user.ID, Status: int8(email.Status)})
			return resolveErr
		}
		return nil
	}
	fromDomain := domain.DefaultName()
	if primary, primaryErr := address.Primary(userID); primaryErr == nil && primary.Domain != "" {
		fromDomain = primary.Domain
	}
	mailFrom := BuildSRS(email.From.EmailAddress, fromDomain)
	decorated := parsemail.SignBytes(forwardedRaw, "mailer-daemon@"+fromDomain)
	_, err = queue.Enqueue(queue.KindForward, userID, int(email.MessageId), mailFrom, target, decorated)
	return err
}

func Apply(ctx *pcontext.Context, userID int, email *parsemail.Email, raw []byte) {
	setting, targets, err := Get(userID)
	if err != nil || !setting.Enabled || len(targets) == 0 {
		return
	}
	if len(raw) == 0 && email != nil {
		raw = email.BuildBytes(ctx, false)
	}
	if parsemail.ShouldSkipForward(email, raw) {
		log.WithContext(ctx).Infof("skip forwarding automatic or daemon message user=%d", userID)
		return
	}
	var relation models.UserEmail
	found, err := db.Instance.Where("email_id=? AND user_id=?", email.MessageId, userID).Get(&relation)
	if err != nil || !found || relation.Status == 3 {
		return
	}
	allOK := true
	for _, target := range targets {
		if err := EnqueueForUser(ctx, userID, email, raw, target); err != nil {
			allOK = false
		}
	}
	if allOK && !setting.KeepCopy {
		if _, err := db.Instance.Table(&models.UserEmail{}).Where("email_id=? AND user_id=?", email.MessageId, userID).Cols("status").Update(map[string]interface{}{"status": consts.EmailStatusDel}); err != nil {
			log.Errorf("mark forwarded copy deleted email=%d user=%d failed: %v", email.MessageId, userID, err)
		}
	}
}

func BuildSRS(original, forwardDomain string) string {
	local, host, err := address.Split(original)
	if err != nil {
		return original
	}
	tt := base32Days(time.Now().UTC())
	payload := tt + "=" + host + "=" + local
	h := hmac.New(sha1.New, []byte(config.Instance.SecretKey))
	_, _ = h.Write([]byte(payload))
	hash := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(h.Sum(nil))[:4]
	return "SRS0=" + hash + "=" + payload + "@" + forwardDomain
}

func ResolveSRS(raw string) (models.User, bool, error) {
	local, host, ok := strings.Cut(strings.TrimSpace(raw), "@")
	if !ok || !domain.IsLocal(host) || !strings.HasPrefix(strings.ToUpper(local), "SRS0=") {
		return models.User{}, false, nil
	}
	original, err := parseSRS(local, host)
	if err != nil {
		return models.User{}, true, err
	}
	var row models.OutboundQueue
	found, err := db.Instance.Where("kind=? AND mail_from=?", queue.KindForward, local+"@"+host).Desc("id").Get(&row)
	if err != nil {
		return models.User{}, true, err
	}
	if found && row.UserID > 0 {
		var user models.User
		ok, err := db.Instance.ID(row.UserID).Get(&user)
		return user, true, errOrNotFound(ok, err)
	}
	// A retained forwarding setting is a fallback for rows already cleaned up.
	var settings []models.ForwardSetting
	if err := db.Instance.Where("enabled=?", true).Find(&settings); err != nil {
		return models.User{}, true, err
	}
	for _, setting := range settings {
		for _, target := range Targets(setting.Targets) {
			if strings.EqualFold(target, original) {
				var user models.User
				ok, err := db.Instance.ID(setting.UserID).Get(&user)
				return user, true, errOrNotFound(ok, err)
			}
		}
	}
	return models.User{}, true, address.ErrNotFound
}

func errOrNotFound(ok bool, err error) error {
	if err != nil {
		return err
	}
	if !ok {
		return address.ErrNotFound
	}
	return nil
}

func parseSRS(local, forwardDomain string) (string, error) {
	parts := strings.Split(local, "=")
	if len(parts) != 5 || !strings.EqualFold(parts[0], "SRS0") || len(parts[1]) != 4 || len(parts[2]) != 2 {
		return "", ErrInvalidSRS
	}
	var day int
	for _, c := range strings.ToUpper(parts[2]) {
		value := strings.IndexRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", c)
		if value < 0 {
			return "", ErrInvalidSRS
		}
		day = day*32 + value
	}
	current := int(time.Now().UTC().Unix() / 86400)
	modulus := 32 * 32
	age := (current%modulus - day + modulus) % modulus
	if age > 21 {
		return "", ErrInvalidSRS
	}
	payload := parts[2] + "=" + parts[3] + "=" + parts[4]
	h := hmac.New(sha1.New, []byte(config.Instance.SecretKey))
	_, _ = h.Write([]byte(payload))
	expected := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(h.Sum(nil))[:4]
	if !hmac.Equal([]byte(strings.ToUpper(parts[1])), []byte(strings.ToUpper(expected))) {
		return "", ErrInvalidSRS
	}
	return parts[4] + "@" + parts[3], nil
}

func base32Days(t time.Time) string {
	value := int(t.Unix() / 86400)
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	return string([]byte{alphabet[(value/32)%32], alphabet[value%32]})
}

func nodeID() string {
	h := hmac.New(sha256.New, []byte(config.Instance.SecretKey))
	_, _ = h.Write([]byte("pmail-node"))
	return hex.EncodeToString(h.Sum(nil))[:8]
}

func headerRegion(raw []byte) []byte {
	if separator := bytes.Index(raw, []byte("\r\n\r\n")); separator >= 0 {
		return raw[:separator]
	}
	if separator := bytes.Index(raw, []byte("\n\n")); separator >= 0 {
		return raw[:separator]
	}
	return raw
}

func shouldStop(email *parsemail.Email, raw []byte, node, target string) bool {
	if email == nil {
		return true
	}
	headers := headerRegion(raw)
	received := 0
	nodeRecords := 0
	normalizedTarget := strings.ToLower(strings.TrimSpace(target))
	for _, line := range strings.Split(strings.ReplaceAll(string(headers), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "received:"):
			received++
		case strings.HasPrefix(lower, "x-pmail-forwarded-for:"):
			value := strings.TrimSpace(line[len("x-pmail-forwarded-for:"):])
			parts := strings.SplitN(value, ";", 2)
			if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), node) {
				continue
			}
			nodeRecords++
			if strings.EqualFold(strings.TrimSpace(parts[1]), normalizedTarget) {
				return true
			}
		}
	}
	if received > 30 || nodeRecords >= 5 {
		return true
	}
	for _, list := range [][]*parsemail.User{email.To, email.Cc, email.Bcc} {
		for _, recipient := range list {
			if recipient != nil && strings.EqualFold(recipient.EmailAddress, target) {
				return true
			}
		}
	}
	return false
}

func addForwardHeaders(raw []byte, node, target string) []byte {
	separator := bytes.Index(raw, []byte("\r\n\r\n"))
	lineEnd := []byte("\r\n")
	if separator < 0 {
		separator = bytes.Index(raw, []byte("\n\n"))
		lineEnd = []byte("\n")
	}
	line := []byte("X-PMail-Forwarded-For: " + node + "; " + target)
	if separator < 0 {
		out := make([]byte, 0, len(raw)+len(line)+len(lineEnd)*2)
		out = append(out, line...)
		out = append(out, lineEnd...)
		out = append(out, lineEnd...)
		out = append(out, raw...)
		return out
	}
	// separator points at the original blank-line delimiter. Insert a complete
	// header line before it, preserving every original header byte unchanged.
	header := make([]byte, 0, len(line)+len(lineEnd))
	header = append(header, lineEnd...)
	header = append(header, line...)
	out := make([]byte, 0, len(raw)+len(header))
	out = append(out, raw[:separator]...)
	out = append(out, header...)
	out = append(out, raw[separator:]...)
	return out
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}
