package ssl

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/testschema"
)

func TestCheckSSLCrtInfo(t *testing.T) {
	oldRoot := config.ROOT_PATH
	config.ROOT_PATH = t.TempDir()
	t.Cleanup(func() { config.ROOT_PATH = oldRoot })
	cert, key, err := testschema.WriteSelfSignedCertificate(filepath.Join(config.ROOT_PATH, "config", "ssl"),
		[]string{"mail.example.test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.WriteConfig(&config.Config{
		SSLType: config.SSLTypeUser, SSLPublicKeyPath: cert, SSLPrivateKeyPath: key, WebDomain: "mail.example.test",
	}); err != nil {
		t.Fatal(err)
	}
	for _, env := range []string{"PMAIL_DOMAIN", "PMAIL_DOMAINS", "PMAIL_WEB_DOMAIN", "PMAIL_SSL_TYPE"} {
		t.Setenv(env, "")
	}
	days, expiry, mismatch, err := CheckSSLCrtInfo()
	if err != nil {
		t.Fatal(err)
	}
	if days < 0 || !expiry.After(time.Now()) || mismatch {
		t.Fatalf("certificate info = days=%d, expiry=%v, mismatch=%v; want a valid matching certificate", days, expiry, mismatch)
	}
}
