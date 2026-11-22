package controllers

import (
	"errors"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	"github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

func TestC7bRuleCountLimitIsPerOwner(t *testing.T) {
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
	a := models.User{Account: "test-a", Password: "fixture"}
	b := models.User{Account: "test-b", Password: "fixture"}
	if _, err = e.Insert(&a, &b); err != nil {
		t.Fatal(err)
	}
	actor := &context.Context{UserID: a.ID}
	for range maxUserRules {
		if err := save(actor, &models.Rule{Name: "fixture"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := save(actor, &models.Rule{Name: "too many"}); !errors.Is(err, errTooManyRules) {
		t.Fatal(err)
	}
	if err := save(&context.Context{UserID: b.ID}, &models.Rule{Name: "still allowed"}); err != nil {
		t.Fatal(err)
	}
}
