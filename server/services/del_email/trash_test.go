package del_email

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/testschema"
	"github.com/Jinnrry/pmail/utils/context"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func trashFixture(t *testing.T) (*xorm.Engine, *context.Context, *context.Context) {
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
	u, v := models.User{Account: "trash-owner"}, models.User{Account: "trash-other"}
	if _, err := e.Insert(&u, &v); err != nil {
		t.Fatal(err)
	}
	return e, &context.Context{Context: t.Context(), UserID: u.ID}, &context.Context{Context: t.Context(), UserID: v.ID}
}

func trashMail(t *testing.T, e *xorm.Engine, owner int, typ, status int8, group int, at *int64, size int) models.UserEmail {
	t.Helper()
	row := models.Email{Type: typ, Size: size}
	if _, err := e.Insert(&row); err != nil {
		t.Fatal(err)
	}
	link := models.UserEmail{EmailID: row.Id, UserID: owner, Status: status, GroupId: group, DeletedAt: at}
	if _, err := e.Insert(&link); err != nil {
		t.Fatal(err)
	}
	return link
}

func readLink(t *testing.T, e *xorm.Engine, id int) models.UserEmail {
	t.Helper()
	var row models.UserEmail
	if ok, err := e.ID(id).Get(&row); err != nil || !ok {
		t.Fatalf("link %d: %v, %v", id, ok, err)
	}
	return row
}

func TestTrashRestoreAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		name                                        string
		typ, priorStatus, wantStatus                int8
		legacy, hasGroup, deleteGroup, foreignGroup bool
	}{
		{name: "original group", hasGroup: true},
		{name: "deleted group returns to inbox", hasGroup: true, deleteGroup: true, priorStatus: consts.EmailStatusJunk},
		{name: "foreign group returns to inbox", hasGroup: true, foreignGroup: true, priorStatus: consts.EmailStatusJunk},
		{name: "sent", typ: consts.EmailTypeSend, priorStatus: consts.EmailStatusSent, wantStatus: consts.EmailStatusSent},
		{name: "legacy sent", typ: consts.EmailTypeSend, legacy: true, wantStatus: consts.EmailStatusSent},
		{name: "legacy received", legacy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, owner, other := trashFixture(t)
			var folder models.Group
			if tc.hasGroup {
				folder = models.Group{UserId: owner.UserID, Name: "test"}
				if tc.foreignGroup {
					folder.UserId = other.UserID
				}
				if _, err := e.Insert(&folder); err != nil {
					t.Fatal(err)
				}
			}
			status := tc.priorStatus
			var when *int64
			if tc.legacy {
				status = consts.EmailStatusDel
				ts := int64(1)
				when = &ts
			}
			link := trashMail(t, e, owner.UserID, tc.typ, status, folder.ID, when, 12)
			if err := DelEmail(owner, []int{link.EmailID}, false); err != nil {
				t.Fatal(err)
			}
			first := readLink(t, e, link.ID)
			if first.Status != consts.EmailStatusDel || first.GroupId != 0 || first.DeletedAt == nil {
				t.Fatalf("Trash row: %+v", first)
			}
			if !tc.legacy && (first.PrevStatus == nil || *first.PrevStatus != tc.priorStatus || first.PrevGroupID == nil || *first.PrevGroupID != folder.ID) {
				t.Fatalf("previous placement was not recorded: %+v", first)
			}
			// An old deletion time deterministically distinguishes recording from overwriting.
			if _, err := e.ID(link.ID).Cols("deleted_at").Update(&models.UserEmail{DeletedAt: new(int64)}); err != nil {
				t.Fatal(err)
			}
			if err := DelEmail(owner, []int{link.EmailID}, false); err != nil {
				t.Fatal(err)
			}
			twice := readLink(t, e, link.ID)
			if twice.DeletedAt == nil || *twice.DeletedAt != 0 || !equalPointer(first.PrevStatus, twice.PrevStatus) || !equalPointer(first.PrevGroupID, twice.PrevGroupID) {
				t.Fatalf("repeat delete overwrote metadata: %+v", twice)
			}
			if n, err := Restore(other, []int{link.EmailID}); n != 0 || err != nil {
				t.Fatalf("foreign restore n=%d err=%v", n, err)
			}
			if tc.deleteGroup {
				if _, err := e.ID(folder.ID).Delete(&models.Group{}); err != nil {
					t.Fatal(err)
				}
			}
			if n, err := Restore(owner, []int{link.EmailID, link.EmailID}); n != 1 || err != nil {
				t.Fatalf("restore n=%d err=%v", n, err)
			}
			wantGroup := folder.ID
			if tc.deleteGroup || tc.foreignGroup {
				wantGroup = 0
			}
			live := readLink(t, e, link.ID)
			if live.Status != tc.wantStatus || live.GroupId != wantGroup || live.DeletedAt != nil || live.PrevStatus != nil || live.PrevGroupID != nil || live.IMAPDeleted {
				t.Fatalf("restored row: %+v", live)
			}
		})
	}
}

