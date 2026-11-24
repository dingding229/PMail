package forward

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/arc"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/testschema"
	pcontext "github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/dkimkey"
	"github.com/emersion/go-msgauth/dkim"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

type keyDNS map[string]string

func (k keyDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := k[name]; ok {
		return []string{v}, nil
	}
	return nil, errors.New("not found")
}

func TestExternalForwardIsARCSealedAndDKIMSigned(t *testing.T) {
	oldConfig, oldDB, oldResolver := config.Instance, db.Instance, arcResolver
	t.Cleanup(func() { config.Instance, db.Instance, arcResolver = oldConfig, oldDB, oldResolver })
	config.Instance = &config.Config{Domain: "example.com", Domains: []string{"example.com"}, SecretKey: "fixture"}
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	db.Instance = engine
	if err := testschema.Sync(engine); err != nil {
		t.Fatal(err)
	}
	key, err := dkimkey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := dkimkey.Parse(key)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(signer.Public())
	record := "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(der)
	dns := keyDNS{"default._domainkey.example.com": record}
	arcResolver = dns
	if _, err := engine.Insert(&models.Domain{ID: 1, Name: "example.com", IsDefault: true, DkimSelector: "default", DkimPrivateKey: key}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&models.User{ID: 1, Account: "owner", Name: "owner"}); err != nil {
		t.Fatal(err)
	}
	if err := domain.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err := address.Backfill(); err != nil {
		t.Fatal(err)
	}
	raw := []byte("From: Sender <sender@origin.example>\r\nTo: owner@example.com\r\nSubject: hi\r\nMessage-ID: <x@origin.example>\r\n\r\nbody line\r\n")
	email := &parsemail.Email{
		MessageId: 7,
		From:      &parsemail.User{EmailAddress: "sender@origin.example"},
		RawData:   raw,
		AuthResults: &models.MailAuthResults{
			SPF:   models.MailAuthCheck{Result: "pass", Domain: "origin.example"},
			DKIM:  models.MailAuthCheck{Result: "pass", Domain: "origin.example"},
			DMARC: models.MailAuthCheck{Result: "pass", Domain: "origin.example"},
		},
	}
	if err := EnqueueForUser(&pcontext.Context{Context: t.Context()}, 1, email, raw, "someone@external.example"); err != nil {
		t.Fatal(err)
	}
	var payload models.OutboundPayload
	if ok, err := engine.Get(&payload); err != nil || !ok {
		t.Fatalf("no queued payload: %v", err)
	}
	wire := payload.Data
	for _, want := range []string{"ARC-Seal: i=1; a=rsa-sha256;", "cv=none", "ARC-Message-Signature: i=1;", "ARC-Authentication-Results: i=1; example.com; spf=pass smtp.mailfrom=origin.example;", "dmarc=pass header.from=origin.example", "DKIM-Signature:"} {
		if !bytes.Contains(wire, []byte(want)) {
			t.Fatalf("missing %q in:\n%s", want, wire)
		}
	}
	// DKIM-Signature is added after ARC and must still verify.
	verifs, err := dkim.VerifyWithOptions(bytes.NewReader(wire), &dkim.VerifyOptions{LookupTXT: func(name string) ([]string, error) {
		return dns.LookupTXT(t.Context(), name)
	}})
	if err != nil || len(verifs) == 0 || verifs[0].Err != nil {
		t.Fatalf("forwarder DKIM: %v %+v", err, verifs)
	}
	// The ARC chain itself must verify, as the next receiver would check it.
	if err := arc.Verify(t.Context(), wire, dns); err != nil {
		t.Fatalf("ARC chain: %v", err)
	}
	t.Setenv("PMAIL_ARC_DISABLE", "1")
	if out := arcSeal(&pcontext.Context{Context: t.Context()}, raw, email, "example.com"); !bytes.Equal(out, raw) {
		t.Fatal("PMAIL_ARC_DISABLE did not turn sealing off")
	}
}
