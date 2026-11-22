package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// InitDatabasePassword prepares the password file before PostgreSQL starts.
// An explicit legacy password must agree with an already provisioned file.
func InitDatabasePassword(path, password string) error {
	if strings.HasSuffix(password, "\n") {
		return fmt.Errorf("POSTGRES_PASSWORD must not end with a newline when using a password file")
	}
	value, err := loadOrCreateSecret(path, password, 24)
	if err != nil {
		return err
	}
	if password != "" && value != password {
		return fmt.Errorf("POSTGRES_PASSWORD differs from the persisted password in %s; restore the matching credential (the database password was not changed)", path)
	}
	return nil
}

// InitialAdminPassword is called only if the bootstrap account does not exist.
// Persist before inserting the account so a failed bootstrap can be retried.
func InitialAdminPassword() (string, error) {
	return loadOrCreateSecret(filepath.Join(ROOT_PATH, "config", "initial_admin_password"), "", 24)
}

func readSecret(path string) (string, error) {
	data, err := os.ReadFile(path)
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
