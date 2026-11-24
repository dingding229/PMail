// Package maintenance coordinates database replacement with active clients and
// workers. It deliberately does not depend on the database or HTTP routing.
package maintenance

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

var ErrBusy = errors.New("maintenance")
var Default Gate

type Gate struct {
	mu     sync.Mutex
	active bool
	next   uint64
	tasks  map[uint64]func()
	empty  chan struct{}
}

func (g *Gate) Active() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.active }
func (g *Gate) Enter(stop func()) (func(), bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active {
		return nil, false
	}
	if len(g.tasks) == 0 {
		g.empty = make(chan struct{})
		g.tasks = map[uint64]func(){}
	}
	g.next++
	id := g.next
	g.tasks[id] = stop
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		if _, ok := g.tasks[id]; ok {
			delete(g.tasks, id)
			if len(g.tasks) == 0 {
				close(g.empty)
			}
		}
	}, true
}
func (g *Gate) begin() (chan struct{}, []func(), error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active {
		return nil, nil, ErrBusy
	}
	g.active = true
	var stops []func()
	for _, fn := range g.tasks {
		if fn != nil {
			stops = append(stops, fn)
		}
	}
	if len(g.tasks) == 0 {
		return nil, stops, nil
	}
	return g.empty, stops, nil
}
func (g *Gate) Begin(ctx context.Context) (func(), error) {
	empty, stops, err := g.begin()
	if err != nil {
		return nil, err
	}
	var once sync.Once
	resume := func() { once.Do(func() { g.mu.Lock(); g.active = false; g.mu.Unlock() }) }
	for _, stop := range stops {
		stop()
	}
	if empty != nil {
		select {
		case <-empty:
		case <-ctx.Done():
			resume()
			return nil, ctx.Err()
		}
	}
	return resume, nil
}

// Work registers a bounded job and cancels its context when maintenance starts.
func Work(ctx context.Context, fn func(context.Context)) bool {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done, ok := Default.Enter(cancel)
	if !ok {
		return false
	}
	defer done()
	fn(ctx)
	return true
}

// Wait runs a worker only after maintenance is open, without busy spinning.
func Wait(ctx context.Context) bool {
	for Default.Active() {
		t := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return false
		case <-t.C:
		}
	}
	return ctx.Err() == nil
}

type listener struct {
	net.Listener
	tls      *tls.Config
	greeting string
}
type tracked struct {
	net.Conn
	done func()
}

func (c *tracked) Close() error { defer c.done(); return c.Conn.Close() }

// Listen preserves the concrete *tls.Conn type recognized by SMTP/IMAP. For
// live connections maintenance closes the underlying socket, and the serving
// library releases its slot when it finishes the current handler and closes it.
func Listen(l net.Listener, tlsConfig *tls.Config, greeting string) net.Listener {
	return &listener{Listener: l, tls: tlsConfig, greeting: greeting}
}
func (l *listener) Accept() (net.Conn, error) {
	for {
		raw, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		done, ok := Default.Enter(func() { _ = raw.Close() })
		c := raw
		if ok {
			c = &tracked{Conn: raw, done: done}
		}
		if l.tls != nil {
			c = tls.Server(c, l.tls)
		}
		if ok {
			return c, nil
		}
		_ = raw.SetDeadline(time.Now().Add(time.Second))
		_, _ = io.WriteString(c, l.greeting)
		_ = c.Close()
	}
}
