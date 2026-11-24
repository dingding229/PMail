package telegram

import (
	"context"
	"html"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	pctx "github.com/Jinnrry/pmail/utils/context"
)

// Queue an ID, not mail or account data. Negative IDs are an independent
// deduplication namespace in the existing, binding-scoped push queue.
func NotifyAddressRequest(id int) {
	if id <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	c, err := getConfig(ctx)
	if err != nil || c.Token == "" {
		return
	}
	var bindings []models.TelegramBinding
	accounts := db.Instance.Quote("user")
	if err = db.Instance.Context(ctx).Where("bot_generation=?", c.Generation).Where("user_id IN (SELECT id FROM " + accounts + " WHERE disabled=0 AND is_admin=1)").Find(&bindings); err != nil {
		return
	}
	for _, b := range bindings {
		if ctx.Err() != nil {
			return
		}
		enqueuePush(ctx, b, -id)
	}
}

func (w *worker) queueAddressRequest(ctx context.Context, row models.TelegramPush, c models.TelegramConfig, b models.TelegramBinding) bool {
	return deliverPush(ctx, row, c, b, func() (*pushPayload, error) {
		current, err := getConfig(ctx)
		if err != nil {
			return nil, err
		}
		if current.Generation != c.Generation || current.Token == "" {
			return nil, nil
		}
		bound, ok, err := binding(ctx, current, row.UserID)
		if err != nil || !ok || bound.Identity != row.Binding {
			return nil, err
		}
		var owner models.User
		ok, err = db.Instance.Context(ctx).ID(row.UserID).Where("disabled=0 AND is_admin=1").Cols("lang").Get(&owner)
		if err != nil || !ok {
			return nil, err
		}
		request, ok, err := address.PendingAddress(ctx, -row.EmailID)
		if err != nil || !ok {
			return nil, err
		}
		locale := &pctx.Context{Context: ctx, Lang: owner.Lang}
		return &pushPayload{text: html.EscapeString(i18n.T(locale, "telegram_address_requested", request.Address))}, nil
	})
}
