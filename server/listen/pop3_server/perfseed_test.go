//go:build perfseed

package pop3_server

import (
	"github.com/Jinnrry/pmail/tools/perfseed"
	"testing"
)

func BenchmarkPMailPOP3STAT(b *testing.B) {
	owner := perfseed.Seed(b)
	b.ReportAllocs()
	for b.Loop() {
		d, err := openMaildrop(owner, 0)
		if err != nil {
			b.Fatal(err)
		}
		n, _, err := d.stat()
		d.close()
		if err != nil || n != perfseed.Inbox {
			b.Fatalf("count=%d %v", n, err)
		}
	}
}
