package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jinnrry/pmail/models"
	pctx "github.com/Jinnrry/pmail/utils/context"
)

func TestC7bQueueFencingRejectsStaleCompletion(t *testing.T) {
	engine := newQueueTestEngine(t)
	if _, err := Enqueue(KindSend, 1, 0, "first@example.test", "last@example.test", []byte("body")); err != nil {
		t.Fatal(err)
	}
	stale, ok, err := claimOne()
	if err != nil || !ok {
		t.Fatal(stale, err)
	}
	if err = recoverExpiredLeases(time.Now().Add(11 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	active, ok, err := claimOne()
	if err != nil || !ok || active.LeaseFence <= stale.LeaseFence {
		t.Fatal(active, err)
	}
	handleSuccess(stale)
	handleFailure(context.Background(), stale, errors.New("expired worker"))
	var row models.OutboundQueue
	if _, err := engine.ID(active.ID).Get(&row); err != nil || row.Status != StatusSending || row.LeaseFence != active.LeaseFence {
		t.Fatalf("stale worker updated row: %+v %v", row, err)
	}
	handleSuccess(active)
	row = models.OutboundQueue{}
	if _, err := engine.ID(active.ID).Get(&row); err != nil || row.Status != StatusSent {
		t.Fatalf("active worker did not commit: %+v %v", row, err)
	}
}

func TestC7bQueueCancelsAndDrainsWorkers(t *testing.T) {
	newQueueTestEngine(t)
	t.Setenv("PMAIL_QUEUE_WORKERS", "1")
	handlers.RLock()
	old := handlers.external
	handlers.RUnlock()
	t.Cleanup(func() { SetExternalDelivery(old) })
	started := make(chan struct{})
	SetExternalDelivery(func(ctx *pctx.Context, _ string, _ []byte, _ string) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if _, err := Enqueue(KindSend, 1, 0, "first@example.test", "last@example.test", []byte("body")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := Start(ctx)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker never claimed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not drain")
	}
}

func TestC7bBounceReservationOneWinnerAfterExpiry(t *testing.T) {
	engine := newQueueTestEngine(t)
	if _, err := engine.NoAutoTime().Insert(&models.BounceNotice{NoticeKey: "3:receiver@example.test", RcptTo: "receiver@example.test", UserID: 3, Created: time.Now().Add(-25 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			yes, err := ReserveBounceNotice(3, "receiver@example.test")
			if err != nil {
				t.Error(err)
			}
			if yes {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("reservation count: %d", wins.Load())
	}
}
