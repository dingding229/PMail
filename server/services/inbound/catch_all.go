package inbound

import (
	"context"
	"database/sql"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

type CatchAllPolicy struct {
	Enabled       bool `json:"enabled"`
	EnvControlled bool `json:"env_controlled"`
	RecipientID   int  `json:"recipient_id"` // 0 selects the oldest enabled administrator
}

type CatchAllUpdate struct {
	Enabled     *bool `json:"enabled"`
	RecipientID *int  `json:"recipient_id"`
}

type CatchAllOption struct {
	ID      int    `json:"id"`
	Account string `json:"account"`
	Name    string `json:"name"`
}

func catchAllPolicy(row models.InboundSettings) (CatchAllPolicy, error) {
	// NULL preserves the legacy config fallback until the operator first
	// saves a choice. Fresh configurations default to enabled.
	policy := CatchAllPolicy{Enabled: true, RecipientID: row.CatchAllRecipientID}
	if row.CatchAllEnabled.Valid {
		policy.Enabled = row.CatchAllEnabled.Bool
	} else if config.Instance != nil {
		policy.Enabled = config.Instance.CatchAll
	}
	enabled, set, err := config.CatchAllOverride()
	if set {
		policy.Enabled, policy.EnvControlled = enabled, true
	}
	return policy, err
}

func updateCatchAll(tx *xorm.Session, row *models.InboundSettings, change CatchAllUpdate) error {
	_, locked, err := config.CatchAllOverride()
	if err != nil {
		return err
	}
	if change.Enabled != nil && !locked {
		row.CatchAllEnabled = sql.NullBool{Bool: *change.Enabled, Valid: true}
	}
	if id := change.RecipientID; id != nil && *id != row.CatchAllRecipientID {
		if *id < 0 {
			return ErrInvalid
		}
		if *id != 0 {
			ok, err := tx.ID(*id).Where("disabled = 0").Exist(&models.User{})
			if err != nil {
				return err
			}
			if !ok {
				return ErrInvalid
			}
		}
		row.CatchAllRecipientID = *id
	}
	return nil
}

// CatchAllRecipient returns at most one enabled user. A missing or disabled
// configured recipient never causes delivery to a disabled user, or fan-out.
func CatchAllRecipient(ctx context.Context) (models.User, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var row models.InboundSettings
	if _, err := db.Instance.Context(ctx).ID(1).Cols("catch_all_enabled", "catch_all_recipient_id").Get(&row); err != nil {
		return models.User{}, false, err
	}
	p, err := catchAllPolicy(row)
	if err != nil || !p.Enabled {
		return models.User{}, false, err
	}
	var user models.User
	if p.RecipientID != 0 {
		ok, err := db.Instance.Context(ctx).ID(p.RecipientID).Where("disabled = 0").Get(&user)
		if ok || err != nil {
			return user, ok, err
		}
	}
	ok, err := db.Instance.Context(ctx).Where("is_admin = 1 AND disabled = 0").Asc("created", "id").Get(&user)
	return user, ok, err
}

// CatchAllOptions is only exposed by the admin settings handler.
func CatchAllOptions(ctx context.Context) ([]CatchAllOption, error) {
	users := []models.User{}
	if err := db.Instance.Context(ctx).Cols("id", "account", "name").Where("disabled = 0").Asc("id").Find(&users); err != nil {
		return nil, err
	}
	options := make([]CatchAllOption, 0, len(users))
	for _, u := range users {
		options = append(options, CatchAllOption{u.ID, u.Account, u.Name})
	}
	return options, nil
}
