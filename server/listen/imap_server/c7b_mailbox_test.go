package imap_server

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/group"
	"github.com/Jinnrry/pmail/testschema"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/emersion/go-imap/v2"
	"xorm.io/xorm"
)

func imapFixture(t *testing.T) (*xorm.Engine, *serverSession, int) {
	t.Helper()
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
	owner := models.User{Account: "reader", Password: "fixture"}
	outside := models.User{Account: "other", Password: "fixture"}
	if _, err = e.Insert(&owner, &outside); err != nil {
		t.Fatal(err)
	}
	return e, &serverSession{ctx: &context.Context{UserID: owner.ID}}, outside.ID
}
func addFixtureLink(t *testing.T, e *xorm.Engine, owner, emailID, group int, deleted bool) int {
	t.Helper()
	r := models.UserEmail{UserID: owner, EmailID: emailID, GroupId: group, IMAPDeleted: deleted}
	if _, err := e.Insert(&r); err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func TestC7bIMAPStoreExpungeScopesLinksAndUIDWatermark(t *testing.T) {
	e, s, other := imapFixture(t)
	addFixtureLink(t, e, s.ctx.UserID, 101, 0, false)
	target := addFixtureLink(t, e, s.ctx.UserID, 102, 0, false)
	outside := addFixtureLink(t, e, other, 102, 0, true)
	g, err := group.CreateGroup(s.ctx, "side", 0)
	if err != nil {
		t.Fatal(err)
	}
	copyID := addFixtureLink(t, e, s.ctx.UserID, 102, g.ID, true)
	if _, err := s.Select("missing", nil); err == nil {
		t.Fatal("SELECT invented a mailbox")
	}
	init, err := s.Select("INBOX", nil)
	if err != nil {
		t.Fatal(err)
	}
	flags := &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}
	if err := s.Store(nil, imap.SeqSetNum(2), flags, nil); err != nil {
		t.Fatal(err)
	}
	var seen models.UserEmail
	if ok, err := e.ID(target).Get(&seen); err != nil || !ok || !seen.IMAPDeleted {
		t.Fatalf("STORE did not only mark link: %+v %v", seen, err)
	}
	var emitted []uint32
	set := imap.UIDSet{{Start: 1, Stop: math.MaxUint32}}
	if err := s.expunge(set, func(n uint32) error { emitted = append(emitted, n); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(emitted, []uint32{2}) {
		t.Fatalf("expunge responses: %v", emitted)
	}
	state, err := mailboxSnapshot(s.ctx, "INBOX")
	if err != nil || len(state.items) != 1 || state.next != init.UIDNext || state.validity != init.UIDValidity {
		t.Fatalf("watermark rewound: %+v %v", state, err)
	}
	for _, id := range []int{outside, copyID} {
		if ok, err := e.ID(id).Exist(&models.UserEmail{}); err != nil || !ok {
			t.Fatal("EXPUNGE touched another owner or mailbox", id, err)
		}
	}
	if _, err := s.Append("INBOX", nil, nil); err == nil {
		t.Fatal("APPEND claimed success")
	}
}

func TestC7bIMAPSparseCopyMoveAndOlderLinkArrival(t *testing.T) {
	e, s, _ := imapFixture(t)
	for _, id := range []int{21, 22, 23, 24} {
		addFixtureLink(t, e, s.ctx.UserID, id, 0, false)
	}
	archive, err := group.CreateGroup(s.ctx, "archive", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Select("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	copied, _, err := s.transfer(imap.SeqSetNum(1, 3), "archive", false)
	if err != nil || copied == nil || copied.SourceUIDs.String() != "1,3" {
		t.Fatalf("COPY sparse set: %+v %v", copied, err)
	}
	before, err := mailboxSnapshot(s.ctx, "archive")
	if err != nil || len(before.items) != 2 {
		t.Fatal(before, err)
	}
	_, removed, err := s.transfer(imap.SeqSetNum(2, 4), "archive", true)
	if err != nil || len(removed) != 2 {
		t.Fatalf("MOVE sparse set: %v %v", removed, err)
	}
	s.announced = nil
	after, err := mailboxSnapshot(s.ctx, "archive")
	if err != nil || len(after.items) != 4 || after.next <= before.next {
		t.Fatal(after, err)
	}
	source, err := mailboxSnapshot(s.ctx, "INBOX")
	if err != nil || len(source.items) != 2 {
		t.Fatal(source, err)
	}
	// Web can move an older DB primary key into an already populated folder.
	if _, err := e.ID(source.items[0].row.ID).Cols("group_id").Update(&models.UserEmail{GroupId: archive.ID}); err != nil {
		t.Fatal(err)
	}
	newer, err := mailboxSnapshot(s.ctx, "archive")
	if err != nil || len(newer.items) != 5 || newer.items[4].uid != after.next {
		t.Fatal(newer, err)
	}
}

func TestC7bIDLECoalescesNotices(t *testing.T) {
	c, release, err := listenIdle(808)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	other, done, err := listenIdle(809)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	for range 100 {
		notifyUser(808)
	}
	if len(c) != 1 || len(other) != 0 {
		t.Fatal(errors.New("IDLE queue not coalesced or leaked across owners"))
	}
}

func TestIMAPRejectsOutOfRangePersistedUID(t *testing.T) {
	e, s, _ := imapFixture(t)
	addFixtureLink(t, e, s.ctx.UserID, 101, 0, false)
	if _, err := s.Select("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Table(&models.IMAPMailboxUID{}).Where("uid=?", 1).Update(map[string]any{"uid": int64(math.MaxUint32) + 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Select("INBOX", nil); err == nil {
		t.Fatal("corrupt UID accepted by SELECT")
	}
	if _, err := s.searchSnapshot(); err == nil {
		t.Fatal("corrupt UID accepted by SEARCH")
	}
}
