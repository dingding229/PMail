// Package arc adds an Authenticated Received Chain set (RFC 8617) to mail
// that PMail forwards, so the final receiver can see the authentication
// results PMail observed before forwarding changed the envelope.
package arc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxInstance is the largest ARC instance number (RFC 8617 4.2.1).
const MaxInstance = 50

// Headers signed by the ARC-Message-Signature when present.
var signedHeaders = []string{
	"from", "to", "cc", "subject", "date", "message-id", "reply-to",
	"in-reply-to", "references", "mime-version", "content-type",
	"content-transfer-encoding", "list-id", "list-unsubscribe", "dkim-signature",
}

var ErrChainFailed = errors.New("arc: existing chain did not validate")

type Signer struct {
	Domain     string
	Selector   string
	Key        crypto.Signer
	AuthServID string
	Resolver   KeyResolver
	Now        func() time.Time
}

// Seal prepends ARC-Seal, ARC-Message-Signature and ARC-Authentication-Results
// to raw. results is the authentication result list without the authserv-id,
// e.g. "spf=pass smtp.mailfrom=example.com; dkim=pass header.d=example.com".
// A message whose existing chain fails validation is returned unchanged with
// ErrChainFailed; callers should still deliver it.
func (s Signer) Seal(ctx context.Context, raw []byte, results string) ([]byte, error) {
	if s.Key == nil || s.Domain == "" || s.Selector == "" {
		return raw, errors.New("arc: signer not configured")
	}
	algo, err := algorithm(s.Key)
	if err != nil {
		return raw, err
	}
	msg := parse(raw)
	sets, err := msg.arcSets()
	if err != nil {
		return raw, ErrChainFailed
	}
	n := len(sets) + 1
	if n > MaxInstance {
		return raw, ErrChainFailed
	}
	cv := "none"
	if len(sets) > 0 {
		if err := s.verifyChain(ctx, msg, sets); err != nil {
			return raw, errors.Join(ErrChainFailed, err)
		}
		cv = "pass"
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	t := strconv.FormatInt(now().Unix(), 10)
	authserv := s.AuthServID
	if authserv == "" {
		authserv = s.Domain
	}
	if strings.TrimSpace(results) == "" {
		results = "none"
	}
	aar := field{name: "ARC-Authentication-Results", value: fmt.Sprintf(" i=%d; %s; %s", n, authserv, results)}

	// ARC-Message-Signature covers the message as forwarded.
	bh := bodyHash(msg.body)
	var names []string
	var selected []field
	used := map[string]int{}
	for _, name := range signedHeaders {
		for {
			f, ok := msg.lastUnused(name, used)
			if !ok {
				break
			}
			names = append(names, name)
			selected = append(selected, f)
		}
	}
	amsValue := fmt.Sprintf(" i=%d; a=%s; c=relaxed/relaxed; d=%s; s=%s; t=%s;\r\n\th=%s;\r\n\tbh=%s; b=", n, algo, s.Domain, s.Selector, t, strings.Join(names, ":"), bh)
	var data bytes.Buffer
	for _, f := range selected {
		data.WriteString(relaxedHeader(f))
	}
	data.WriteString(strings.TrimSuffix(relaxedHeader(field{name: "ARC-Message-Signature", value: amsValue}), "\r\n"))
	sig, err := sign(s.Key, data.Bytes())
	if err != nil {
		return raw, err
	}
	ams := field{name: "ARC-Message-Signature", value: amsValue + fold(sig)}

	// ARC-Seal covers every ARC set, oldest first, ending with this one.
	asValue := fmt.Sprintf(" i=%d; a=%s; t=%s; cv=%s;\r\n\td=%s; s=%s; b=", n, algo, t, cv, s.Domain, s.Selector)
	data.Reset()
	for _, set := range sets {
		data.WriteString(relaxedHeader(set.aar))
		data.WriteString(relaxedHeader(set.ams))
		data.WriteString(relaxedHeader(set.as))
	}
	data.WriteString(relaxedHeader(aar))
	data.WriteString(relaxedHeader(ams))
	data.WriteString(strings.TrimSuffix(relaxedHeader(field{name: "ARC-Seal", value: asValue}), "\r\n"))
	sig, err = sign(s.Key, data.Bytes())
	if err != nil {
		return raw, err
	}
	as := field{name: "ARC-Seal", value: asValue + fold(sig)}

	eol := "\r\n"
	if !bytes.Contains(raw, []byte("\r\n")) {
		eol = "\n"
	}
	var out bytes.Buffer
	for _, f := range []field{as, ams, aar} {
		out.WriteString(f.name + ":" + strings.ReplaceAll(f.value, "\r\n", eol) + eol)
	}
	out.Write(raw)
	return out.Bytes(), nil
}

// ErrUnsupportedKey is returned for non-RSA keys: RFC 8617 defines only
// rsa-sha256 for ARC (Ed25519 is a DKIM-only algorithm, RFC 8463).
var ErrUnsupportedKey = errors.New("arc: only RSA keys can sign ARC sets")

func algorithm(k crypto.Signer) (string, error) {
	if _, ok := k.Public().(*rsa.PublicKey); ok {
		return "rsa-sha256", nil
	}
	return "", ErrUnsupportedKey
}

func sign(k crypto.Signer, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	sig, err := k.Sign(rand.Reader, sum[:], crypto.SHA256)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// fold splits a base64 value so header lines stay short. Verifiers remove
// whitespace from b= values.
func fold(v string) string {
	var b strings.Builder
	for len(v) > 0 {
		n := min(len(v), 72)
		if b.Len() > 0 {
			b.WriteString("\r\n\t ")
		}
		b.WriteString(v[:n])
		v = v[n:]
	}
	return b.String()
}

// ---- message parsing and canonicalization ----

type field struct{ name, value string } // value keeps folding, without the final line break

type message struct {
	fields []field
	body   []byte
}

func parse(raw []byte) message {
	norm := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	head, body, found := bytes.Cut(norm, []byte("\n\n"))
	if !found {
		head, body = norm, nil
	}
	var m message
	for _, line := range strings.Split(string(head), "\n") {
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && len(m.fields) > 0 {
			m.fields[len(m.fields)-1].value += "\r\n" + line
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		m.fields = append(m.fields, field{name: name, value: value})
	}
	m.body = bytes.ReplaceAll(body, []byte("\n"), []byte("\r\n"))
	return m
}

// lastUnused returns the bottom-most occurrence of name not yet used, the
// order DKIM-style signatures select repeated header fields.
func (m message) lastUnused(name string, used map[string]int) (field, bool) {
	seen := 0
	for i := len(m.fields) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(m.fields[i].name), name) {
			continue
		}
		if seen == used[name] {
			used[name]++
			return m.fields[i], true
		}
		seen++
	}
	return field{}, false
}

var wsp = regexp.MustCompile(`[ \t]+`)

func relaxedHeader(f field) string {
	v := strings.ReplaceAll(f.value, "\r\n", "")
	v = strings.ReplaceAll(v, "\n", "")
	v = strings.TrimSpace(wsp.ReplaceAllString(v, " "))
	return strings.ToLower(strings.TrimSpace(f.name)) + ":" + v + "\r\n"
}

func relaxedBody(body []byte) []byte {
	lines := strings.Split(string(body), "\r\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(wsp.ReplaceAllString(l, " "), " ")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\r\n") + "\r\n")
}

