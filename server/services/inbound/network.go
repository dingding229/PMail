package inbound

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/dto/parsemail"
	log "github.com/sirupsen/logrus"
)

const reputationTTL = 30 * time.Minute
const reputationCacheSize = 4096

var networkSlots = make(chan struct{}, 16)

type netResult struct {
	listed bool
	code   string
	until  time.Time
}
type lookupIP func(context.Context, string, string) ([]netip.Addr, error)
type networkState struct {
	sync.Mutex
	cached   map[string]netResult
	warnings map[string]netResult
}

var health = networkState{cached: map[string]netResult{}, warnings: map[string]netResult{}}

func acquireNetwork() bool {
	select {
	case networkSlots <- struct{}{}:
		return true
	default:
		return false
	}
}
func releaseNetwork() { <-networkSlots }
func (n *networkState) warn(which, code string) {
	n.Lock()
	now := time.Now()
	old := n.warnings[which]
	if old.code == code && now.Before(old.until) {
		n.Unlock()
		return
	}
	if len(n.warnings) >= 128 {
		clear(n.warnings)
	}
	n.warnings[which] = netResult{code: code, until: now.Add(reputationTTL)}
	n.Unlock()
	log.Warnf("inbound scanner degraded provider=%q code=%q", which, code)
}
func Warnings(s Settings) []Warning {
	out := []Warning{}
	health.Lock()
	defer health.Unlock()
	add := func(key, zone string) {
		w := health.warnings[key]
		if time.Now().Before(w.until) {
			out = append(out, Warning{Code: w.code, Zone: zone})
		}
	}
	if s.DNSBLEnabled {
		for _, z := range s.Zones {
			add(z, z)
		}
	}
	if s.ClamdEnabled {
		add("clamd", "")
	}
	return out
}

var fromBracket = regexp.MustCompile(`(?i)\[((?:IPv6:)?[0-9a-f:.]+)\]`)
var relayBY = regexp.MustCompile(`(?i)\sby\s`)

// Only a configured, authenticated peer may tell us the previous hop. Use the
// single topmost Received field it added; never search untrusted trace history.
func SourceIP(peer netip.Addr, raw []byte) netip.Addr {
	peer = peer.Unmap()
	nets, _ := config.CIDRs(os.Getenv("PMAIL_TRUSTED_RELAYS"))
	trusted := false
	for _, p := range nets {
		if p.Contains(peer) {
			trusted = true
			break
		}
	}
	if !trusted {
		return peer
	}
	received := parsemail.HeaderValue(clippedBytes(raw, 256<<10), "Received")
	at := relayBY.FindStringIndex(received)
	if len(at) == 0 {
		return netip.Addr{}
	}
	from := strings.TrimSpace(received[:at[0]])
	if !strings.HasPrefix(strings.ToLower(from), "from ") {
		return netip.Addr{}
	}
	ips := fromBracket.FindAllStringSubmatch(from, 2)
	if len(ips) != 1 {
		return netip.Addr{}
	}
	candidate := ips[0][1]
	if strings.HasPrefix(strings.ToLower(candidate), "ipv6:") {
		candidate = candidate[5:]
	}
	out, err := netip.ParseAddr(candidate)
	if err != nil {
		return netip.Addr{}
	}
	return out.Unmap()
}
func dnsKey(ip netip.Addr, zone string) string {
	ip = ip.Unmap()
	if ip.Is4() {
		b := ip.As4()
		return fmt.Sprintf("%d.%d.%d.%d.%s.", b[3], b[2], b[1], b[0], zone)
	}
	b := ip.As16()
	encoded := hex.EncodeToString(b[:])
	var reversed strings.Builder
	for i := len(encoded) - 1; i >= 0; i-- {
		reversed.WriteByte(encoded[i])
		reversed.WriteByte('.')
	}
	reversed.WriteString(zone)
	reversed.WriteByte('.')
	return reversed.String()
}
func cacheableIP(ip netip.Addr) bool {
	return ip.IsValid() && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !netip.MustParsePrefix("100.64.0.0/10").Contains(ip.Unmap())
}
func (n *networkState) dns(ctx context.Context, ip netip.Addr, zone string, lookup lookupIP) netResult {
	key := dnsKey(ip, zone)
	now := time.Now()
	n.Lock()
	entry, ok := n.cached[key]
	n.Unlock()
	if ok && now.Before(entry.until) {
		return entry
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addrs, err := lookup(ctx, "ip4", key)
	hit, warning := false, ""
	if err != nil {
		var dn *net.DNSError
		if !(errors.As(err, &dn) && dn.IsNotFound) {
			warning = "dnsbl_unavailable"
		}
	}
	for _, answer := range addrs {
		answer = answer.Unmap()
		if !answer.Is4() {
			continue
		}
		b := answer.As4()
		if b[0] == 127 && b[1] == 255 && b[2] == 255 {
			warning = "dnsbl_resolver_refused"
			break
		}
		if b[0] == 127 {
			hit = true
		}
	}
	if warning != "" {
		n.warn(zone, warning)
		hit = false
	}
	ttl := reputationTTL
	if warning == "dnsbl_unavailable" {
		ttl = time.Minute
	}
	result := netResult{listed: hit, code: warning, until: now.Add(ttl)}
	n.Lock()
	if len(n.cached) >= reputationCacheSize {
		for k, v := range n.cached {
			if now.After(v.until) {
				delete(n.cached, k)
			}
		}
		if len(n.cached) >= reputationCacheSize {
			for k := range n.cached {
				delete(n.cached, k)
				break
			}
		}
	}
	n.cached[key] = result
	n.Unlock()
	return result
}

// Scan only the explicitly configured clamd service. Dial the vetted address
// directly after resolution so an unsafe resolver answer cannot be swapped in.
func dialClamd(ctx context.Context, addr string) (net.Conn, error) {
	if !safeClamd(addr) {
		return nil, ErrInvalid
	}
	d := net.Dialer{}
	if path, ok := strings.CutPrefix(addr, "unix:"); ok {
		return d.DialContext(ctx, "unix", path)
	}
	host, port, _ := net.SplitHostPort(addr)
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if !allowedScannerIP(ip) {
			continue
		}
		if conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.Unmap().String(), port)); err == nil {
			return conn, nil
		}
	}
	return nil, errors.New("clamd endpoint unavailable")
}

