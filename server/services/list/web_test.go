package list_test

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/testschema"
	pctx "github.com/Jinnrry/pmail/utils/context"
	"github.com/emersion/go-imap/v2"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func listDB(t *testing.T) *xorm.Engine {
	t.Helper()
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	prev := db.Instance
	db.Instance = e
	t.Cleanup(func() { db.Instance = prev; e.Close() })
	if err = testschema.Sync(e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestProjectedListCursorWithCopyAndNewMail(t *testing.T) {
	e := listDB(t)
	ctx := &pctx.Context{Context: context.Background(), UserID: 1}
	content := strings.Repeat("界", 1000) + " search-match"
	for _, id := range []int{11, 12, 13} {
		row := models.Email{Id: id, Text: sql.NullString{String: content, Valid: true}, Attachments: `[{"Filename":"clip.txt","ContentType":"text/plain","Content":"YWJj"}]`}
		mailstore.Prepare(&row)
		if err := db.Transaction(e, func(tx *xorm.Session) error {
			if _, err := tx.Insert(&row); err != nil {
				return err
			}
			return mailstore.WriteParts(tx, &row, false)
		}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if _, err := e.Insert(&models.UserEmail{UserID: 1, EmailID: id, IsStarred: true}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := e.Insert(&models.UserEmail{UserID: 2, EmailID: id}); err != nil {
			t.Fatal(err)
		}
	}
	tag := dto.SearchTag{Type: 0, Status: -1, GroupId: 0}
	page, total, err := list.WebPage(ctx, tag, "search-match", list.MailCursor{}, 0, 1, true)
	if err != nil || total != 6 || len(page) != 1 || page[0].Id != 13 {
		t.Fatalf("page=%v total=%d %v", page, total, err)
	}
	first := page[0]
	if first.Attachments != "" || first.Text.Valid || first.Html.Valid || len(first.AttachmentInfo) != 1 || first.AttachmentInfo[0].Size != 3 || len(first.ListSnippet) != 1023 {
		t.Fatalf("invalid projection: %+v", first)
	}
	c, err := list.DecodeCursor(list.EncodeCursor(first))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Insert(&models.Email{Id: 20}, &models.UserEmail{EmailID: 20, UserID: 1}); err != nil {
		t.Fatal(err)
	}
	next, total, err := list.WebPage(ctx, tag, "", c, 0, 2, false)
	if err != nil || total != 0 || len(next) != 2 || next[0].Id != 13 || next[0].UeId == first.UeId || next[1].Id != 12 {
		t.Fatalf("keyset=%+v total=%d %v", next, total, err)
	}
	star, _, err := list.WebPage(ctx, dto.SearchTag{Starred: true, Type: -1, Status: -1, GroupId: -1}, "", list.MailCursor{}, 0, 25, true)
	if err != nil || len(star) != 3 {
		t.Fatalf("starred count=%d %v", len(star), err)
	}
}

func TestIMAPSearchLogicPatternAndQueryError(t *testing.T) {
	e := listDB(t)
	ctx := &pctx.Context{Context: context.Background()}
	fixtures := []models.Email{{Id: 1, Subject: "A 100%_ finish"}, {Id: 2, Subject: "A 100XX finish"}, {Id: 3, Subject: "Unicode Kilo"}}
	for i := range fixtures {
		if _, err := e.Insert(&fixtures[i]); err != nil {
			t.Fatal(err)
		}
	}
	rows := []*response.UserEmailUIDData{}
	for i := 1; i <= 3; i++ {
		rows = append(rows, &response.UserEmailUIDData{UserEmail: models.UserEmail{ID: i * 10, EmailID: i, IMAPDeleted: i == 2}, SerialNumber: i})
	}
	header := func(value string) imap.SearchCriteria {
		return imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: "Subject", Value: value}}}
	}
	c := header("%_")
	m, err := list.SearchRows(ctx, rows, &c)
	if err != nil || len(m) != 1 || m[0].ID != 10 {
		t.Fatalf("literal wildcard %v %v", m, err)
	}
	c = imap.SearchCriteria{Or: [][2]imap.SearchCriteria{{header("100"), header("kilo")}, {header("finish"), header("kilo")}}, Not: []imap.SearchCriteria{{Flag: []imap.Flag{imap.FlagDeleted}}}}
	m, err = list.SearchRows(ctx, rows, &c)
	if err != nil || len(m) != 2 || !reflect.DeepEqual([]int{m[0].ID, m[1].ID}, []int{10, 30}) {
		t.Fatalf("OR/NOT %v %v", m, err)
	}
	if _, err = e.Exec("DROP TABLE email"); err != nil {
		t.Fatal(err)
	}
	if m, err = list.SearchRows(ctx, rows, &c); err == nil || len(m) != 0 {
		t.Fatalf("query error returned %v %v", m, err)
	}
}

func TestDraftCursorContinuesAfterAnchorDeletion(t *testing.T) {
	e := listDB(t)
	ctx := &pctx.Context{Context: context.Background(), UserID: 3}
	date := time.Date(2026, 11, 23, 2, 38, 31, 0, time.FixedZone("fixture-zone", 8*3600))
	for i, d := range []time.Time{date.Add(time.Hour), date, date, date.Add(-time.Hour)} {
		row := models.Email{Id: i + 1, SendDate: d, Status: 4}
		if _, err := e.Insert(&row, &models.UserEmail{EmailID: row.Id, UserID: ctx.UserID, Status: 4}); err != nil {
			t.Fatal(err)
		}
	}
	tag := dto.SearchTag{Status: 4, GroupId: -1}
	got, _, err := list.WebPage(ctx, tag, "", list.MailCursor{}, 0, 2, false)
	if err != nil || len(got) != 2 || got[0].Id != 1 || got[1].Id != 3 {
		t.Fatalf("first page=%v %v", got, err)
	}
	c, err := list.DecodeCursor(list.EncodeCursor(got[1]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.ID(3).Delete(&models.Email{}); err != nil {
		t.Fatal(err)
	}
	got, _, err = list.WebPage(ctx, tag, "", c, 0, 2, false)
	if err != nil || len(got) != 2 || got[0].Id != 2 || got[1].Id != 4 {
		t.Fatalf("next page=%v %v", got, err)
	}
}
