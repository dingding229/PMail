package user

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/password"
	"xorm.io/xorm"
)

var (
	ErrInvalidQuota = errors.New("invalid user quota")
	ErrBadPassword  = errors.New("incorrect current password")
	ErrInvalid      = errors.New("invalid user parameters")
	ErrNotFound     = errors.New("user not found")
	ErrExists       = errors.New("account already exists")
	ErrLastAdmin    = errors.New("cannot disable the last active administrator")
	ErrRevoked      = errors.New("session was revoked")
)

type View struct {
	models.User
	DisplayName string         `json:"display_name"`
	Addresses   []address.View `json:"addresses"`
	UsedBytes   int64          `json:"used_bytes"`
}

func ValidName(name string) bool {
	return strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 50
}
func validQuota(mb int64) bool { return mb >= 0 && mb <= quota.MaxMB }

func Create(account, name, secret string, disabled int, mb int64) (models.User, error) {
	local, err := address.NormalizeLocal(account)
	if !validQuota(mb) {
		return models.User{}, ErrInvalidQuota
	}
	if err != nil || utf8.RuneCountInString(local) > 20 || !ValidName(name) || (disabled != 0 && disabled != 1) {
		return models.User{}, ErrInvalid
	}
	hash, err := password.Hash(secret)
	if err != nil {
		return models.User{}, err
	}
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	var created models.User
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		found, err := tx.Where("LOWER(account) = ?", local).Exist(&models.User{})
		if err != nil {
			return err
		}
		if found {
			return ErrExists
		}
		created = models.User{Account: local, Name: name, Password: hash, Disabled: disabled, QuotaMB: mb}
		if _, err := tx.Insert(&created); err != nil {
			if db.IsUniqueViolation(err) {
				return ErrExists
			}
			return err
		}
		return address.Seed(tx, created)
	})
	return created, err
}

func Get(id int) (models.User, error) {
	var u models.User
	found, err := db.Instance.ID(id).Get(&u)
	if err != nil {
		return u, err
	}
	if !found {
		return u, ErrNotFound
	}
	return u, nil
}

func Views(rows []models.User) ([]View, error) {
	out := make([]View, 0, len(rows))
	ids := make([]int, 0, len(rows))
	for _, u := range rows {
		ids = append(ids, u.ID)
	}
	use, err := quota.Usage(ids)
	if err != nil {
		return nil, err
	}
	for _, u := range rows {
		addresses, err := address.List(u.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, View{User: u, DisplayName: u.Name, Addresses: addresses, UsedBytes: use[u.ID]})
	}
	return out, nil
}

type Patch struct {
	ID       int
	Account  string
	Name     string
	Password string
	Disabled *int
	QuotaMB  *int64
	Lang     *string
}

func Edit(p Patch) (models.User, error) {
	var updated models.User
	var hash string
	if p.Password != "" {
		var err error
		hash, err = password.Hash(p.Password)
		if err != nil {
			return updated, err
		}
	}
	if p.QuotaMB != nil && !validQuota(*p.QuotaMB) {
		return updated, ErrInvalidQuota
	}
	if (p.ID <= 0 && p.Account == "") || (p.Name != "" && !ValidName(p.Name)) || (p.Disabled != nil && *p.Disabled != 0 && *p.Disabled != 1) {
		return updated, ErrInvalid
	}
	db.MailboxRegistry.Lock()
	defer db.MailboxRegistry.Unlock()
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		updated = models.User{}
		if p.ID > 0 {
			tx.ID(p.ID)
		} else {
			tx.Where("account = ?", p.Account)
		}
		found, err := tx.Get(&updated)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		updates := map[string]any{}
		revoke := false
		if p.Name != "" {
			updated.Name = p.Name
			updates["name"] = p.Name
		}
		if p.Lang != nil {
			updated.Lang = *p.Lang
			updates["lang"] = *p.Lang
		}
		if p.QuotaMB != nil {
			updated.QuotaMB = *p.QuotaMB
			updates["quota_mb"] = *p.QuotaMB
		}
		if p.Disabled != nil && updated.Disabled != *p.Disabled {
			if updated.IsAdmin == 1 && *p.Disabled == 1 {
				n, err := tx.Where("is_admin = 1 AND disabled = 0").Count(&models.User{})
				if err != nil {
					return err
				}
				if n <= 1 {
					return ErrLastAdmin
				}
			}
			updated.Disabled = *p.Disabled
			updates["disabled"] = *p.Disabled
			revoke = true
		}
		if hash != "" {
			updates["password"] = hash
			revoke = true
		}
		if len(updates) == 0 {
			return nil
		}
		tx.Table(&models.User{}).ID(updated.ID)
		if revoke {
			tx.Incr("auth_version")
			updated.AuthVersion++
		}
		_, err = tx.Update(updates)
		return err
	})
	return updated, err
}

func ChangePassword(id int, version int64, old, next string) error {
	u, err := Get(id)
	if err != nil {
		return err
	}
	if u.Disabled != 0 || u.AuthVersion != version {
		return ErrRevoked
	}
	if valid, _ := password.Verify(u.Password, old); !valid {
		return ErrBadPassword
	}
	hash, err := password.Hash(next)
	if err != nil {
		return err
	}
	n, err := db.Instance.Table(&models.User{}).ID(id).Where("disabled = 0 AND auth_version = ?", version).Incr("auth_version").Update(map[string]any{"password": hash})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrRevoked
	}
	return nil
}
