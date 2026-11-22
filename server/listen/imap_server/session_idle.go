package imap_server

import (
	"errors"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/emersion/go-imap/v2/imapserver"
)

// One coalesced notification slot per connection, bounded globally. Producers
// never perform queries, write to a client socket or spawn goroutines.
var idleQueue = struct {
	sync.Mutex
	subscribers map[int]map[chan struct{}]struct{}
	count       int
}{subscribers: make(map[int]map[chan struct{}]struct{})}

func listenIdle(user int) (chan struct{}, func(), error) {
	idleQueue.Lock()
	defer idleQueue.Unlock()
	if idleQueue.count >= 1024 {
		return nil, nil, errors.New("IDLE subscriber limit reached")
	}
	c := make(chan struct{}, 1)
	if idleQueue.subscribers[user] == nil {
		idleQueue.subscribers[user] = make(map[chan struct{}]struct{})
	}
	idleQueue.subscribers[user][c] = struct{}{}
	idleQueue.count++
	return c, func() {
		idleQueue.Lock()
		defer idleQueue.Unlock()
		delete(idleQueue.subscribers[user], c)
		if len(idleQueue.subscribers[user]) == 0 {
			delete(idleQueue.subscribers, user)
		}
		idleQueue.count--
	}, nil
}

func notifyUser(user int) {
	idleQueue.Lock()
	defer idleQueue.Unlock()
	for c := range idleQueue.subscribers[user] {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

func (s *serverSession) Idle(w *imapserver.UpdateWriter, stop <-chan struct{}) error {
	if err := s.requireSession(); err != nil {
		return err
	}
	c, release, err := listenIdle(s.ctx.UserID)
	if err != nil {
		return protocolNO(err)
	}
	defer release()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	if err = s.Poll(w, true); err != nil {
		return err
	}
	for {
		select {
		case <-stop:
			return nil
		case <-c:
		case <-tick.C:
		}
		if err = s.Poll(w, true); err != nil {
			return err
		}
	}
}

func IdleNotice(_ *context.Context, userID int, email *models.Email) error {
	if userID > 0 && email != nil && email.Id > 0 {
		notifyUser(userID)
	}
	return nil
}
