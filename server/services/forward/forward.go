package forward

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"

	"net/mail"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/del_email"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/services/quota"
	pcontext "github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

var (
	ErrTooManyTargets = errors.New("at most 10 forwarding targets are allowed")
	ErrInvalidTarget  = errors.New("invalid forwarding target")
	ErrSelfTarget     = errors.New("loop forwarding to self: forwarding target cannot be one of your own addresses")
	ErrInvalidSRS     = errors.New("invalid SRS address")
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
		return nil, ErrTooManyTargets
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
		if row.Status == models.AddressActive {
			out = append(out, row.Address)
		}
	}
	return out, nil
}

func EnqueueForUser(ctx *pcontext.Context, userID int, email *parsemail.Email, raw []byte, target string) error {
	if email == nil || email.From == nil {
		return errors.New("forward source email is missing")
	}
	if len(raw) == 0 {
		var err error
		raw, err = email.BuildBytes(ctx, false)
		if err != nil {
			return err
		}
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
			if email.Status < 0 || email.Status > math.MaxInt8 {
				return errors.New("invalid forwarded mail status")
			}
			return quota.EnsureLink(user.ID, int(forwarded.MessageId), int8(email.Status), target)
		}
		return nil
	}
	fromDomain := domain.DefaultName()
	if primary, primaryErr := address.Primary(userID); primaryErr == nil && primary.Domain != "" {
		fromDomain = primary.Domain
	}
	mailFrom, err := BuildSRS(email.From.EmailAddress, fromDomain)
	if err != nil {
		return err
	}
	decorated, err := parsemail.SignBytes(forwardedRaw, "mailer-daemon@"+fromDomain)
	if err != nil {
		return err
	}
	_, err = queue.Enqueue(queue.KindForward, userID, int(email.MessageId), mailFrom, target, decorated)
	return err
}

func Apply(ctx *pcontext.Context, userID int, email *parsemail.Email, raw []byte) {
	setting, targets, err := Get(userID)
	if err != nil || !setting.Enabled || len(targets) == 0 {
		return
	}
	if len(raw) == 0 && email != nil {
		raw, err = email.BuildBytes(ctx, false)
		if err != nil {
			log.WithContext(ctx).Warnf("cannot rebuild forward source: %v", err)
			return
		}
	}
	if parsemail.ShouldSkipForward(email, raw) {
		log.WithContext(ctx).Infof("skip forwarding automatic or daemon message user=%d", userID)
		return
	}
	var relation models.UserEmail
	found, err := db.Instance.Where("email_id=? AND user_id=?", email.MessageId, userID).Get(&relation)
	if err != nil || !found || relation.Status == consts.EmailStatusDel {
		return
	}
	allOK := true
	for _, target := range targets {
		if err := EnqueueForUser(ctx, userID, email, raw, target); err != nil {
			allOK = false
		}
	}
	if allOK && !setting.KeepCopy {
		if err := del_email.DelEmail(&pcontext.Context{Context: ctx.Context, UserID: userID}, []int{int(email.MessageId)}, false); err != nil {
			log.Errorf("mark forwarded copy deleted email=%d user=%d failed: %v", email.MessageId, userID, err)
		}
	}
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
