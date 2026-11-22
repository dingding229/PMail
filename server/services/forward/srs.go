package forward

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/queue"
)

var srsBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// The nonce makes each queued forward independently addressable. A 128-bit
// HMAC authenticates the purpose, recipient domain and bounded lifetime; the
// resulting local part still fits in 64 octets without embedding addresses.
func BuildSRS(_ string, receivingDomain string) (string, error) {
	host, err := domain.Normalize(receivingDomain)
	if err != nil || config.Instance.SecretKey == "" {
		return "", ErrInvalidSRS
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	issued := strconv.FormatInt(time.Now().UTC().Unix()/86400, 36)
	payload := issued + "=" + strings.ToLower(srsBase32.EncodeToString(nonce[:]))
	return "srs1=" + payload + "=" + srsTag(payload, host) + "@" + host, nil
}

func srsTag(payload, host string) string {
	m := hmac.New(sha256.New, []byte(config.Instance.SecretKey))
	_, _ = m.Write([]byte("pmail/bounce/v1\x00" + host + "\x00" + payload))
	return strings.ToLower(srsBase32.EncodeToString(m.Sum(nil)[:16]))
}

func verifyBounce(local, host string) bool {
	if config.Instance.SecretKey == "" || len(local) > 64 {
		return false
	}
	parts := strings.Split(local, "=")
	if len(parts) != 4 || parts[0] != "srs1" || len(parts[2]) != 20 || len(parts[3]) != 26 {
		return false
	}
	day, err := strconv.ParseInt(parts[1], 36, 32)
	if err != nil {
		return false
	}
	age := time.Now().UTC().Unix()/86400 - day
	if age < 0 || age > 21 {
		return false
	}
	if nonce, err := srsBase32.DecodeString(strings.ToUpper(parts[2])); err != nil || len(nonce) != 12 {
		return false
	}
	return hmac.Equal([]byte(parts[3]), []byte(srsTag(parts[1]+"="+parts[2], host)))
}

func SRSAddress(raw string) bool {
	local, host, ok := strings.Cut(strings.ToLower(strings.TrimSpace(raw)), "@")
	return ok && domain.IsLocal(host) && (strings.HasPrefix(local, "srs1=") || strings.HasPrefix(local, "srs0="))
}

func ResolveSRS(raw string) (models.User, bool, error) {
	local, host, _ := strings.Cut(strings.TrimSpace(raw), "@")
	if !SRSAddress(raw) {
		return models.User{}, false, nil
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	lookup := local + "@" + host
	if strings.HasPrefix(strings.ToLower(local), "srs1=") {
		local = strings.ToLower(local)
		if !verifyBounce(local, host) {
			return models.User{}, true, ErrInvalidSRS
		}
		lookup = local + "@" + host
	} else if !legacySignature(local) {
		return models.User{}, true, ErrInvalidSRS
	}
	// SQL uses (kind, mail_from) index. Legacy signatures only work when
	// the exact address was actually issued; no forwarding-settings fallback.
	var row models.OutboundQueue
	found, err := db.Instance.Where("kind=? AND mail_from=?", queue.KindForward, lookup).Cols("user_id").Desc("id").Get(&row)
	if err != nil {
		return models.User{}, true, err
	}
	if !found || row.UserID <= 0 {
		return models.User{}, true, address.ErrNotFound
	}
	var user models.User
	found, err = db.Instance.ID(row.UserID).Where("disabled=0").Get(&user)
	if err != nil {
		return models.User{}, true, err
	}
	if !found {
		return models.User{}, true, address.ErrNotFound
	}
	return user, true, nil
}

func legacySignature(local string) bool {
	p := strings.Split(local, "=")
	if len(p) != 5 || !strings.EqualFold(p[0], "srs0") || len(p[1]) != 4 || len(p[2]) != 2 || config.Instance.SecretKey == "" {
		return false
	}
	days := 0
	for _, c := range strings.ToUpper(p[2]) {
		n := strings.IndexRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", c)
		if n < 0 {
			return false
		}
		days = days*32 + n
	}
	if (int(time.Now().UTC().Unix()/86400)%1024-days+1024)%1024 > 21 {
		return false
	}
	h := hmac.New(sha1.New, []byte(config.Instance.SecretKey))
	_, _ = h.Write([]byte(strings.Join(p[2:], "=")))
	return hmac.Equal([]byte(strings.ToUpper(p[1])), []byte(srsBase32.EncodeToString(h.Sum(nil))[:4]))
}

type srsBudget struct {
	sync.Mutex
	start time.Time
	total int
	ips   map[string]int
}

var srsLookups srsBudget

func AllowSRSLookup(addr net.Addr) bool {
	source := "unknown"
	if addr != nil {
		if host, _, err := net.SplitHostPort(addr.String()); err == nil {
			if ip, err := netip.ParseAddr(host); err == nil {
				source = ip.Unmap().WithZone("").String()
			}
		}
	}
	return srsLookups.take(source, time.Now())
}
func (b *srsBudget) take(ip string, now time.Time) bool {
	b.Lock()
	defer b.Unlock()
	if b.start.IsZero() || now.Sub(b.start) >= time.Minute {
		b.start = now
		b.total = 0
		b.ips = make(map[string]int)
	}
	if b.total >= 1200 || b.ips[ip] >= 60 {
		return false
	}
	b.total++
	b.ips[ip]++
	return true
}
