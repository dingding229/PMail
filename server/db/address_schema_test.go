package db

import (
	"testing"

	"github.com/Jinnrry/pmail/models"
	"xorm.io/xorm"
)

// Schema before C29; this test exercises ALTER + index replacement, not just
// creating a fresh database that already has the new defaults.
type legacyMailboxAddress struct {
	ID        int    `xorm:"id int notnull pk autoincr"`
	UserID    int    `xorm:"user_id bigint notnull index"`
	LocalPart string `xorm:"local_part varchar(64) notnull unique(address)"`
	DomainID  int    `xorm:"domain_id int notnull index unique(address)"`
	IsPrimary bool   `xorm:"is_primary bool notnull default(false)"`
	CanSend   bool   `xorm:"can_send bool notnull default(true)"`
}

func (legacyMailboxAddress) TableName() string { return "mailbox_address" }

func TestAddressApprovalSchemaUpgradesExistingRowsAndReservationIndex(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	defer e.Close()
	if err := e.Sync2(&legacyMailboxAddress{}); err != nil {
		t.Fatal(err)
	}
	old := legacyMailboxAddress{UserID: 1, LocalPart: "old", DomainID: 1, IsPrimary: true, CanSend: true}
	if _, err := e.Insert(&old); err != nil {
		t.Fatal(err)
	}
	if err := SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	var migrated models.MailboxAddress
	if ok, err := e.ID(old.ID).Get(&migrated); err != nil || !ok || migrated.Status != models.AddressActive || !migrated.IsPrimary || !migrated.CanSend || migrated.RequestedAt != nil || migrated.ReservationKey != 0 {
		t.Fatalf("legacy address was changed or not activated: %+v %v", migrated, err)
	}
	duplicate := models.MailboxAddress{UserID: 2, LocalPart: "old", DomainID: 1, Status: models.AddressPending}
	if _, err := e.Insert(&duplicate); !IsUniqueViolation(err) {
		t.Fatalf("claim is not unique: %v", err)
	}
	pending := models.MailboxAddress{UserID: 2, LocalPart: "new", DomainID: 1, Status: models.AddressPending}
	if _, err := e.Insert(&pending); err != nil {
		t.Fatal(err)
	}
	rejected := models.MailboxAddress{Status: models.AddressRejected, ReservationKey: pending.ID}
	if _, err := e.ID(pending.ID).Cols("status", "reservation_key").Update(&rejected); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Insert(&models.MailboxAddress{UserID: 1, LocalPart: "new", DomainID: 1, Status: models.AddressPending}); err != nil {
		t.Fatalf("legacy index still blocks released addresses: %v", err)
	}
	if err := SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	var row models.MailboxAddress
	if ok, err := e.ID(pending.ID).Get(&row); err != nil || !ok || row.Status != models.AddressRejected || row.ReservationKey != pending.ID {
		t.Fatal("running migration again changed review history")
	}
}

func TestAddressSchemaPreservesConflictingLegacyRows(t *testing.T) {
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	e.SetMaxOpenConns(1)
	if err := e.Sync2(&legacyMailboxAddress{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Exec(`DROP INDEX "UQE_mailbox_address_address"`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Exec(`INSERT INTO mailbox_address (user_id,local_part,domain_id) VALUES (1,'conflict',1),(2,'conflict',1)`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := SyncSchema(e); err != nil {
			t.Fatalf("dirty data must not block startup: %v", err)
		}
		if n, err := e.Count(&models.MailboxAddress{}); err != nil || n != 2 {
			t.Fatalf("migration changed conflicting rows: %d/%v", n, err)
		}
	}
	// Once repaired by the operator the next startup retries the constraint.
	if _, err := e.Exec(`DELETE FROM mailbox_address WHERE user_id=2`); err != nil {
		t.Fatal(err)
	}
	if err := SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Exec(`INSERT INTO mailbox_address (user_id,local_part,domain_id) VALUES (2,'conflict',1)`); !IsUniqueViolation(err) {
		t.Fatalf("reservation index was not installed after repair: %v", err)
	}
}
