package external

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

var ErrSyncInProgress = errors.New("external account is already syncing or scheduler is unavailable")

type scheduler struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	closed  bool
	active  map[int]bool
	permits chan struct{}
	wg      sync.WaitGroup
	run     func(context.Context, int) error
}

func newScheduler(parent context.Context, workers int, run func(context.Context, int) error) *scheduler {
	ctx, cancel := context.WithCancel(parent)
	return &scheduler{ctx: ctx, cancel: cancel, active: map[int]bool{}, permits: make(chan struct{}, workers), run: run}
}

func (s *scheduler) acquire(id int) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil || s.active[id] {
		return nil, false
	}
	select {
	case s.permits <- struct{}{}:
	default:
		return nil, false
	}
	s.active[id] = true
	s.wg.Add(1)
	return func() { s.mu.Lock(); delete(s.active, id); <-s.permits; s.mu.Unlock(); s.wg.Done() }, true
}

func (s *scheduler) launch(id int) error {
	release, ok := s.acquire(id)
	if !ok {
		return ErrSyncInProgress
	}
	go func() {
		defer release()
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("external sync panic contained account_id=%d kind=%T", id, r)
			}
		}()
		if err := s.run(s.ctx, id); err != nil {
			log.WithField("account_id", id).Warnf("external sync failed: %v", err)
		}
	}()
	return nil
}

func (s *scheduler) drain(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

var current struct {
	sync.Mutex
	s *scheduler
}
var direct = newScheduler(context.Background(), 32, syncAccountLocked)

// Start owns a generation; the caller must drain it before replacing the DB.
func Start(ctx context.Context) func(context.Context) error {
	workers := 3
	if n, err := strconv.Atoi(strings.TrimSpace(getenv("PMAIL_FETCH_WORKERS"))); err == nil && n > 0 && n <= 32 {
		workers = n
	}
	s := newScheduler(ctx, workers, syncAccountLocked)
	current.Lock()
	current.s = s
	current.Unlock()
	s.wg.Go(func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-tick.C:
				scan(s)
			}
		}
	})
	return func(wait context.Context) error {
		current.Lock()
		if current.s == s {
			current.s = nil
		}
		current.Unlock()
		return s.drain(wait)
	}
}

func scheduleNow(id int) error {
	current.Lock()
	s := current.s
	current.Unlock()
	if s == nil {
		return ErrSyncInProgress
	}
	return s.launch(id)
}

// SyncAccount is also used by protocol fixtures without starting the periodic worker.
func SyncAccount(ctx context.Context, id int) (err error) {
	current.Lock()
	s := current.s
	current.Unlock()
	if s == nil {
		s = direct
	}
	release, ok := s.acquire(id)
	if !ok {
		return ErrSyncInProgress
	}
	defer release()
	job, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("external sync panic contained: %T", r)
		}
	}()
	return s.run(job, id)
}
