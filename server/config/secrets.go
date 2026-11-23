package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/Jinnrry/pmail/utils/securefile"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// InitDatabasePassword prepares the password file before PostgreSQL starts.
// An explicit legacy password must agree with an already provisioned file.
func InitDatabasePassword(path, password string) error {
	if strings.HasSuffix(password, "\n") {
		return fmt.Errorf("POSTGRES_PASSWORD must not end with a newline when using a password file")
	}
	uid, err := postgresSecretID("PMAIL_DB_SECRET_UID")
	if err != nil {
		return err
	}
	gid, err := postgresSecretID("PMAIL_DB_SECRET_GID")
	if err != nil {
		return err
	}
	if err := ensureSearchableSecretDir(filepath.Dir(path)); err != nil {
		return err
	}
	value, err := loadOrCreateSecret(path, password, 24)
	if err != nil {
		return err
	}
	if password != "" && value != password {
		return fmt.Errorf("POSTGRES_PASSWORD differs from the persisted password in %s; restore the matching credential (the database password was not changed)", path)
	}
	file, err := openDBSecret(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return fmt.Errorf("set database password file permissions: %w", err)
	}
	if err := file.Chown(uid, gid); err != nil {
		return fmt.Errorf("set database password file owner to %d:%d: %w", uid, gid, err)
	}
	return nil
}

func postgresSecretID(name string) (int, error) {
	// postgres:17-alpine uses UID and GID 70.
	id, err := strconv.Atoi(envDefault(name, "70"))
	if err != nil || id < 0 || uint64(id) >= 0xffffffff {
		return 0, fmt.Errorf("%s must be a valid numeric UID or GID", name)
	}
	return id, nil
}

func ensureSearchableSecretDir(dir string) error {
	err := os.MkdirAll(dir, 0711) // #nosec G301 -- PostgreSQL UID needs traverse only; others cannot list/write the directory or read the 0600 password.
	if err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	// Let PostgreSQL traverse the mount, including an existing root-owned
	// 0700 directory. Do not grant directory listing or file read access.
	if info.Mode().Perm()&0111 != 0111 {
		if err := os.Chmod(dir, info.Mode()|0111); err != nil {
			return fmt.Errorf("allow access through database secrets directory: %w", err)
		}
	}
	return nil
}

// Exactly 32 symbols: removing O/I/0/1 leaves five unbiased bits per
// character, for 80 bits of entropy in sixteen characters.
const registrationAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func RegistrationKeyPath() string { return filepath.Join(ROOT_PATH, "config", "registration_key") }

func EnsureRegistrationKey() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	var key strings.Builder
	for i, b := range random {
		if i > 0 && i%4 == 0 {
			key.WriteByte('-')
		}
		key.WriteByte(registrationAlphabet[b&31])
	}
	path := RegistrationKeyPath()
	if err := securefile.Write(path, []byte(key.String()+"\n"), true); err != nil && !os.IsExist(err) {
		return "", err
	}
	return ReadRegistrationKey()
}

func ReadRegistrationKey() (string, error) {
	data, err := securefile.Read(RegistrationKeyPath())
	if err != nil {
		return "", err
	}
	return validateRegistrationKey(strings.TrimRight(string(data), "\n"))
}

func validateRegistrationKey(value string) (string, error) {
	if len(value) == 19 {
		valid := true
		for i := range len(value) {
			if i%5 == 4 {
				valid = valid && value[i] == '-'
			} else {
				valid = valid && strings.ContainsRune(registrationAlphabet, rune(value[i]))
			}
		}
		if valid {
			return value, nil
		}
	}
	return "", fmt.Errorf("invalid registration key format in %s", RegistrationKeyPath())
}

func RemoveRegistrationKey() error {
	err := os.Remove(RegistrationKeyPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Secret paths originate from an operator's *_FILE setting or the local
// init-secrets command. The descriptor is confined to its configured parent.
func openDBSecret(path string) (*os.File, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dir, err := os.OpenRoot(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	name := filepath.Base(abs)
	info, err := dir.Stat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("database secret must be a regular file")
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil, err
	}
	current, err := f.Stat()
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) {
		f.Close()
		return nil, fmt.Errorf("database secret changed while opening")
	}
	return f, nil
}

func readSecret(path string) (string, error) {
	f, err := openDBSecret(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	const maxDBSecret = 64 << 10
	data, err := io.ReadAll(io.LimitReader(f, maxDBSecret+1))
	if err == nil && len(data) > maxDBSecret {
		return "", fmt.Errorf("database secret exceeds size limit")
	}
	if err != nil {
		return "", err
	}
	// Match the PostgreSQL image's shell command substitution for *_FILE.
	value := strings.TrimRight(string(data), "\n")
	if value == "" {
		return "", fmt.Errorf("secret file %s is empty; restore it before starting PMail", path)
	}
	return value, nil
}

func loadOrCreateSecret(path, value string, randomBytes int) (string, error) {
	existing, err := readSecret(path)
	if !os.IsNotExist(err) {
		return existing, err
	}
	if value == "" {
		data := make([]byte, randomBytes)
		if _, err := rand.Read(data); err != nil {
			return "", err
		}
		value = hex.EncodeToString(data)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	// CreateTemp uses 0600. Link the completed file atomically without replacing
	// a secret created by another process.
	file, err := os.CreateTemp(filepath.Dir(path), ".pmail-secret-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	_, err = file.WriteString(value + "\n")
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Link(file.Name(), path); os.IsExist(err) {
		return readSecret(path)
	} else if err != nil {
		return "", err
	}
	return value, nil
}

func postgresDSN(path string) (string, error) {
	password, err := readSecret(path)
	if err != nil {
		return "", fmt.Errorf("read PMAIL_DB_PASSWORD_FILE: %w", err)
	}
	host, port := envDefault("PMAIL_DB_HOST", "postgres"), envDefault("PMAIL_DB_PORT", "5432")
	name := envDefault("PMAIL_DB_NAME", "pmail")
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(envDefault("PMAIL_DB_USER", "pmail"), password),
		Host:     net.JoinHostPort(host, port),
		Path:     "/" + name,
		RawPath:  "/" + url.PathEscape(name),
		RawQuery: "sslmode=disable",
	}
	return dsn.String(), nil
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