func equalPointer[T comparable](a, b *T) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func TestEmptyTrashBatchesKeepsSharedContentAndChargesOnlyDeletedLinks(t *testing.T) {
	e, a, b := trashFixture(t)
	stamp := time.Now().UnixMilli()
	link := trashMail(t, e, a.UserID, 0, consts.EmailStatusDel, 0, &stamp, 90)
	live := models.UserEmail{UserID: a.UserID, EmailID: link.EmailID, Status: consts.EmailStatusWait}
	shared := models.UserEmail{UserID: b.UserID, EmailID: link.EmailID, Status: consts.EmailStatusDel, DeletedAt: &stamp}
	if _, err := e.Insert(&live, &shared); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.MailAttachmentChunk{EmailID: link.EmailID, Data: []byte("shared bytes")}); err != nil {
		t.Fatal(err)
	}
	var duplicates []models.UserEmail
	for range BatchSize {
		duplicates = append(duplicates, models.UserEmail{UserID: a.UserID, EmailID: link.EmailID, Status: consts.EmailStatusDel, DeletedAt: &stamp})
	}
	if _, err := e.Insert(&duplicates); err != nil {
		t.Fatal(err)
	}
	if n, err := Empty(t.Context(), a.UserID); n != BatchSize+1 || err != nil {
		t.Fatalf("first empty=%d %v", n, err)
	}
	if n, err := Empty(t.Context(), a.UserID); n != 0 || err != nil {
		t.Fatalf("repeat empty=%d %v", n, err)
	}
	for _, r := range []models.UserEmail{live, shared} {
		readLink(t, e, r.ID)
	}
	if n, err := Empty(t.Context(), b.UserID); n != 1 || err != nil {
		t.Fatalf("second owner empty=%d %v", n, err)
	}
	if ok, err := e.Where("email_id=?", link.EmailID).Exist(&models.MailAttachmentChunk{}); !ok || err != nil {
		t.Fatalf("shared attachment removed %v %v", ok, err)
	}
	usage, err := quota.Usage([]int{a.UserID, b.UserID})
	if err != nil || usage[a.UserID] != 90 || usage[b.UserID] != 0 {
		t.Fatalf("remaining usage=%v %v", usage, err)
	}
	if err := DelEmail(a, []int{link.EmailID}, true); err != nil {
		t.Fatal(err)
	}
	if ok, err := e.ID(link.EmailID).Exist(&models.Email{}); ok || err != nil {
		t.Fatalf("orphan left %v %v", ok, err)
	}
	if ok, err := e.Where("email_id=?", link.EmailID).Exist(&models.MailAttachmentChunk{}); ok || err != nil {
		t.Fatalf("orphan attachment left %v %v", ok, err)
	}
}

func TestPurgeExpiredBoundaryAndConcurrentCleanup(t *testing.T) {
	e, a, _ := trashFixture(t)
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	day := int64(30) * DayMillis
	cutoff := now.UnixMilli() - day
	before, after := cutoff-1, cutoff+1
	old := trashMail(t, e, a.UserID, 0, consts.EmailStatusDel, 0, &before, 5)
	exact := trashMail(t, e, a.UserID, 0, consts.EmailStatusDel, 0, &cutoff, 7)
	future := trashMail(t, e, a.UserID, 0, consts.EmailStatusDel, 0, &after, 11)
	live := trashMail(t, e, a.UserID, 0, 0, 0, &before, 13)
	if days, err := Retention(t.Context()); days != 30 || err != nil {
		t.Fatalf("default retention=%d %v", days, err)
	}
	for _, days := range []int{-1, 3651} {
		if err := SetRetention(t.Context(), days); !errors.Is(err, ErrRetention) {
			t.Fatalf("retention %d: %v", days, err)
		}
	}
	if err := SetRetention(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	if n, err := PurgeExpired(t.Context(), now); n != 0 || err != nil {
		t.Fatalf("disabled cleanup: %d %v", n, err)
	}
	readLink(t, e, old.ID)
	if err := SetRetention(t.Context(), 30); err != nil {
		t.Fatal(err)
	}
	var counts [2]int64
	var errs [2]error
	var wg sync.WaitGroup
	for i := range counts {
		wg.Go(func() { counts[i], errs[i] = PurgeExpired(t.Context(), now) })
	}
	wg.Wait()
	if counts[0]+counts[1] != 1 || errs[0] != nil || errs[1] != nil {
		t.Fatalf("concurrent sweep counts=%v errors=%v", counts, errs)
	}
	if ok, err := e.ID(old.ID).Exist(&models.UserEmail{}); ok || err != nil {
		t.Fatalf("expired copy: %t %v", ok, err)
	}
	for _, r := range []models.UserEmail{exact, future, live} {
		readLink(t, e, r.ID)
	}
	usage, err := quota.Usage([]int{a.UserID})
	if err != nil || usage[a.UserID] != 31 {
		t.Fatalf("quota after sweep %v %v", usage, err)
	}
	if n, err := PurgeExpired(t.Context(), now.Add(time.Millisecond)); n != 1 || err != nil {
		t.Fatalf("now strictly older: %d %v", n, err)
	}
}
