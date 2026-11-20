package domain

import (
	"crypto"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/dkimkey"
	"golang.org/x/net/idna"
	"xorm.io/xorm"
)

var (
	ErrInvalid  = errors.New("invalid domain name")
	ErrNotFound = errors.New("domain not found")
	ErrExists   = errors.New("domain already exists")
	ErrInUse    = errors.New("domain has mailbox addresses")
	ErrDefault  = errors.New("set a different default domain before deleting this domain")
)

type entry struct {
	models.Domain
	signer crypto.Signer
}

type snapshot struct {
	ordered []models.Domain
	byName  map[string]entry
	byID    map[int]models.Domain
}

var cache struct {
	sync.RWMutex
	snapshot
}

func Normalize(value string) (string, error) {
	name, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if err != nil || name == "" || len(name) > 253 {
		return "", ErrInvalid
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrInvalid
		}
		for _, c := range strings.ToLower(label) {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return "", ErrInvalid
			}
		}
	}
	return strings.ToLower(name), nil
}

func Initialize(cfg *config.Config) error {
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		n, err := tx.Count(&models.Domain{})
		if err != nil || n != 0 {
			return err
		}
		configured := append([]string{cfg.Domain}, cfg.Domains...)
		seen := make(map[string]bool)
		for _, raw := range configured {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			name, err := Normalize(raw)
			if err != nil {
				return fmt.Errorf("import configured domain %q: %w", raw, err)
			}
			if seen[name] {
				continue
			}
			d := models.Domain{Name: name, IsDefault: len(seen) == 0, DkimSelector: "default"}
			seen[name] = true
			if d.IsDefault && cfg.DkimPrivateKeyPath != "" {
				key, err := os.ReadFile(cfg.DkimPrivateKeyPath)
				if err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("read legacy DKIM key: %w", err)
				}
				if err == nil {
					if _, err := dkimkey.Parse(string(key)); err != nil {
						return fmt.Errorf("read legacy DKIM key: %w", err)
					}
					d.DkimPrivateKey = string(key)
				}
			}
			if d.DkimPrivateKey == "" {
				d.DkimPrivateKey, err = dkimkey.Generate()
				if err != nil {
					return err
				}
			}
			if _, err := tx.Insert(&d); err != nil {
				return err
			}
		}
		if len(seen) == 0 {
			return errors.New("no mail domain configured")
		}
		return nil
	})
	if err != nil {
		return err
	}
	return refresh()
}

func Refresh() error {
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	return refresh()
}

func refresh() error {
	var records []models.Domain
	if err := db.Instance.Desc("is_default").Asc("id").Find(&records); err != nil {
		return err
	}
	next := snapshot{ordered: make([]models.Domain, 0, len(records)), byName: make(map[string]entry), byID: make(map[int]models.Domain)}
	defaults := 0
	for _, d := range records {
		if d.IsDefault {
			defaults++
		}
		signer, err := dkimkey.Parse(d.DkimPrivateKey)
		if err != nil {
			return fmt.Errorf("load DKIM key for %s: %w", d.Name, err)
		}
		d.DkimPrivateKey = ""
		next.ordered = append(next.ordered, d)
		next.byName[d.Name] = entry{d, signer}
		next.byID[d.ID] = d
	}
	if defaults != 1 {
		return fmt.Errorf("expected one default mail domain, found %d", defaults)
	}
	cache.Lock()
	cache.snapshot = next
	cache.Unlock()
	return nil
}

func List() []models.Domain {
	cache.RLock()
	defer cache.RUnlock()
	return append([]models.Domain{}, cache.ordered...)
}
func Default() models.Domain {
	cache.RLock()
	defer cache.RUnlock()
	if len(cache.ordered) == 0 {
		return models.Domain{}
	}
	return cache.ordered[0]
}
func DefaultName() string { return Default().Name }
func Names() []string {
	cache.RLock()
	defer cache.RUnlock()
	names := make([]string, 0, len(cache.ordered))
	for _, d := range cache.ordered {
		names = append(names, d.Name)
	}
	return names
}
func Get(name string) (models.Domain, bool) {
	name, err := Normalize(name)
	if err != nil {
		return models.Domain{}, false
	}
	cache.RLock()
	defer cache.RUnlock()
	d, ok := cache.byName[name]
	return d.Domain, ok
}
func GetByID(id int) (models.Domain, bool) {
	cache.RLock()
	defer cache.RUnlock()
	d, ok := cache.byID[id]
	return d, ok
}
func IsLocal(name string) bool { _, ok := Get(name); return ok }
func Signer(name string) (models.Domain, crypto.Signer, bool) {
	name, err := Normalize(name)
	if err != nil {
		return models.Domain{}, nil, false
	}
	cache.RLock()
	defer cache.RUnlock()
	e, ok := cache.byName[name]
	return e.Domain, e.signer, ok
}

func Create(name string) (models.Domain, error) {
	name, err := Normalize(name)
	if err != nil {
		return models.Domain{}, err
	}
	key, err := dkimkey.Generate()
	if err != nil {
		return models.Domain{}, err
	}
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	d := models.Domain{Name: name, DkimSelector: "default", DkimPrivateKey: key}
	if _, err := db.Instance.Insert(&d); err != nil {
		return models.Domain{}, err
	}
	d.DkimPrivateKey = ""
	return d, refresh()
}

func Delete(id int) error {
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	if err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		var d models.Domain
		ok, err := tx.ID(id).Get(&d)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		if d.IsDefault {
			return ErrDefault
		}
		n, err := tx.Where("domain_id = ?", id).Count(&models.MailboxAddress{})
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrInUse
		}
		_, err = tx.ID(id).Delete(&models.Domain{})
		return err
	}); err != nil {
		return err
	}
	return refresh()
}

func SetDefault(id int) error {
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	if err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		var d models.Domain
		ok, err := tx.ID(id).Get(&d)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		if d.IsDefault {
			return nil
		}
		if _, err := tx.Table(&models.Domain{}).Where("is_default = ?", true).Update(map[string]any{"is_default": false}); err != nil {
			return err
		}
		_, err = tx.ID(id).Cols("is_default").Update(&models.Domain{IsDefault: true})
		return err
	}); err != nil {
		return err
	}
	return refresh()
}