func bodyHash(body []byte) string {
	sum := sha256.Sum256(relaxedBody(body))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// tags parses a tag=value list. Whitespace inside values is removed, which is
// correct for the b=, bh= and h= tags used here.
func tags(v string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(v, ";") {
		k, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		val = strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(val, "\r\n", " "), "\n", " ")), "")
		if _, dup := out[k]; !dup {
			out[k] = val
		}
	}
	return out
}

var bTag = regexp.MustCompile(`(^|;)([ \t\r\n]*b[ \t\r\n]*=)[^;]*`)

// withoutSignature empties the b= tag for signature computation.
func withoutSignature(f field) field {
	return field{name: f.name, value: bTag.ReplaceAllString(f.value, "$1$2")}
}

type arcSet struct {
	instance     int
	aar, ams, as field
}

func instanceOf(f field) (int, bool) {
	v := strings.TrimSpace(f.value)
	k, rest, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(k) != "i" {
		return 0, false
	}
	num, _, _ := strings.Cut(rest, ";")
	i, err := strconv.Atoi(strings.TrimSpace(num))
	return i, err == nil && i >= 1 && i <= MaxInstance
}

// arcSets returns existing sets ordered by instance, or an error when the
// structure is invalid (gaps, duplicates or missing members).
func (m message) arcSets() ([]arcSet, error) {
	byI := map[int]*arcSet{}
	put := func(f field, which string) error {
		i, ok := instanceOf(f)
		if !ok {
			return errors.New("bad instance")
		}
		set := byI[i]
		if set == nil {
			set = &arcSet{instance: i}
			byI[i] = set
		}
		target := map[string]*field{"aar": &set.aar, "ams": &set.ams, "as": &set.as}[which]
		if target.name != "" {
			return errors.New("duplicate ARC header")
		}
		*target = f
		return nil
	}
	for _, f := range m.fields {
		var err error
		switch strings.ToLower(strings.TrimSpace(f.name)) {
		case "arc-authentication-results":
			err = put(f, "aar")
		case "arc-message-signature":
			err = put(f, "ams")
		case "arc-seal":
			err = put(f, "as")
		}
		if err != nil {
			return nil, err
		}
	}
	var sets []arcSet
	for i := 1; i <= len(byI); i++ {
		set := byI[i]
		if set == nil || set.aar.name == "" || set.ams.name == "" || set.as.name == "" {
			return nil, errors.New("incomplete ARC chain")
		}
		sets = append(sets, *set)
	}
	if len(sets) != len(byI) {
		return nil, errors.New("ARC instance gap")
	}
	return sets, nil
}

func validAlgo(a string) bool { return a == "rsa-sha256" }
