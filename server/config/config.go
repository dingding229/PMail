package config

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/errors"
	log "github.com/sirupsen/logrus"
)

var IsInit bool

type Config struct {
	LogLevel           string   `json:"logLevel"`
	Domain             string   `json:"domain"`
	Domains            []string `json:"domains"`
	WebDomain          string   `json:"webDomain"`
	DkimPrivateKeyPath string   `json:"dkimPrivateKeyPath"`
	SSLType            string   `json:"sslType"`
	SSLPrivateKeyPath  string   `json:"SSLPrivateKeyPath"`
	SSLPublicKeyPath   string   `json:"SSLPublicKeyPath"`
	DbDSN              string   `json:"dbDSN"`
	DbType             string   `json:"dbType"`
	HttpsEnabled       int      `json:"httpsEnabled"`
	SpamFilterLevel    int      `json:"spamFilterLevel"`
	HttpPort           int      `json:"httpPort"`
	HttpsPort          int      `json:"httpsPort"`
	// Obsolete notification settings are retained for compatibility when rewriting legacy configuration.
	WeChatPushAppId      string            `json:"weChatPushAppId"`
	WeChatPushSecret     string            `json:"weChatPushSecret"`
	WeChatPushTemplateId string            `json:"weChatPushTemplateId"`
	WeChatPushUserId     string            `json:"weChatPushUserId"`
	TgBotToken           string            `json:"tgBotToken"`
	TgChatId             string            `json:"tgChatId"`
	SecretKey            string            `json:"-"`
	CatchAll             bool              `json:"catchAll"`
	AllowUserAlias       bool              `json:"allowUserAlias"`
	IsInit               bool              `json:"isInit"`
	WebPushUrl           string            `json:"webPushUrl"`
	WebPushToken         string            `json:"webPushToken"`
	Tables               map[string]string `json:"-"`
	TablesInitData       map[string]string `json:"-"`
	setupPort            int
}

var ROOT_PATH = ""

func init() {
	if root := os.Getenv("PMail_ROOT"); root != "" {
		ROOT_PATH = ensureTrailingSlash(root)
		fmt.Println("Env Root Path:", ROOT_PATH)
		return
	}
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	realPath, err := filepath.EvalSymlinks(filepath.Dir(ex))
	if err != nil {
		panic(err)
	}
	if strings.Contains(realPath, "GoLand") && strings.Contains(realPath, "JetBrains") {
		return
	}
	ROOT_PATH = ensureTrailingSlash(realPath)
	fmt.Println("Root Path:", ROOT_PATH)
}

func ensureTrailingSlash(path string) string {
	path = filepath.Clean(path)
	if strings.HasSuffix(path, string(os.PathSeparator)) {
		return path
	}
	return path + string(os.PathSeparator)
}

func (c *Config) GetSetupPort() int     { return c.setupPort }
func (c *Config) SetSetupPort(port int) { c.setupPort = port }

const (
	DBTypeMySQL     = "mysql"
	DBTypeSQLite    = "sqlite"
	DBTypePostgres  = "postgres"
	SSLTypeAutoHTTP = "0"
	SSLTypeUser     = "1"
	SSLTypeAutoDNS  = "2"
	SSLTypeDisabled = "3"
)

var DBTypes = []string{DBTypePostgres, DBTypeMySQL, DBTypeSQLite}
var Instance = &Config{}

type logFormatter struct{}

func (l *logFormatter) Format(entry *log.Entry) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("[%s]", entry.Level.String()))
	b.WriteString(fmt.Sprintf("[%s]", entry.Time.Format("2006-01-02 15:04:05")))
	if entry.Context != nil {
		if ctx, ok := entry.Context.(*context.Context); ok && ctx != nil {
			b.WriteString(fmt.Sprintf("[%s]", ctx.GetValue(context.LogID)))
		}
	}
	b.WriteString(fmt.Sprintf("[%s:%d]", entry.Caller.File, entry.Caller.Line))
	b.WriteString(entry.Message)
	b.WriteByte('\n')
	return b.Bytes(), nil
}

func Init() error {
	cfg, err := ReadEffectiveConfig()
	if err != nil {
		return err
	}
	cfg.SecretKey, err = loadSecretKey()
	if err != nil {
		return err
	}
	if Instance != nil {
		cfg.setupPort = Instance.setupPort
	}
	Instance = cfg
	IsInit = cfg.IsInit

	log.SetFormatter(&logFormatter{})
	log.SetReportCaller(true)
	log.SetOutput(os.Stdout)
	time.Local = time.FixedZone("CST", 8*3600)
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "warn", "warning":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
	return nil
}

// ReadPrivateKey loads or creates the local ACME account key. A bad or unreadable
// key is reported to the caller so certificate maintenance can retry safely.
func ReadPrivateKey() (*ecdsa.PrivateKey, error) {
	path := filepath.Join(ROOT_PATH, "config", "ssl", "account_private.pem")
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("invalid PEM in ACME account key %s", path)
		}
		return x509.ParseECPrivateKey(block.Bytes)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	encoded, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := writeConfigFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encoded})); err != nil {
		return nil, err
	}
	return key, nil
}

func WriteConfig(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return errors.Wrap(err)
	}
	return writeConfigFile(configPath(), data)
}

func configPath() string {
	name := "config.json"
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "dev" {
		name = "config.dev.json"
	}
	return filepath.Join(ROOT_PATH, "config", name)
}

// ReadConfig reads persisted settings without applying environment overrides.
func ReadConfig() (*Config, error) {
	cfg := &Config{
		DbType:             DBTypePostgres,
		CatchAll:           true,
		SSLType:            SSLTypeAutoHTTP,
		DkimPrivateKeyPath: "config/dkim/dkim.priv",
		SSLPrivateKeyPath:  "config/ssl/private.key",
		SSLPublicKeyPath:   "config/ssl/public.crt",
	}
	data, err := os.ReadFile(configPath())
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read PMail configuration: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse PMail configuration: %w", err)
	}
	return cfg, nil
}

func writeConfigFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".pmail-config-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (c *Config) fixPaths() {
	if c.DbType == DBTypeSQLite && c.DbDSN != "" && c.DbDSN != ":memory:" &&
		!strings.HasPrefix(c.DbDSN, "file:") && !filepath.IsAbs(c.DbDSN) {
		c.DbDSN = filepath.Join(ROOT_PATH, c.DbDSN)
	}
	for _, path := range []*string{&c.SSLPublicKeyPath, &c.SSLPrivateKeyPath, &c.DkimPrivateKeyPath} {
		if *path != "" && !filepath.IsAbs(*path) {
			*path = filepath.Join(ROOT_PATH, *path)
		}
	}
}
