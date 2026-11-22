//go:build perfseed

package list

import (
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/tools/perfseed"
	"github.com/Jinnrry/pmail/utils/context"
)

func TestPMailSQLiteQueryPlans(t *testing.T) {
	owner := perfseed.Seed(t)
	ctx := &context.Context{UserID: owner.ID}
	for _, tcase := range []struct {
		name string
		tag  dto.SearchTag
		key  string
	}{
		{"Inbox", dto.SearchTag{Type: 0, Status: -1, GroupId: 0}, ""},
		{"Search", dto.SearchTag{Type: 0, Status: -1, GroupId: 0}, "needle"},
		{"Starred", dto.SearchTag{Type: -1, Status: -1, GroupId: -1, Starred: true}, ""},
	} {
		query, params, _, _, err := webPageSQL(ctx, tcase.tag, tcase.key, MailCursor{}, 0, 25)
		if err != nil {
			t.Fatal(err)
		}
		var plan []struct {
			Detail string `xorm:"detail"`
		}
		if err = db.Instance.SQL("EXPLAIN QUERY PLAN "+query, params...).Find(&plan); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s:", tcase.name)
		for _, step := range plan {
			t.Log(step.Detail)
		}
	}
}
