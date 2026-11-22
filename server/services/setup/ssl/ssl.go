package ssl

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Jinnrry/pmail/utils/securefile"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/utils/errors"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	log "github.com/sirupsen/logrus"
)

type MyUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *MyUser) GetEmail() string {
	return u.Email
}
func (u MyUser) GetRegistration() *registration.Resource {
	return u.Registration
}
func (u *MyUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func GetSSL() string {
	cfg, err := config.ReadConfig()
	if err != nil {
		panic(err)
	}
	if cfg.SSLType == "" {
		return config.SSLTypeAutoHTTP
	}

	return cfg.SSLType
}

var (
	ErrInvalidType    = errors.New("invalid TLS mode")
	ErrDomainRequired = errors.New("domain required before automatic TLS")
)

func SetSSL(sslType, priKey, crtKey string) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}
	if sslType == config.SSLTypeAutoHTTP || sslType == config.SSLTypeUser || sslType == config.SSLTypeAutoDNS || sslType == config.SSLTypeDisabled {
		cfg.SSLType = sslType
	} else {
		return ErrInvalidType
	}

	if cfg.SSLType != config.SSLTypeUser && cfg.SSLType != config.SSLTypeDisabled && strings.TrimSpace(cfg.Domain) == "" {
		return ErrDomainRequired
	}

	if cfg.SSLType == config.SSLTypeUser {
		cfg.SSLPrivateKeyPath = priKey
		cfg.SSLPublicKeyPath = crtKey
		// 手动设置证书的情况下后台地址默认不启用https
		cfg.HttpsEnabled = 2
	}
	candidate, err := config.EffectiveConfig(cfg)
	if err != nil {
		return err
	}
	if candidate.Domain == "" || candidate.WebDomain == "" {
		return ErrDomainRequired
	}
	if err := config.ValidateWebRoot(candidate); err != nil {
		return err
	}
	if cfg.SSLType == config.SSLTypeUser {
		if _, err := LoadCertificate(candidate); err != nil {
			return err
		}
	}
	engine, err := db.Open(candidate, false)
	if err != nil {
		return errors.New("candidate database is not reachable")
	}
	if err = engine.Close(); err != nil {
		return err
	}

	err = config.WriteConfig(cfg)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

// Serialize requests from the wizard and scheduler, including their writes to disk.
var issuanceGate = make(chan struct{}, 1)

func Automatic(cfg *config.Config) bool {
	return cfg.SSLType == config.SSLTypeAutoHTTP || cfg.SSLType == config.SSLTypeAutoDNS
}

func certificateDomains(cfg *config.Config) []string {
	domains := make([]string, 0, 1+3*len(cfg.Domains))
	// With web HTTPS disabled in HTTP-01 mode, the reverse proxy owns the
	// web certificate and may handle its challenge path itself.
	if cfg.SSLType != config.SSLTypeAutoHTTP || cfg.HttpsEnabled != 2 {
		domains = append(domains, cfg.WebDomain)
	}
	for _, domain := range cfg.Domains {
		domains = append(domains, "smtp."+domain, "pop."+domain, "imap."+domain)
	}
	return domains
}

// LoadCertificate only reads local files and checks the leaf validity window.
// Return the loaded pair so listeners never need to reopen the files at startup.
func LoadCertificate(cfg *config.Config) (*tls.Certificate, error) {
	key, err := securefile.Read(cfg.SSLPrivateKeyPath)
	if err != nil {
		return nil, err
	}
	public, err := os.ReadFile(cfg.SSLPublicKeyPath)
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(public, key)
	if err != nil {
		return nil, err
	}
	if err := validateCertificate(&cert); err != nil {
		return nil, err
	}
	return &cert, nil
}

func validateCertificate(cert *tls.Certificate) error {
	if cert.Leaf == nil {
		if len(cert.Certificate) == 0 {
			return errors.New("TLS certificate chain is empty")
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return err
		}
		cert.Leaf = leaf
	}
	now := time.Now()
	if now.Before(cert.Leaf.NotBefore) || !now.Before(cert.Leaf.NotAfter) {
		return fmt.Errorf("TLS certificate is not currently valid (valid from %s until %s)", cert.Leaf.NotBefore, cert.Leaf.NotAfter)
	}
	return nil
}

func coversDomains(cfg *config.Config, cert *tls.Certificate) bool {
	for _, domain := range certificateDomains(cfg) {
		if cert.Leaf.VerifyHostname(domain) != nil {
			return false
		}
	}
	return true
}

func NeedsRenewal(cfg *config.Config, cert *tls.Certificate) bool {
	return time.Until(cert.Leaf.NotAfter) < 30*24*time.Hour || !coversDomains(cfg, cert)
}

// ObtainCertificate acquires or renews automatic TLS material. It never starts or restarts listeners.
func ObtainCertificate(ctx context.Context, cfg *config.Config) error {
	select {
	case issuanceGate <- struct{}{}:
		defer func() { <-issuanceGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if cfg.Domain == "" || !Automatic(cfg) {
		return errors.New("Automatic TLS requires an ACME mode and a configured domain")
	}
	key, err := config.ReadPrivateKey()
	if err != nil {
		return fmt.Errorf("load ACME account key: %w", err)
	}
	user := &MyUser{Email: "SSLMaster@" + cfg.Domain, key: key}
	conf := lego.NewConfig(user)
	conf.UserAgent = "PMail"
	conf.Certificate.KeyType = certcrypto.RSA2048
	client, err := lego.NewClient(conf)
	if err != nil {
		return err
	}
	if cfg.SSLType == config.SSLTypeAutoHTTP {
		err = client.Challenge.SetHTTP01Provider(GetHttpChallengeInstance())
	} else {
		err = client.Challenge.SetDNS01Provider(GetDnsChallengeInstance(), dns01.AddDNSTimeout(60*time.Minute))
	}
	if err != nil {
		return err
	}
	// ACME newAccount returns the existing registration for a known key. It also
	// recovers when a key was written before an earlier registration failed.
	user.Registration, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		user.Registration, err = client.Registration.ResolveAccountByKey()
		if err != nil {
			return err
		}
	}

	domains := certificateDomains(cfg)
	log.Infof("Requesting ACME certificate for %s", strings.Join(domains, ", "))
	certs, err := client.Certificate.Obtain(certificate.ObtainRequest{Domains: domains, Bundle: true})
	if err != nil {
		return err
	}
	pair, err := tls.X509KeyPair(certs.Certificate, certs.PrivateKey)
	if err != nil {
		return err
	}
	if err := validateCertificate(&pair); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, path := range []string{cfg.SSLPrivateKeyPath, cfg.SSLPublicKeyPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
	}
	if err := securefile.Write(cfg.SSLPrivateKeyPath, certs.PrivateKey, false); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.SSLPublicKeyPath, certs.Certificate, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(cfg.SSLPublicKeyPath), "issuerCert.crt"), certs.IssuerCertificate, 0644); err != nil {
		log.Warnf("Could not save ACME issuer certificate: %v", err)
	}
	return nil
}

// CheckSSLCrtInfo returns the certificate's remaining days, expiry and name mismatch.
func CheckSSLCrtInfo() (int, time.Time, bool, error) {
	cfg, err := config.ReadEffectiveConfig()
	if err != nil {
		return -1, time.Time{}, true, err
	}
	cert, err := LoadCertificate(cfg)
	if err != nil {
		return -1, time.Time{}, true, err
	}
	return int(time.Until(cert.Leaf.NotAfter).Hours() / 24), cert.Leaf.NotAfter, !coversDomains(cfg, cert), nil
}
