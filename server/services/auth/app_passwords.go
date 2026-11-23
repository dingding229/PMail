package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/password"
	"xorm.io/xorm"
)

var (
	ErrAppInput   = errors.New("app_password_invalid")
	ErrAppLimit   = errors.New("app_password_limit")
	ErrAppMissing = errors.New("app_password_not_found")
)

const appPasswordLimit = 20

type CreatedAppPassword struct {
	models.AppPassword
	Password string `json:"password"`
}

func AppPasswords(ctx context.Context, userID int) ([]models.AppPassword, error) {
	rows := []models.AppPassword{}
	err := db.Instance.Context(ctx).Where("user_id = ?", userID).Desc("id").Find(&rows)
	return rows, err
}

func CreateAppPassword(ctx context.Context, expected models.User, name string) (CreatedAppPassword, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return CreatedAppPassword{}, ErrAppInput
	}
	secret, err := randomSecret(16)
	if err != nil {
		return CreatedAppPassword{}, err
	}
	hash, err := password.Hash(secret)
	if err != nil {
		return CreatedAppPassword{}, err
	}
	row := models.AppPassword{UserID: expected.ID, Name: name, Selector: appSelector(secret), Hash: hash, CreatedAt: time.Now().UTC()}
	_, err = changeFactor(ctx, expected, func(tx *xorm.Session, u *models.User) error {
		n, err := tx.Where("user_id = ?", u.ID).Count(&models.AppPassword{})
		if err != nil {
			return err
		}
		if n >= appPasswordLimit {
			return ErrAppLimit
		}
		_, err = tx.Insert(&row)
		return err
	})
	if err != nil {
		return CreatedAppPassword{}, err
	}
	return CreatedAppPassword{row, secret}, nil
}

func DeleteAppPassword(ctx context.Context, expected models.User, id int) error {
	_, err := changeFactor(ctx, expected, func(tx *xorm.Session, u *models.User) error {
		n, err := tx.ID(id).Where("user_id = ?", u.ID).Delete(&models.AppPassword{})
		if err == nil && n != 1 {
			err = ErrAppMissing
		}
		return err
	})
	return err
}

// The indexed lookup hint is derived from the entire random password; no
// plaintext prefix is kept. A match still requires bcrypt verification.
func appSelector(secret string) string { return digest(secret)[:4] }

func authenticateApp(userID int, secret string) (int, error) {
	if len(secret) != 16 || strings.IndexFunc(secret, func(c rune) bool { return !strings.ContainsRune(secretAlphabet, c) }) >= 0 {
		return 0, nil
	}
	// The selector only narrows bcrypt candidates; it cannot authenticate.
	var rows []models.AppPassword
	if err := db.Instance.Where("user_id = ? AND selector = ?", userID, appSelector(secret)).Find(&rows); err != nil {
		return 0, err
	}
	for _, row := range rows {
		if ok, _ := password.Verify(row.Hash, secret); ok {
			return row.ID, nil
		}
	}
	return 0, nil
}

func touchApp(id, userID int, now time.Time) error {
	_, err := db.Instance.ID(id).Where("user_id = ?", userID).Cols("last_used_at").Update(&models.AppPassword{LastUsedAt: &now})
	return err
}

// ValidateMailSession rechecks both generations and the existence of the
// app credential, so deletion revokes an already authenticated connection.
func ValidateMailSession(userID int, authVersion, factorVersion int64, appID int, admin *bool) error {
	if userID <= 0 {
		return ErrCredentials
	}
	var u models.User
	query := db.Instance.ID(userID).Where("disabled = 0 AND auth_version = ? AND factor_version = ?", authVersion, factorVersion)
	if admin != nil {
		role := 0
		if *admin {
			role = 1
		}
		query.Where("is_admin = ?", role)
	}
	ok, err := query.Cols("totp_enabled").Get(&u)
	if err != nil {
		return err
	}
	if !ok || (appID == 0 && u.TOTPEnabled) {
		return ErrCredentials
	}
	if appID != 0 {
		ok, err := db.Instance.ID(appID).Where("user_id = ?", userID).Exist(&models.AppPassword{})
		if err != nil {
			return err
		}
		if !ok {
			return ErrCredentials
		}
	}
	return nil
}
