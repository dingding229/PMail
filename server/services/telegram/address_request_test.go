package telegram

import (
	"strings"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
)

func TestAddressRequestTelegramPushOnlyToCurrentlyBoundAdmins(t *testing.T) {
	w, e, bot, owner, other, c := callbackFixture(t)
	config.Instance.Domain = "approval-telegram.test"
	if err := domain.Initialize(config.Instance); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ID(owner).Cols("is_admin").Update(&models.User{IsAdmin: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.TelegramBinding{UserID: other, ChatID: testChatID + 1, Bot: c.Generation, Identity: "other-binding"}); err != nil {
		t.Fatal(err)
	}
	create := func(name string) address.View {
		t.Helper()
		v, err := address.Create(t.Context(), other, domain.Default().ID, name, true, false)
		if err != nil {
			t.Fatal(err)
		}
		NotifyAddressRequest(v.ID)
		return v
	}
	first := create("requested")
	NotifyAddressRequest(first.ID)
	var rows []models.TelegramPush
	if err := e.Find(&rows); err != nil || len(rows) != 1 || rows[0].UserID != owner || rows[0].EmailID != -first.ID {
		t.Fatal("admin notice wasn't deduplicated or was queued for a non-admin")
	}
	if !w.queueOnce(t.Context()) {
		t.Fatal("admin notice did not run")
	}
	_, sent := bot.snapshot()
	if len(sent) != 1 || sent[0].ChatID != testChatID || sent[0].Markup != nil || !strings.Contains(sent[0].Text, first.Address) {
		t.Fatal("incorrect Telegram request notification")
	}
	create("not-leaked")
	if _, err := e.ID(owner).Cols("is_admin").Update(&models.User{IsAdmin: 0}); err != nil {
		t.Fatal(err)
	}
	resetRate()
	if !w.queueOnce(t.Context()) {
		t.Fatal("admin notice not discarded after demotion")
	}
	_, sent = bot.snapshot()
	if len(sent) != 1 {
		t.Fatal("demoted administrator received an admin notification")
	}
}
