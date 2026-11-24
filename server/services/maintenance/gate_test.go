package maintenance

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDrainAndAbort(t *testing.T) {
	var g Gate
	stopped := make(chan struct{})
	leave, ok := g.Enter(func() { close(stopped) })
	if !ok {
		t.Fatal("gate unexpectedly active")
	}
	done := make(chan func(), 1)
	go func() {
		release, err := g.Begin(t.Context())
		if err != nil {
			done <- nil
			return
		}
		done <- release
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("existing work not canceled")
	}
	if _, ok = g.Enter(nil); ok {
		t.Fatal("new work accepted during drain")
	}
	select {
	case <-done:
		t.Fatal("did not wait for a registered operation")
	default:
	}
	leave()
	select {
	case release := <-done:
		if release == nil {
			t.Fatal("drain failed")
		}
		release()
	case <-time.After(time.Second):
		t.Fatal("did not finish after leave")
	}
	leave, ok = g.Enter(nil)
	if !ok {
		t.Fatal("did not resume")
	}
	defer leave()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := g.Begin(ctx); !errors.Is(err, context.DeadlineExceeded) || g.Active() {
		t.Fatalf("timeout left maintenance active: %v", err)
	}
}
