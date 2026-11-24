// Package mailevents coalesces mailbox changes for HTTP long polling.
package mailevents

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"slices"
	"sync"
)

const (
	MaxPerUser = 8
	MaxWaiters = 4096
)

var ErrBusy = errors.New("live_updates_busy")

type waiter struct {
	signal  chan struct{}
	evicted bool // protected by Broadcaster.mu
}

type userState struct {
	version uint64
	waiters []*waiter // registration order, oldest first
}

type Broadcaster struct {
	epoch   string
	mu      sync.Mutex
	users   map[int]*userState
	waiting int
}

func New() *Broadcaster {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic("could not generate mail events epoch")
	}
	return &Broadcaster{epoch: base64.RawURLEncoding.EncodeToString(nonce[:]), users: make(map[int]*userState)}
}

var Default = New()

func (b *Broadcaster) Epoch() string { return b.epoch }

func (b *Broadcaster) Version(userID int) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if u := b.users[userID]; u != nil {
		return u.version
	}
	return 0
}

func (b *Broadcaster) state(userID int) *userState {
	u := b.users[userID]
	if u == nil {
		u = &userState{}
		b.users[userID] = u
	}
	return u
}

func Publish(userID int) { Default.Publish(userID) }

func (b *Broadcaster) Publish(userID int) {
	if userID <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	u := b.state(userID)
	u.version++
	for _, w := range u.waiters {
		w.wake()
	}
}

func (w *waiter) wake() {
	select {
	case w.signal <- struct{}{}:
	default: // a pending signal already covers this version
	}
}

type Result struct {
	Version uint64
	Changed bool
	Evicted bool
}

// Wait checks the version and registers atomically so Publish cannot be lost
// between the check and going to sleep. The caller supplies its own timeout.
func (b *Broadcaster) Wait(ctx context.Context, userID int, sinceVersion uint64) (Result, error) {
	b.mu.Lock()
	u := b.users[userID]
	if err := ctx.Err(); err != nil {
		var v uint64
		if u != nil {
			v = u.version
		}
		b.mu.Unlock()
		return Result{Version: v, Changed: v > sinceVersion}, err
	}
	if u != nil && u.version > sinceVersion {
		v := u.version
		b.mu.Unlock()
		return Result{Version: v, Changed: true}, nil
	}
	if u != nil && len(u.waiters) == MaxPerUser {
		old := u.waiters[0]
		u.waiters = slices.Delete(u.waiters, 0, 1)
		b.waiting--
		old.evicted = true
		old.wake()
	}
	if b.waiting >= MaxWaiters {
		b.mu.Unlock()
		return Result{}, ErrBusy
	}
	if u == nil {
		u = b.state(userID)
	}
	w := &waiter{signal: make(chan struct{}, 1)}
	u.waiters = append(u.waiters, w)
	b.waiting++
	b.mu.Unlock()

	var err error
	select {
	case <-w.signal:
	case <-ctx.Done():
		err = ctx.Err()
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if w.evicted {
		return Result{Version: sinceVersion, Evicted: true}, nil
	}
	if i := slices.Index(u.waiters, w); i >= 0 {
		u.waiters = slices.Delete(u.waiters, i, i+1)
		b.waiting--
	}
	result := Result{Version: u.version, Changed: u.version > sinceVersion}
	// Never evict a version that has been published, but avoid retaining a
	// zero-version entry just because somebody opened a long poll once.
	if len(u.waiters) == 0 && u.version == 0 {
		delete(b.users, userID)
	}
	return result, err
}
