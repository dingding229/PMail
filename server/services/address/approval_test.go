package address

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/domain"
	"xorm.io/xorm"
)

func approvalFixture(t *testing.T) (*xorm.Engine, int, models.User, models.User) {
	t.Helper()
	e, err := xorm.NewEngine("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	e.SetMaxOpenConns(1)
	old := db.Instance
	db.Instance = e
	t.Cleanup(func() {
		db.Instance = old
		e.Close()
		if old != nil {
			_ = domain.Refresh()
		}
	})
	if err := db.SyncSchema(e); err != nil {
		t.Fatal(err)
	}
	if err := domain.Initialize(&config.Config{Domain: "approval.test"}); err != nil {
		t.Fatal(err)
	}
	a, b := models.User{Account: "applicant"}, models.User{Account: "reviewer", IsAdmin: 1}
	if _, err := e.Insert(&a, &b); err != nil {
		t.Fatal(err)
	}
	return e, domain.Default().ID, a, b
}

func TestAddressApprovalPolicyAndPermissions(t *testing.T) {
	e, d, a, b := approvalFixture(t)
	ctx := t.Context()
	if enabled, err := ApprovalRequired(ctx); err != nil || !enabled {
		t.Fatalf("default approval: %v %v", enabled, err)
	}
	if err := db.Transaction(e, func(tx *xorm.Session) error { return SeedPrimary(tx, a) }); err != nil {
		t.Fatal(err)
	}
	if row, err := Primary(a.ID); err != nil || row.Status != models.AddressActive {
		t.Fatalf("registration primary blocked: %v", err)
	}
	pending, err := Create(ctx, a.ID, d, "Alias", true, false)
	if err != nil || pending.Status != models.AddressPending || pending.RequestedAt == nil || pending.IsPrimary || pending.CanSend {
		t.Fatalf("pending settings: %+v %v", pending, err)
	}
	if _, found, err := OwnerForLogin(pending.Address); err != nil || found {
		t.Fatalf("pending login resolved: %v", err)
	}
	if _, err := SendAddress(a.ID, pending.Address); !errors.Is(err, ErrForbidden) {
		t.Fatalf("pending send: %v", err)
	}
	if err := CheckSubmission(b.ID, pending.Address, true); !errors.Is(err, ErrInactive) {
		t.Fatalf("admin SMTP used pending address: %v", err)
	}
	if err := SetPrimary(pending.ID, a.ID, false); !errors.Is(err, ErrInactive) {
		t.Fatalf("pending primary: %v", err)
	}
	if err := SetCanSend(pending.ID, a.ID, false, true); !errors.Is(err, ErrInactive) {
		t.Fatalf("pending edit: %v", err)
	}
	for _, owner := range []int{a.ID, b.ID} {
		if row, err := Create(ctx, owner, d, fmt.Sprintf("admin-%d", owner), true, true); err != nil || row.Status != models.AddressActive || row.RequestedAt != nil {
			t.Fatalf("admin provisioning failed: %v", err)
		}
	}
	if err := SetApprovalRequired(ctx, false); err != nil {
		t.Fatal(err)
	}
	if enabled, err := ApprovalRequired(ctx); err != nil || enabled {
		t.Fatalf("policy did not persist false: %v", err)
	}
	if row, err := Create(ctx, a.ID, d, "immediate", false, false); err != nil || row.Status != models.AddressActive || row.CanSend {
		t.Fatalf("policy off: %v", err)
	}
	approved, err := Review(ctx, pending.ID, b.ID, true, "")
	if err != nil || approved.Status != models.AddressActive || !approved.CanSend || approved.ReviewedAt == nil || approved.ReviewedBy == nil || *approved.ReviewedBy != b.ID {
		t.Fatalf("approval: %+v %v", approved, err)
	}
	if owner, ok, err := OwnerForLogin(pending.Address); err != nil || !ok || owner.ID != a.ID {
		t.Fatalf("approval did not enable login: %v", err)
	}
	if _, err := SendAddress(a.ID, pending.Address); err != nil {
		t.Fatal(err)
	}
	if _, err := Review(ctx, pending.ID, b.ID, false, ""); !errors.Is(err, ErrAlreadyReviewed) {
		t.Fatalf("repeat decision: %v", err)
	}
}

func TestConcurrentClaimsPendingCapAndRejectionRelease(t *testing.T) {
	_, d, a, b := approvalFixture(t)
	ctx := t.Context()
	start := make(chan struct{})
	type result struct {
		row View
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, uid := range []int{a.ID, b.ID} {
		wg.Go(func() { <-start; r, e := Create(ctx, uid, d, "Same", true, false); results <- result{r, e} })
	}
	close(start)
	wg.Wait()
	close(results)
	var winner View
	n, dups := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			n++
			winner = result.row
		case errors.Is(result.err, ErrExists):
			dups++
		default:
			t.Fatal(result.err)
		}
	}
	if n != 1 || dups != 1 {
		t.Fatal("concurrent duplicate requests succeeded")
	}
	loser := b.ID
	if winner.UserID == b.ID {
		loser = a.ID
	}
	if _, err := Review(ctx, winner.ID, b.ID, false, strings.Repeat("字", 201)); !errors.Is(err, ErrRejectReason) {
		t.Fatalf("accepted overlong reason: %v", err)
	}
	rejected, err := Review(ctx, winner.ID, b.ID, false, "review reason")
	if err != nil || rejected.Status != models.AddressRejected || rejected.Address != winner.Address || rejected.RejectReason != "review reason" {
		t.Fatalf("rejection: %v", err)
	}
	reapplied, err := Create(ctx, loser, d, "same", true, false)
	if err != nil || reapplied.ID == winner.ID {
		t.Fatalf("reject did not release: %v", err)
	}
	if err := Delete(winner.ID, loser, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user withdrew rejected request: %v", err)
	}
	if err := Delete(winner.ID, winner.UserID, false); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < pendingLimit; i++ {
		if _, err := Create(ctx, loser, d, fmt.Sprintf("alias-%d", i), true, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Create(ctx, loser, d, "over-limit", true, false); !errors.Is(err, ErrPendingLimit) {
		t.Fatalf("pending cap not enforced: %v", err)
	}
	if err := Delete(reapplied.ID, loser, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(ctx, winner.UserID, d, "same", true, false); err != nil {
		t.Fatalf("withdrawing pending request did not release address: %v", err)
	}
}

func TestApprovalRevalidatesDomainAndClaim(t *testing.T) {
	e, d, a, b := approvalFixture(t)
	ctx := t.Context()
	lost, err := Create(ctx, a.ID, d, "lost", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ID(lost.ID).Cols("reservation_key").Update(&models.MailboxAddress{ReservationKey: lost.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(ctx, b.ID, d, "lost", true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := Review(ctx, lost.ID, b.ID, true, ""); !errors.Is(err, ErrExists) {
		t.Fatalf("lost reservation approved: %v", err)
	}
	invalid, err := Create(ctx, a.ID, d, "domain-check", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ID(d).Cols("name").Update(&models.Domain{Name: "not allowed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Review(ctx, invalid.ID, b.ID, true, ""); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid domain approved despite stale cache: %v", err)
	}
	if _, err := e.ID(d).Delete(&models.Domain{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Review(ctx, invalid.ID, b.ID, true, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted domain approved: %v", err)
	}
	var saved models.MailboxAddress
	if ok, err := e.ID(invalid.ID).Get(&saved); err != nil || !ok || saved.Status != models.AddressPending || saved.ReviewedAt != nil {
		t.Fatal("failed approval mutated the request")
	}
}
