package mailevents

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type answer struct {
	result Result
	err    error
}

func waitAsync(b *Broadcaster, ctx context.Context, uid int, v uint64) <-chan answer {
	out := make(chan answer, 1)
	go func() { result, err := b.Wait(ctx, uid, v); out <- answer{result, err} }()
	return out
}
func receive(t *testing.T, out <-chan answer) answer {
	t.Helper()
	select {
	case a := <-out:
		return a
	case <-time.After(3 * time.Second):
		t.Fatal("waiter did not return")
		return answer{}
	}
}
func requireWaiting(t *testing.T, b *Broadcaster, n int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for {
		b.mu.Lock()
		waiting := b.waiting
		b.mu.Unlock()
		if waiting == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("want %d waiters, got %d", n, waiting)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEpochVersionImmediateWakeAndIsolation(t *testing.T) {
	b := New()
	if b.Epoch() == "" || b.Epoch() == New().Epoch() {
		t.Fatal("epoch must be random per process startup")
	}
	b.Publish(1)
	if got, err := b.Wait(t.Context(), 1, 0); err != nil || got != (Result{Version: 1, Changed: true}) {
		t.Fatalf("immediate wait: %+v %v", got, err)
	}
	out := waitAsync(b, t.Context(), 1, 1)
	requireWaiting(t, b, 1)
	b.Publish(2)
	select {
	case <-out:
		t.Fatal("another user woke the waiter")
	default:
	}
	b.Publish(1)
	if a := receive(t, out); a.err != nil || a.result != (Result{Version: 2, Changed: true}) {
		t.Fatalf("wake: %+v", a)
	}
	requireWaiting(t, b, 0)
}

func TestWaitTimeoutAndCancellationFreeSlots(t *testing.T) {
	b := New()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if got, err := b.Wait(ctx, 1, 0); !errors.Is(err, context.DeadlineExceeded) || got.Changed || got.Evicted {
		t.Fatalf("timeout: %+v %v", got, err)
	}
	requireWaiting(t, b, 0)
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	out := waitAsync(b, ctx, 1, 0)
	requireWaiting(t, b, 1)
	cancel()
	if a := receive(t, out); !errors.Is(a.err, context.Canceled) || a.result.Changed {
		t.Fatalf("cancel: %+v", a)
	}
	requireWaiting(t, b, 0)
}

func TestNinthWaitReplacesOldest(t *testing.T) {
	b := New()
	var tabs []<-chan answer
	for i := 1; i <= MaxPerUser; i++ {
		tabs = append(tabs, waitAsync(b, t.Context(), 1, 0))
		requireWaiting(t, b, i)
	}
	ninth := waitAsync(b, t.Context(), 1, 0)
	if a := receive(t, tabs[0]); a.err != nil || a.result != (Result{Evicted: true}) {
		t.Fatalf("oldest tab: %+v", a)
	}
	requireWaiting(t, b, MaxPerUser)
	b.Publish(1)
	for _, w := range append(tabs[1:], ninth) {
		if a := receive(t, w); a.err != nil || a.result != (Result{Version: 1, Changed: true}) {
			t.Fatalf("another tab was displaced: %+v", a)
		}
	}
	requireWaiting(t, b, 0)
}

func TestGlobalCapAndConcurrentPublish(t *testing.T) {
	b := New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var wg sync.WaitGroup
	for i := range MaxWaiters {
		wg.Go(func() { _, _ = b.Wait(ctx, i+1, 0) })
	}
	requireWaiting(t, b, MaxWaiters)
	if _, err := b.Wait(ctx, MaxWaiters+1, 0); !errors.Is(err, ErrBusy) {
		t.Fatalf("global cap: %v", err)
	}
	cancel()
	wg.Wait()
	requireWaiting(t, b, 0)
	for range 8 {
		wg.Go(func() {
			for range 100 {
				b.Publish(1)
				_ = b.Version(1)
			}
		})
	}
	wg.Wait()
	if b.Version(1) != 800 {
		t.Fatal("concurrent version increments lost")
	}
}