const maxClamdPart = 1 << 20
const maxClamdTotal = 4 << 20
const maxClamdTargets = 16

var clamdName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func scanClamd(ctx context.Context, addr string, data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxClamdPart {
		return "", nil
	}
	conn, err := dialClamd(ctx, addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if _, err = io.WriteString(conn, "zINSTREAM\x00"); err != nil {
		return "", err
	}
	var size [4]byte
	for start := 0; start < len(data); start += 32768 {
		part := data[start:min(start+32768, len(data))]
		binary.BigEndian.PutUint32(size[:], uint32(len(part)))
		if _, err = conn.Write(size[:]); err != nil {
			return "", err
		}
		if _, err = conn.Write(part); err != nil {
			return "", err
		}
	}
	clear(size[:])
	if _, err = conn.Write(size[:]); err != nil {
		return "", err
	}
	line, err := bufio.NewReaderSize(conn, 1024).ReadSlice(0)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(strings.TrimSuffix(string(line), "\x00"))
	if text == "stream: OK" {
		return "", nil
	}
	if name, ok := strings.CutPrefix(text, "stream: "); ok && strings.HasSuffix(name, " FOUND") {
		name = clamdName.ReplaceAllString(strings.TrimSuffix(name, " FOUND"), "_")
		name = strings.Trim(name, "_.-")
		if name != "" {
			return "clamav_" + name[:min(80, len(name))], nil
		}
	}
	return "", errors.New("unexpected clamd response")
}

type malwareHit struct {
	index int
	code  string
}

func scanMailClamd(ctx context.Context, addr string, e *parsemail.Email) []malwareHit {
	var out []malwareHit
	total, num := 0, 0
	scan := func(b []byte, index int) bool {
		if len(b) == 0 || len(b) > maxClamdPart || total+len(b) > maxClamdTotal || num >= maxClamdTargets {
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		total += len(b)
		num++
		code, err := scanClamd(ctx, addr, b)
		if err != nil {
			health.warn("clamd", "clamd_unavailable")
			return false
		}
		if code != "" {
			out = append(out, malwareHit{index, code})
		}
		return true
	}
	if !scan(e.Text, -1) {
		return out
	}
	if !e.TextFromHTML && !scan(e.HTML, -1) {
		return out
	}
	for i, a := range e.Attachments {
		if a != nil && !scan(a.Content, i) {
			break
		}
	}
	return out
}
