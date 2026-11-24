package arc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// KeyResolver looks up DKIM key records (selector._domainkey.domain TXT).
type KeyResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// verifyChain validates existing sets before PMail adds the next one: every
// ARC-Seal, the cv= progression, and the most recent ARC-Message-Signature.
func (s Signer) verifyChain(ctx context.Context, m message, sets []arcSet) error {
	resolver := s.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for i, set := range sets {
		t := tags(set.as.value)
		want := "pass"
		if i == 0 {
			want = "none"
		}
		if t["cv"] != want {
			return fmt.Errorf("instance %d cv=%s", set.instance, t["cv"])
		}
		var data bytes.Buffer
		for j := 0; j <= i; j++ {
			data.WriteString(relaxedHeader(sets[j].aar))
			data.WriteString(relaxedHeader(sets[j].ams))
			if j < i {
				data.WriteString(relaxedHeader(sets[j].as))
			}
		}
		data.WriteString(strings.TrimSuffix(relaxedHeader(withoutSignature(set.as)), "\r\n"))
		if err := verifySig(ctx, resolver, t, data.Bytes()); err != nil {
			return fmt.Errorf("instance %d seal: %w", set.instance, err)
		}
	}
	latest := sets[len(sets)-1]
	t := tags(latest.ams.value)
	headerCanon, bodyCanon, _ := strings.Cut(t["c"], "/")
	if headerCanon == "" {
		headerCanon = "simple"
	}
	if bodyCanon == "" {
		bodyCanon = "simple"
	}
	body := m.body
	if bodyCanon == "relaxed" {
		body = relaxedBody(body)
	} else {
		body = simpleBody(body)
	}
	sum := sha256.Sum256(body)
	if base64.StdEncoding.EncodeToString(sum[:]) != t["bh"] {
		return errors.New("latest message signature: body hash mismatch")
	}
	canon := relaxedHeader
	if headerCanon == "simple" {
		canon = simpleHeader
	}
	var data bytes.Buffer
	used := map[string]int{}
	for _, name := range strings.Split(t["h"], ":") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || name == "arc-seal" {
			continue
		}
		if f, ok := m.lastUnused(name, used); ok {
			data.WriteString(canon(f))
		}
	}
	data.WriteString(strings.TrimSuffix(canon(withoutSignature(latest.ams)), "\r\n"))
	if err := verifySig(ctx, resolver, t, data.Bytes()); err != nil {
		return fmt.Errorf("latest message signature: %w", err)
	}
	return nil
}

func simpleHeader(f field) string { return f.name + ":" + f.value + "\r\n" }

func simpleBody(body []byte) []byte {
	for bytes.HasSuffix(body, []byte("\r\n\r\n")) {
		body = body[:len(body)-2]
	}
	if len(body) == 0 || !bytes.HasSuffix(body, []byte("\r\n")) {
		body = append(append([]byte(nil), body...), '\r', '\n')
	}
	return body
}

func verifySig(ctx context.Context, r KeyResolver, t map[string]string, data []byte) error {
	if !validAlgo(t["a"]) || t["d"] == "" || t["s"] == "" || t["b"] == "" {
		return errors.New("missing signature tags")
	}
	sig, err := base64.StdEncoding.DecodeString(t["b"])
	if err != nil {
		return errors.New("bad signature encoding")
	}
	pub, err := lookupKey(ctx, r, t["s"]+"._domainkey."+t["d"])
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	switch k := pub.(type) {
	case *rsa.PublicKey:
		if t["a"] != "rsa-sha256" {
			return errors.New("key and algorithm differ")
		}
		return rsa.VerifyPKCS1v15(k, crypto.SHA256, sum[:], sig)
	}
	return errors.New("unsupported key")
}

func lookupKey(ctx context.Context, r KeyResolver, name string) (crypto.PublicKey, error) {
	records, err := r.LookupTXT(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("key lookup: %w", err)
	}
	for _, rec := range records {
		t := tags(rec)
		if v, ok := t["v"]; ok && v != "DKIM1" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(t["p"])
		if err != nil || len(raw) == 0 {
			continue
		}
		if k := t["k"]; k != "" && k != "rsa" {
			continue
		}
		if pub, err := x509.ParsePKIXPublicKey(raw); err == nil {
			if k, ok := pub.(*rsa.PublicKey); ok {
				return k, nil
			}
		}
		if k, err := x509.ParsePKCS1PublicKey(raw); err == nil {
			return k, nil
		}
	}
	return nil, errors.New("no usable key record")
}

// Verify validates every ARC set in raw, as a receiver would.
func Verify(ctx context.Context, raw []byte, r KeyResolver) error {
	m := parse(raw)
	sets, err := m.arcSets()
	if err != nil {
		return err
	}
	if len(sets) == 0 {
		return errors.New("no ARC sets")
	}
	return Signer{Resolver: r}.verifyChain(ctx, m, sets)
}
