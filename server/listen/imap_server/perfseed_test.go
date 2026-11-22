//go:build perfseed

package imap_server

import (
	stdcontext "context"
	"github.com/Jinnrry/pmail/tools/perfseed"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"testing"
)

func BenchmarkPMailIMAPSearch(b *testing.B) {
	owner := perfseed.Seed(b)
	s := &serverSession{ctx: &context.Context{Context: stdcontext.Background(), UserID: owner.ID}}
	if _, err := s.Select("INBOX", nil); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		r, err := s.Search(imapserver.NumKindUID, &imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: "Subject", Value: "needle"}}}, nil)
		if err != nil || r.Count != perfseed.Inbox/100 {
			b.Fatalf("%+v %v", r, err)
		}
	}
}
