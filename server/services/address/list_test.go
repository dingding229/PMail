package address

import (
	"testing"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/testschema"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestListByUsersKeepsOwnerOrderAndDomain(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	defer e.Close()
	old := db.Instance
	db.Instance = e
	defer func() { db.Instance = old }()
	if err = testschema.Sync(e); err != nil {
		t.Fatal(err)
	}
	for _, d := range []models.Domain{{ID: 1, Name: "example.org"}, {ID: 2, Name: "example.net"}} {
		if _, err = e.Insert(&d); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []models.MailboxAddress{{UserID: 1, DomainID: 1, LocalPart: "alias"}, {UserID: 2, DomainID: 1, LocalPart: "other"}, {UserID: 1, DomainID: 2, LocalPart: "primary", IsPrimary: true}} {
		if _, err = e.Insert(&a); err != nil {
			t.Fatal(err)
		}
	}
	byID, err := ListByUsers([]int{1, 3})
	if err != nil || len(byID[1]) != 2 || byID[1][0].Address != "primary@example.net" || byID[1][1].Address != "alias@example.org" || byID[3] == nil || byID[2] != nil {
		t.Fatalf("unexpected address batch: %+v %v", byID, err)
	}
}
