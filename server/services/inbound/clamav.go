package inbound

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

var ErrClamAVEnv = errors.New("clamav_env_controlled")

// Only the documented booleans override persisted settings. An absent,
// empty, or invalid setting is not an instruction to change the database.
func ClamAVOverride() (enabled, controlled bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PMAIL_CLAMAV"))) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func ClamAVPolicy(ctx context.Context) (bool, bool, error) {
	if enabled, controlled := ClamAVOverride(); controlled {
		return enabled, true, nil
	}
	var row models.InboundSettings
	if db.Instance != nil {
		if _, err := db.Instance.Context(ctx).ID(1).Cols("clamav_enabled").Get(&row); err != nil {
			return false, false, err
		}
	}
	return row.ClamAVEnabled, false, nil
}

func SetClamAV(ctx context.Context, enabled bool) error {
	if _, controlled := ClamAVOverride(); controlled {
		return ErrClamAVEnv
	}
	settingsWrite.Lock()
	defer settingsWrite.Unlock()
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		exists, err := tx.ID(1).Exist(&models.InboundSettings{})
		if err != nil {
			return err
		}
		if !exists {
			_, err = tx.Insert(&models.InboundSettings{ID: 1, ZonesJSON: "[]", DefaultSpamThreshold: 6, ClamAVEnabled: enabled})
			return err
		}
		_, err = tx.ID(1).Cols("clamav_enabled").Update(&models.InboundSettings{ClamAVEnabled: enabled})
		return err
	})
}
