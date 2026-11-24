package arc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeDNS map[string]string

func (f fakeDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := f[name]; ok {
		return []string{v}, nil
	}
	return nil, errors.New("no such record")
}

func rsaKey(t *testing.T) (crypto.Signer, string) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	return k, "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(der)
}

func edKey(t *testing.T) (crypto.Signer, string) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, "v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(pub)
}

const sample = "From: Alice <alice@example.org>\r\n" +
	"To: bob@example.net\r\n" +
	"Subject: =?UTF-8?B?5rWL6K+V?= hello\r\n" +
	"Date: Tue, 24 Nov 2026 12:00:00 +0000\r\n" +
	"Message-ID: <m1@example.org>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Hello  world \r\n\r\n\r\n"

func TestSealTwoHopsAndVerify(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  func(*testing.T) (crypto.Signer, string)
		algo string
	}{{"rsa", rsaKey, "rsa-sha256"}} {
		t.Run(tc.name, func(t *testing.T) {
			k1, rec1 := tc.key(t)
			k2, rec2 := tc.key(t)
			dns := fakeDNS{"s1._domainkey.fwd1.example": rec1, "s2._domainkey.fwd2.example": rec2}
			now := func() time.Time { return time.Unix(1795500000, 0) }
			hop1 := Signer{Domain: "fwd1.example", Selector: "s1", Key: k1, AuthServID: "mx.fwd1.example", Resolver: dns, Now: now}
			out1, err := hop1.Seal(t.Context(), []byte(sample), "spf=pass smtp.mailfrom=example.org; dkim=pass header.d=example.org; dmarc=pass header.from=example.org")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasSuffix(out1, []byte(sample)) || !strings.HasPrefix(string(out1), "ARC-Seal: i=1; a="+tc.algo+"; t=1795500000; cv=none;") {
				t.Fatalf("unexpected first set:\n%s", out1)
			}
			// A second forwarder validates the first set and continues the chain.
			hop2 := Signer{Domain: "fwd2.example", Selector: "s2", Key: k2, Resolver: dns, Now: now}
			out2, err := hop2.Seal(t.Context(), append([]byte("Received: from fwd1.example\r\n"), out1...), "arc=pass")
			if err != nil {
				t.Fatalf("second hop: %v", err)
			}
			if !strings.Contains(string(out2), "ARC-Seal: i=2; a="+tc.algo+"; t=1795500000; cv=pass;") {
				t.Fatalf("second set missing:\n%s", out2)
			}
			m := parse(out2)
			sets, err := m.arcSets()
			if err != nil || len(sets) != 2 {
				t.Fatalf("sets: %v %d", err, len(sets))
			}
			if err := hop2.verifyChain(t.Context(), m, sets); err != nil {
				t.Fatalf("final chain does not verify: %v", err)
			}
		})
	}
}

func TestSealRefusesBrokenChain(t *testing.T) {
	k, rec := rsaKey(t)
	dns := fakeDNS{"s._domainkey.fwd.example": rec}
	s := Signer{Domain: "fwd.example", Selector: "s", Key: k, Resolver: dns}
	out, err := s.Seal(t.Context(), []byte(sample), "spf=pass")
	if err != nil {
		t.Fatal(err)
	}
	for name, broken := range map[string][]byte{
		"body changed":    bytes.Replace(out, []byte("Hello  world"), []byte("Hello  World"), 1),
		"subject changed": bytes.Replace(out, []byte("hello\r\n"), []byte("bye\r\n"), 1),
		"seal removed":    out[bytes.Index(out, []byte("ARC-Message-Signature")):],
	} {
		got, err := s.Seal(t.Context(), broken, "spf=pass")
		if !errors.Is(err, ErrChainFailed) || !bytes.Equal(got, broken) {
			t.Errorf("%s: err=%v, message changed=%v", name, err, !bytes.Equal(got, broken))
		}
	}
	// Unknown key: chain cannot be validated, so no new set is added.
	if _, err := (Signer{Domain: "x.example", Selector: "s", Key: k, Resolver: fakeDNS{}}).Seal(t.Context(), out, "spf=pass"); !errors.Is(err, ErrChainFailed) {
		t.Fatalf("unverifiable chain sealed: %v", err)
	}
}

func TestSealKeepsLFMessagesAndFoldedHeaders(t *testing.T) {
	k, rec := rsaKey(t)
	dns := fakeDNS{"s._domainkey.fwd.example": rec}
	lf := strings.ReplaceAll(sample, "\r\n", "\n")
	lf = strings.Replace(lf, "Subject: =?UTF-8?B?5rWL6K+V?= hello\n", "Subject: a long\n\tfolded   subject\n", 1)
	s := Signer{Domain: "fwd.example", Selector: "s", Key: k, Resolver: dns}
	out, err := s.Seal(t.Context(), []byte(lf), "spf=pass")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("\r\n")) {
		t.Fatal("LF message got CRLF headers")
	}
	// Transport converts to CRLF; the chain must still verify.
	crlf := bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	m := parse(crlf)
	sets, _ := m.arcSets()
	if err := s.verifyChain(t.Context(), m, sets); err != nil {
		t.Fatalf("CRLF transport broke chain: %v", err)
	}
}

func TestSealRejectsEd25519Key(t *testing.T) {
	k, _ := edKey(t)
	raw := []byte(sample)
	out, err := (Signer{Domain: "fwd.example", Selector: "s", Key: k}).Seal(t.Context(), raw, "spf=pass")
	if !errors.Is(err, ErrUnsupportedKey) || !bytes.Equal(out, raw) {
		t.Fatalf("ed25519 key: %v", err)
	}
}
