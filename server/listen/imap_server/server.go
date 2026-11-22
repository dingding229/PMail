package imap_server

import (
	"errors"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/id"
	"github.com/emersion/go-imap/v2"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2/imapserver"
)

// Server is a server instance.
//
// A server contains a list of users.
type Server struct {
	mutex sync.Mutex
}

// NewServer creates a new server.
func NewServer() *Server {
	return &Server{}
}

type Status int8

const (
	UNAUTHORIZED Status = 1
	AUTHORIZED   Status = 2
	SELECTED     Status = 3
	LOGOUT       Status = 4
)

type serverSession struct {
	server         *Server // immutable
	ctx            *context.Context
	status         Status
	currentMailbox string
	connectTime    time.Time
	announced      []imap.UID
	readOnly       bool
	remoteAddr     string
}

// NewSession creates a new IMAP session.
func (s *Server) NewSession() imapserver.Session { return s.newSession("") }

func (s *Server) newSession(remote string) imapserver.Session {
	tc := &context.Context{}
	tc.SetValue(context.LogID, id.GenLogID())

	return &serverSession{
		server:      s,
		ctx:         tc,
		connectTime: time.Now(),
		remoteAddr:  remote,
	}
}

func (s *serverSession) Close() error {
	return nil
}

func (s *serverSession) Subscribe(mailbox string) error {
	return nil
}

func (s *serverSession) Unsubscribe(mailbox string) error {
	return nil
}

func (s *serverSession) Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	return nil, protocolNO(errors.New("APPEND is not supported"))
}

func (s *serverSession) Unselect() error {
	s.currentMailbox = ""
	s.announced = nil
	s.readOnly = false
	return nil
}
