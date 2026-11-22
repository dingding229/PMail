package auth

import (
	"sync"
	"time"
)

const (
	budgetWindow     = time.Minute
	globalRate       = 1200
	sourceRate       = 180
	globalConcurrent = 64
	sourceConcurrent = 8
)

type quotaBucket struct{ n, active int }
type lookupBudget struct {
	mu            sync.Mutex
	start         time.Time
	calls, active int
	sources       map[string]*quotaBucket
}

var lookupLimits lookupBudget

func (b *lookupBudget) acquire(ip string, now time.Time) (func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.start.IsZero() || now.Sub(b.start) >= budgetWindow {
		b.start, b.calls = now, 0
		if b.sources == nil {
			b.sources = make(map[string]*quotaBucket)
		}
		for key, source := range b.sources {
			if source.active == 0 {
				delete(b.sources, key)
			} else {
				source.n = 0
			}
		}
	}
	if b.calls >= globalRate || b.active >= globalConcurrent {
		return nil, false
	}
	source := b.sources[ip]
	if source == nil {
		source = &quotaBucket{}
		b.sources[ip] = source
	}
	if source.n >= sourceRate || source.active >= sourceConcurrent {
		return nil, false
	}
	source.n++
	source.active++
	b.calls++
	b.active++
	return func() { b.mu.Lock(); defer b.mu.Unlock(); source.active--; b.active-- }, true
}
