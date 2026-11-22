package external

import (
	"errors"
	"os"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func newExternalTestEngine(t *testing.T) *xorm.Engine {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := testschema.Sync(engine); err != nil {
		_ = engine.Close()
		t.Fatal(err)
	}
	old := db.Instance
	db.Instance = engine
	t.Cleanup(func() { db.Instance = old; _ = engine.Close() })
	return engine
}

func TestExternalPasswordEncryptionRoundTripAndTamper(t *testing.T) {
	old := config.Instance
	config.Instance = &config.Config{SecretKey: "external-test-secret"}
	t.Cleanup(func() { config.Instance = old })
	account := models.ExternalAccount{ID: 77, Host: "mail.example.net", Port: 993, Username: "alice"}
	ciphertext, err := encryptAccount(account, "app-password")
	if err != nil {
		t.Fatal(err)
	}
	account.PasswordEnc = ciphertext
	plain, legacy, err := decryptAccount(account)
	if err != nil || legacy || plain != "app-password" {
		t.Fatalf("decrypt = %q, err=%v", plain, err)
	}
	first := len(passwordVersion) + 1
	mutated := ciphertext[:first] + string(ciphertext[first]^1) + ciphertext[first+1:]
	account.PasswordEnc = mutated
	if _, _, err := decryptAccount(account); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}

func TestExternalValidateHostAndDialControl(t *testing.T) {
	old := os.Getenv("PMAIL_FETCH_ALLOW_PRIVATE")
	_ = os.Unsetenv("PMAIL_FETCH_ALLOW_PRIVATE")
	t.Cleanup(func() { _ = os.Setenv("PMAIL_FETCH_ALLOW_PRIVATE", old) })
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "::1", "fc00::1"} {
		if err := validateHost(host); !errors.Is(err, ErrPrivateHost) {
			t.Fatalf("validateHost(%q) = %v", host, err)
		}
	}
	if err := connectionControl()("tcp", "127.0.0.1:993", nil); !errors.Is(err, ErrPrivateHost) {
		t.Fatalf("connection control = %v", err)
	}
}

func TestExternalSeenSupportsPOP3UIDLsAndUIDValidity(t *testing.T) {
	engine := newExternalTestEngine(t)
	if _, err := engine.Insert(&models.ExternalSeen{AccountID: 1, Folder: "INBOX", UIDValidity: 0, UID: 0, UIDL: "a", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&models.ExternalSeen{AccountID: 1, Folder: "INBOX", UIDValidity: 0, UID: 0, UIDL: "b", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Insert(&models.ExternalSeen{AccountID: 1, Folder: "INBOX", UIDValidity: 2, UID: 1, UIDL: "", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	if !seen(1, "INBOX", 0, 0, "a") || !seen(1, "INBOX", 0, 0, "b") || !seen(1, "INBOX", 2, 1, "") {
		t.Fatal("seen keys were not distinct")
	}
}

func TestExternalSeenPendingRollback(t *testing.T) {
	newExternalTestEngine(t)
	already, err := beginSeen(1, "INBOX", 0, 0, "uidl", "<message@example>")
	if err != nil || already {
		t.Fatalf("beginSeen = %v, %v", already, err)
	}
	if err := deleteSeen(1, "INBOX", 0, 0, "uidl"); err != nil {
		t.Fatal(err)
	}
	if seen(1, "INBOX", 0, 0, "uidl") {
		t.Fatal("rolled-back seen row still marked done")
	}
}
