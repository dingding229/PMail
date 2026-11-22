package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func ReadEffectiveConfig() (*Config, error) {
	cfg, err := ReadConfig()
	if err != nil {
		return nil, err
	}
	mode := os.Getenv("PMAIL_WEB_MODE")
	// Preserve settings from a legacy standalone config unless a mode was
	// requested. Compose always supplies its selected mode.
	if mode != "" || cfg.Domain == "" {
		if err := applyWebMode(cfg, mode); err != nil {
			return nil, err
		}
	}
	for name, dst := range map[string]*string{
		"PMAIL_DB_TYPE": &cfg.DbType,
		"PMAIL_DOMAIN":  &cfg.Domain, "PMAIL_WEB_DOMAIN": &cfg.WebDomain,
		"PMAIL_SSL_TYPE": &cfg.SSLType, "PMAIL_LOG_LEVEL": &cfg.LogLevel,
	} {
		if value, ok := os.LookupEnv(name); ok && value != "" {
			*dst = value
		}
	}
	if cfg.DbType == "" {
		cfg.DbType = DBTypePostgres
	}
	if value := os.Getenv("PMAIL_SSL_TYPE"); value != "" &&
		!slices.Contains([]string{SSLTypeAutoHTTP, SSLTypeUser, SSLTypeAutoDNS, SSLTypeDisabled}, value) {
		return nil, fmt.Errorf("PMAIL_SSL_TYPE must be 0, 1, 2 or 3")
	}
	if !slices.Contains(DBTypes, cfg.DbType) {
		return nil, fmt.Errorf("database type must be postgres, sqlite or mysql")
	}
	if dsn := os.Getenv("PMAIL_DB_DSN"); dsn != "" {
		cfg.DbDSN = dsn
	} else if path := os.Getenv("PMAIL_DB_PASSWORD_FILE"); path != "" && cfg.DbType == DBTypePostgres {
		cfg.DbDSN, err = postgresDSN(path)
		if err != nil {
			return nil, err
		}
	}
	if domains, ok := os.LookupEnv("PMAIL_DOMAINS"); ok {
		cfg.Domains = strings.Split(domains, ",")
	}
	var domains []string
	cfg.Domain = strings.TrimSpace(cfg.Domain)
	for _, domain := range append([]string{cfg.Domain}, cfg.Domains...) {
		domain = strings.TrimSpace(domain)
		if domain != "" && !slices.Contains(domains, domain) {
			domains = append(domains, domain)
		}
	}
	cfg.Domains = domains
	if cfg.WebDomain == "" && cfg.Domain != "" {
		cfg.WebDomain = "mail." + cfg.Domain
	}
	for _, setting := range []struct {
		name string
		dst  *int
		min  int
		max  int
	}{
		{"PMAIL_HTTP_PORT", &cfg.HttpPort, 1, 65535},
		{"PMAIL_HTTPS_PORT", &cfg.HttpsPort, 1, 65535},
		{"PMAIL_HTTPS_ENABLED", &cfg.HttpsEnabled, 0, 2},
		{"PMAIL_SPAM_FILTER_LEVEL", &cfg.SpamFilterLevel, 0, 3},
	} {
		if value := os.Getenv(setting.name); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < setting.min || n > setting.max {
				return nil, fmt.Errorf("%s must be an integer in [%d, %d]", setting.name, setting.min, setting.max)
			}
			*setting.dst = n
		}
	}

	if value, ok := os.LookupEnv("PMAIL_CATCH_ALL"); ok && value != "" {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("PMAIL_CATCH_ALL must be true or false")
		}
		cfg.CatchAll = b
	}
	if value, ok := os.LookupEnv("PMAIL_ALLOW_USER_ALIAS"); ok && value != "" {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("PMAIL_ALLOW_USER_ALIAS must be true or false")
		}
		cfg.AllowUserAlias = b
	}
	if cfg.SSLType == SSLTypeDisabled {
		cfg.HttpsEnabled = 2
	}
	cfg.fixPaths()
	return cfg, nil
}

func applyWebMode(cfg *Config, mode string) error {
	switch mode {
	case "", "proxy":
		cfg.HttpsEnabled = 2
	case "public":
		cfg.HttpsEnabled = 0
	default:
		return fmt.Errorf("PMAIL_WEB_MODE must be proxy or public")
	}
	cfg.SSLType = SSLTypeAutoHTTP
	return nil
}

func BootstrapCredentials() (account, password string, ok bool) {
	account = strings.TrimSpace(os.Getenv("PMAIL_ADMIN_ACCOUNT"))
	if account == "" {
		account = "admin"
	}
	password = os.Getenv("PMAIL_ADMIN_PASSWORD")
	ok = Instance.DbDSN != "" && strings.TrimSpace(os.Getenv("PMAIL_DOMAIN")) != ""
	return
}

func loadSecretKey() (string, error) {
	if key := os.Getenv("PMAIL_SECRET_KEY"); key != "" {
		return key, nil
	}
	key, err := loadOrCreateSecret(filepath.Join(ROOT_PATH, "config", "secret.key"), "", 32)
	if err != nil {
		return "", err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("PMail secret.key is empty; restore it or configure PMAIL_SECRET_KEY")
	}
	return key, nil
}

func WebRoot() string {
	if value := strings.TrimSpace(os.Getenv("PMAIL_WEB_ROOT")); value != "" {
		return value
	}
	return "/work/web"
}

func DefaultLang() string {
	if value := strings.TrimSpace(os.Getenv("PMAIL_DEFAULT_LANG")); value != "" {
		return value
	}
	return "zh-CN"
}

func CORSOrigins() []string { return strings.Split(os.Getenv("PMAIL_CORS_ORIGINS"), ",") }
