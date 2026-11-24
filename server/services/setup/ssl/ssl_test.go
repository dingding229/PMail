package ssl

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/testschema"
)

func TestLoadCertificateFromConfig(t *testing.T) {
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
	cfg, err := config.ReadEffectiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCertificate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Leaf == nil || !loaded.Leaf.NotAfter.After(time.Now()) || loaded.Leaf.VerifyHostname("mail.example.test") != nil {
		t.Fatal("expected a valid matching certificate")
	}
}
