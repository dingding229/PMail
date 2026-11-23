package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func Flag(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true":
		return true
	}
	return false
}

func CIDRs(value string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q", item)
		}
		if prefix.Addr().Is4In6() {
			if prefix.Bits() < 96 {
				return nil, fmt.Errorf("ambiguous mapped CIDR %q", item)
			}
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		out = append(out, prefix.Masked())
	}
	return out, nil
}

// canonicalPath also resolves symlinks in existing ancestors of a path that
// will be created after startup. Errors other than ENOENT fail closed.
func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) || abs == filepath.Dir(abs) {
		return "", err
	}
	if info, statErr := os.Lstat(abs); statErr == nil && info.Mode()&os.ModeSymlink != 0 { // #nosec G703 -- abs is a cleaned admin startup path; Lstat detects dangling links without opening file data.
		return "", err
	}
	dir, err := canonicalPath(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(abs)), nil
}

func containsPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func ValidateWebRoot(cfg *Config) error {
	root, err := canonicalPath(WebRoot())
	if err != nil {
		return fmt.Errorf("validate PMAIL_WEB_ROOT: %w", err)
	}
	sensitive := []string{filepath.Join(ROOT_PATH, "config"), filepath.Join(ROOT_PATH, "data")}
	for _, file := range []string{cfg.SSLPrivateKeyPath, cfg.DkimPrivateKeyPath, os.Getenv("PMAIL_DB_PASSWORD_FILE")} {
		if file != "" {
			sensitive = append(sensitive, file, filepath.Dir(file))
		}
	}
	if cfg.DbType == DBTypeSQLite && cfg.DbDSN != ":memory:" && cfg.DbDSN != "" {
		dsn := cfg.DbDSN
		uri := strings.HasPrefix(dsn, "file:")
		if uri {
			u, err := url.Parse(dsn)
			if err != nil || (u.Host != "" && !strings.EqualFold(u.Host, "localhost")) || u.Fragment != "" {
				return fmt.Errorf("invalid SQLite file URI")
			}
			switch {
			case u.Query().Get("mode") == "memory":
				dsn = ""
			case u.Opaque != "":
				dsn, err = url.PathUnescape(u.Opaque)
				if err != nil {
					return fmt.Errorf("invalid SQLite file URI")
				}
			default:
				dsn = u.Path
			}
		}
		if dsn != "" && dsn != ":memory:" {
			addDB := func(path string) { sensitive = append(sensitive, path, path+"-wal", path+"-shm", path+"-journal") }
			if !filepath.IsAbs(dsn) && uri {
				addDB(dsn)
			} // SQLite resolves relative URIs from cwd.
			if !filepath.IsAbs(dsn) {
				dsn = filepath.Join(ROOT_PATH, dsn)
			}
			addDB(dsn)
		}
	}
	for _, path := range sensitive {
		if path == "" {
			continue
		}
		p, err := canonicalPath(path)
		if err != nil {
			return fmt.Errorf("validate sensitive filesystem path: %w", err)
		}
		if containsPath(root, p) || containsPath(p, root) {
			return fmt.Errorf("PMAIL_WEB_ROOT %q overlaps a configuration, key or data path", root)
		}
	}
	return nil
}
