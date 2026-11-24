package group

import (
	"testing"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/del_email"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/utils/context"
	"xorm.io/xorm"
)

func TestMoveOutOfTrashToCustomFolder(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	old := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = old; e.Close() })
	if err := db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	a, b := models.User{Account: "move-owner"}, models.User{Account: "move-other"}
	if _, err := e.Insert(&a, &b); err != nil {
		t.Fatal(err)
	}
	folder, foreign := models.Group{UserId: a.ID, Name: "Saved"}, models.Group{UserId: b.ID, Name: "Private"}
	msg := models.Email{Subject: "keep this mail"}
	if _, err := e.Insert(&msg, &folder, &foreign); err != nil {
		t.Fatal(err)
	}
	stamp, prevStatus, prevGroup := time.Now().Add(-48*time.Hour).UnixMilli(), consts.EmailStatusWait, models.INBOX
	link := models.UserEmail{UserID: a.ID, EmailID: msg.Id, Status: consts.EmailStatusDel, DeletedAt: &stamp, PrevStatus: &prevStatus, PrevGroupID: &prevGroup, IMAPDeleted: true}
	otherLink := models.UserEmail{UserID: b.ID, EmailID: msg.Id}
	if _, err := e.Insert(&link, &otherLink); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.IMAPMailboxUID{BoxID: 1, LinkID: link.ID, UID: 9}); err != nil {
		t.Fatal(err)
	}
	ctx := &context.Context{Context: t.Context(), UserID: a.ID}
	if MoveMailToGroup(ctx, []int{msg.Id}, foreign.ID) {
		t.Fatal("accepted someone else's folder")
	}
	if !MoveMailToGroup(ctx, []int{msg.Id}, folder.ID) {
		t.Fatal("move out of Trash failed")
	}
	var got models.UserEmail
	if ok, err := e.ID(link.ID).Get(&got); err != nil || !ok || got.Status != consts.EmailStatusWait || got.GroupId != folder.ID || got.DeletedAt != nil || got.PrevStatus != nil || got.PrevGroupID != nil || got.IMAPDeleted {
		t.Fatalf("moved link has stale Trash state: %+v %v", got, err)
	}
	if ok, err := e.Where("link_id=?", link.ID).Exist(&models.IMAPMailboxUID{}); err != nil || ok {
		t.Fatalf("stale Trash UID present: %v/%v", ok, err)
	}
	if ok, err := e.ID(otherLink.ID).Where("group_id=0 AND status=0").Exist(&models.UserEmail{}); err != nil || !ok {
		t.Fatalf("affected another owner's copy: %v/%v", ok, err)
	}
	if err := del_email.SetRetention(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if n, err := del_email.PurgeExpired(t.Context(), time.Now()); err != nil || n != 0 {
		t.Fatalf("moved mail eligible for expiry: %d/%v", n, err)
	}
	// Sent mail moved into a custom folder keeps its status and stays visible.
	sentMsg := models.Email{Subject: "sent then filed", Type: consts.EmailTypeSend}
	if _, err := e.Insert(&sentMsg); err != nil {
		t.Fatal(err)
	}
	sentLink := models.UserEmail{UserID: a.ID, EmailID: sentMsg.Id, Status: consts.EmailStatusSent}
	if _, err := e.Insert(&sentLink); err != nil {
		t.Fatal(err)
	}
	if !MoveMailToGroup(ctx, []int{sentMsg.Id}, folder.ID) {
		t.Fatal("move sent mail failed")
	}
	rows, _, err := list.WebPage(ctx, dto.SearchTag{GroupId: folder.ID, Status: -1, Type: -1}, "", list.MailCursor{}, 0, 10, true)
	if err != nil || len(rows) != 2 {
		t.Fatalf("mail invisible in target group: %d/%v", len(rows), err)
	}
}

func TestGroupNameLengthIsValidated(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	old := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = old; e.Close() })
	if err := db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	ctx := &context.Context{Context: t.Context(), UserID: 1}
	if _, err := CreateGroup(ctx, "这是一个超过十个字符的文件夹名", 0); err != ErrInvalidName {
		t.Fatalf("long name: %v", err)
	}
	if _, err := CreateGroup(ctx, "十个字符的文件夹名称", 0); err != nil {
		t.Fatalf("10 runes rejected: %v", err)
	}
	if err := Rename(ctx, "十个字符的文件夹名称", "elevenchars"); err != ErrInvalidName {
		t.Fatalf("long rename: %v", err)
	}
}
