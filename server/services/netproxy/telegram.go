package netproxy

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/seal"
)

const TelegramField = "telegram"

func TelegramDB(row models.TelegramConfig) (Stored, error) {
	var out Stored
	if row.ProxyFields != "" {
		if json.Unmarshal([]byte(row.ProxyFields), &out) != nil {
			return out, ErrInvalid
		}
		return out, nil
	}
	legacy, err := seal.Decode("pmail/telegram/v1", "proxy", row.Proxy)
	if err != nil {
		return out, err
	}
	p, err := ParseLegacy(legacy)
	if err != nil {
		return out, err
	}
	return Apply(out, p, false, TelegramField)
}

func TelegramProxy(row models.TelegramConfig) (*url.URL, *View, string, error) {
	if raw := strings.TrimSpace(os.Getenv("PMAIL_TELEGRAM_PROXY")); raw != "" {
		p, err := ParseLegacy(raw)
		if err != nil || p == nil {
			return nil, nil, "env", ErrInvalid
		}
		u, err := p.Fields.URL(*p.Password)
		return u, &View{p.Fields, *p.Password != ""}, "env", err
	}
	p, err := TelegramDB(row)
	if err != nil {
		return nil, nil, "db", err
	}
	if p.Type == "" {
		return nil, nil, "none", nil
	}
	u, err := p.URL(TelegramField)
	return u, p.View(), "db", err
}

func SystemTelegram(ctx context.Context) (*url.URL, error) {
	var row models.TelegramConfig
	if _, err := db.Instance.Context(ctx).ID(1).Get(&row); err != nil {
		return nil, err
	}
	u, _, _, err := TelegramProxy(row)
	return u, err
}
