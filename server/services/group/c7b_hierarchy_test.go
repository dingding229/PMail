package group

import (
	"fmt"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	"github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

func TestC7bGroupOwnerDepthAndCycle(t *testing.T) {
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
	own := &context.Context{UserID: 7}
	other := &context.Context{UserID: 8}
	foreign, err := CreateGroup(other, "foreign", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateGroup(own, "notmine", foreign.ID); err == nil || ValidMoveTarget(own, foreign.ID) {
		t.Fatal("foreign parent or destination accepted")
	}
	var root, parent int
	for i := 0; i < maxGroupDepth; i++ {
		row, err := CreateGroup(own, fmt.Sprintf("box-%d", i), parent)
		if err != nil {
			t.Fatal(err)
		}
		parent = row.ID
		if i == 0 {
			root = row.ID
		}
	}
	if _, err := CreateGroup(own, "too-deep", parent); err == nil {
		t.Fatal("deep nesting accepted")
	}
	if _, err := e.ID(root).Cols("parent_id").Update(&models.Group{ParentId: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateGroup(own, "cycle", root); err == nil {
		t.Fatal("cyclic parent accepted")
	}
}
