package imap_server

import (
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/emersion/go-imap/v2"
)

func (s *serverSession) Login(username, pwd string) error {
	user, err := auth.Authenticate(username, pwd, s.remoteAddr)
	if err != nil {
		return authFailure()
	}
	s.status = AUTHORIZED
	s.ctx.UserID, s.ctx.UserName, s.ctx.UserAccount, s.ctx.AuthVersion = user.ID, user.Name, user.Account, user.AuthVersion
	return nil
}

func authFailure() *imap.Error {
	return &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeAuthenticationFailed, Text: "Invalid credentials or expired session"}
}

func (s *serverSession) requireSession() error {
	if err := auth.ValidateSession(s.ctx.UserID, s.ctx.AuthVersion); err != nil {
		return authFailure()
	}
	return nil
}
