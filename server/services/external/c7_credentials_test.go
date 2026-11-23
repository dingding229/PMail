package external

import (
	"errors"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/models"
)

func TestC7ExternalEndpointBindingAndLegacyV1(t *testing.T) {
	old := config.Instance
	config.Instance = &config.Config{SecretKey: "external-test-secret"}
	t.Cleanup(func() { config.Instance = old })
	original := models.ExternalAccount{ID: 100, Host: "mail.example.test", Port: 993, Username: "fixture", Protocol: "imap", TLSMode: "tls"}
	var err error
	original.PasswordEnc, err = encryptAccount(original, "only-fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*models.ExternalAccount){func(a *models.ExternalAccount) { a.ID++ }, func(a *models.ExternalAccount) { a.Host = "another.example.test" }, func(a *models.ExternalAccount) { a.Port++ }, func(a *models.ExternalAccount) { a.Username = "changed" }, func(a *models.ExternalAccount) { a.TLSMode = "none" }} {
		candidate := original
		mutate(&candidate)
		if _, _, err := decryptAccount(candidate); err == nil {
			t.Fatal("rebound ciphertext decrypted")
		}
	}
	original.PasswordEnc, err = sealPassword(legacyPasswordVersion, "only-fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	if password, legacy, err := decryptAccount(original); password != "only-fixture" || !legacy || err != nil {
		t.Fatalf("legacy decrypt status: legacy=%t err=%v", legacy, err)
	}
}

func TestC7ExternalEndpointEditRequiresOwnerAndPassword(t *testing.T) {
	// Insert a legacy row to verify the guard doesn't depend on reading secrets.
	engine := newExternalTestEngine(t)
	a := models.ExternalAccount{UserID: 7, Name: "fixture", Host: "mail.example.test", Port: 993, Username: "fixture", Protocol: "imap", TLSMode: "tls", PasswordEnc: "stored-fixture"}
	if _, err := engine.Insert(&a); err != nil {
		t.Fatal(err)
	}
	a.Host = "another.example.test"
	if _, err := Update(7, false, a, ""); !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("missing password: %v", err)
	}
	if _, err := Update(1, true, a, "different"); !errors.Is(err, ErrEndpointOwner) {
		t.Fatalf("cross-owner endpoint change: %v", err)
	}
	if _, err := TestConnection(a, "fixture\ninvalid"); !errors.Is(err, ErrControlCharacters) {
		t.Fatalf("control characters: %v", err)
	}
	if popSequence("1\r") || popSequence("0") || !popSequence("1") {
		t.Fatal("unsafe POP3 message number was accepted")
	}
}

func TestExternalTLSVerificationRequiresAccountOptIn(t *testing.T) {
	a := models.ExternalAccount{Host: "mail.example.test"}
	if got := externalTLSConfig(a); got.InsecureSkipVerify || got.ServerName != a.Host {
		t.Fatal("default TLS verification disabled")
	}
	a.InsecureSkipVerify = true
	if got := externalTLSConfig(a); !got.InsecureSkipVerify {
		t.Fatal("explicit account opt-in ignored")
	}
}
