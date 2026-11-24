package imap_server

import (
	"testing"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/del_email"
	"github.com/Jinnrry/pmail/services/group"
	"github.com/emersion/go-imap/v2"
)

func TestIMAPTrashTransferAndRestore(t *testing.T) {
	for _, tc := range []struct {
		name string
		move bool
	}{{"MOVE", true}, {"COPY", false}} {
		t.Run(tc.name, func(t *testing.T) {
			e, s, _ := imapFixture(t)
			mail := models.Email{Size: 13}
			if _, err := e.Insert(&mail); err != nil {
				t.Fatal(err)
			}
			g, err := group.CreateGroup(s.ctx, "archive", 0)
			if err != nil {
				t.Fatal(err)
			}
			original := models.UserEmail{EmailID: mail.Id, UserID: s.ctx.UserID, Status: consts.EmailStatusWait, GroupId: g.ID}
			if _, err := e.Insert(&original); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Select("archive", nil); err != nil {
				t.Fatal(err)
			}
			before, err := mailboxSnapshot(s.ctx, "archive")
			if err != nil || len(before.items) != 1 {
				t.Fatalf("before: %+v %v", before, err)
			}
			if _, _, err := s.transfer(imap.SeqSetNum(1), "Deleted Messages", tc.move); err != nil {
				t.Fatal(err)
			}
			trash, err := mailboxSnapshot(s.ctx, "Deleted Messages")
			if err != nil || len(trash.items) != 1 {
				t.Fatalf("Trash: %+v %v", trash, err)
			}
			link := trash.items[0].row
			if link.DeletedAt == nil || link.PrevStatus == nil || *link.PrevStatus != original.Status || link.PrevGroupID == nil || *link.PrevGroupID != g.ID {
				t.Fatalf("lost origin metadata: %+v", link)
			}
			if n, err := del_email.Restore(s.ctx, []int{mail.Id}); err != nil || n != 1 {
				t.Fatalf("restore: %d %v", n, err)
			}
			wantCount := 1
			if !tc.move {
				wantCount++
			}
			after, err := mailboxSnapshot(s.ctx, "archive")
			if err != nil || len(after.items) != wantCount || after.validity != before.validity || after.items[wantCount-1].uid < before.next {
				t.Fatalf("restored item did not receive a fresh UID: before=%+v after=%+v error=%v", before, after, err)
			}
			if !tc.move && after.items[0].uid != before.items[0].uid {
				t.Fatal("COPY changed the surviving source UID")
			}
			empty, err := mailboxSnapshot(s.ctx, "Deleted Messages")
			if err != nil || len(empty.items) != 0 || empty.next != trash.next {
				t.Fatalf("restored item still in Trash or UIDNEXT regressed: %+v %v", empty, err)
			}
		})
	}
}

func TestWebTrashRoundTripBeforeIMAPSyncInvalidatesOldUID(t *testing.T) {
	e, s, _ := imapFixture(t)
	mail := models.Email{Size: 8}
	if _, err := e.Insert(&mail); err != nil {
		t.Fatal(err)
	}
	addFixtureLink(t, e, s.ctx.UserID, mail.Id, 0, false)
	before, err := mailboxSnapshot(s.ctx, "INBOX")
	if err != nil || len(before.items) != 1 {
		t.Fatalf("old inbox: %+v %v", before, err)
	}
	if err := del_email.DelEmail(s.ctx, []int{mail.Id}, false); err != nil {
		t.Fatal(err)
	}
	if n, err := del_email.Restore(s.ctx, []int{mail.Id}); err != nil || n != 1 {
		t.Fatalf("restore: %d %v", n, err)
	}
	after, err := mailboxSnapshot(s.ctx, "INBOX")
	if err != nil || len(after.items) != 1 || after.items[0].uid < before.next || after.validity != before.validity {
		t.Fatalf("returned to stale UID: old=%+v new=%+v err=%v", before, after, err)
	}
}
