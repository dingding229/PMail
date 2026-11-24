package rule

import (
	"strconv"
	"testing"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	"github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

func TestC7bRuleMoveDoesNotChangeSharedEmailType(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	old := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = old; _ = e.Close() })
	if err := testschema.Sync(e); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.User{ID: 7, Account: "move-owner"}, &models.User{ID: 8, Account: "move-other"}); err != nil {
		t.Fatal(err)
	}
	msg := models.Email{Type: consts.EmailTypeReceive}
	if _, err := e.Insert(&msg); err != nil {
		t.Fatal(err)
	}
	first := models.UserEmail{UserID: 7, EmailID: msg.Id}
	second := models.UserEmail{UserID: 8, EmailID: msg.Id}
	if _, err := e.Insert(&first, &second); err != nil {
		t.Fatal(err)
	}
	doMove(&context.Context{}, &dto.Rule{UserId: 7, Params: strconv.Itoa(models.Sent)}, &parsemail.Email{MessageId: int64(msg.Id)}, &models.User{ID: 7})
	var a, b models.UserEmail
	if ok, err := e.ID(first.ID).Get(&a); err != nil || !ok || a.Status != consts.EmailStatusSent {
		t.Fatal(a, err)
	}
	if ok, err := e.ID(second.ID).Get(&b); err != nil || !ok || b.Status != consts.EmailStatusWait {
		t.Fatal(b, err)
	}
	if ok, err := e.ID(msg.Id).Get(&msg); err != nil || !ok || msg.Type != consts.EmailTypeReceive {
		t.Fatal(msg, err)
	}
}
