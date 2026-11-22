package group

import (
	"fmt"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	"github.com/Jinnrry/pmail/utils/context"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestGetGroupStatus(t *testing.T) {
	engine, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	engine.SetMaxOpenConns(1)
	old := db.Instance
	db.Instance = engine
	t.Cleanup(func() { db.Instance = old; _ = engine.Close() })
	if err := testschema.Sync(engine); err != nil {
		t.Fatal(err)
	}

	rows := []models.UserEmail{
		{UserID: 1, EmailID: 1},
		{UserID: 1, EmailID: 2, IsRead: 1},
		{UserID: 1, EmailID: 3, Status: 3},
		{UserID: 2, EmailID: 1},
	}
	for i := range rows {
		if _, err := engine.Insert(&rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	params := []string{"MESSAGES", "UIDNEXT", "UIDVALIDITY", "UNSEEN"}
	text, status := GetGroupStatus(&context.Context{UserID: 1}, "INBOX", params)
	want := map[string]int{"MESSAGES": 2, "UIDNEXT": rows[1].ID + 1, "UIDVALIDITY": models.INBOX, "UNSEEN": 1}
	for _, name := range params {
		if got := status[name]; got != want[name] {
			t.Errorf("%s = %d, want %d", name, got, want[name])
		}
	}
	wantText := fmt.Sprintf("(MESSAGES 2 UIDNEXT %d UIDVALIDITY %d UNSEEN 1)", want["UIDNEXT"], models.INBOX)
	if text != wantText {
		t.Errorf("status = %q, want %q", text, wantText)
	}
}
