//go:build perfseed

package list_test

import (
	stdcontext "context"
	"testing"

	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/services/detail"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/tools/perfseed"
	"github.com/Jinnrry/pmail/utils/context"
)

func BenchmarkPMailSQLite(b *testing.B) {
	owner := perfseed.Seed(b)
	ctx := &context.Context{Context: stdcontext.Background(), UserID: owner.ID}
	tag := dto.SearchTag{Type: 0, Status: -1, GroupId: 0}
	for _, q := range []struct {
		name, key string
		count     int64
	}{{"List", "", perfseed.Inbox}, {"Search", "needle", perfseed.Inbox / 100}} {
		b.Run(q.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				rows, total := list.GetEmailList(ctx, tag, q.key, false, 0, 25)
				if len(rows) != 25 || total != q.count {
					b.Fatalf("rows=%d total=%d", len(rows), total)
				}
			}
		})
	}
	b.Run("DetailWithAttachment", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			row, err := detail.GetEmailDetail(ctx, perfseed.Total, false)
			if err != nil || row.Id != perfseed.Total {
				b.Fatalf("%v", err)
			}
		}
	})
}
