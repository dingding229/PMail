package imap_server

import (
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/emersion/go-imap/v2"
)

func (s *serverSession) Login(username, pwd string) error {
	user, appID, err := auth.AuthenticateMail(username, pwd, s.remoteAddr)
	if err != nil {
		return authFailure()
	}
	s.status = AUTHORIZED
	s.ctx.FactorVersion, s.ctx.AppPasswordID = user.FactorVersion, appID
	s.ctx.UserID, s.ctx.UserName, s.ctx.UserAccount, s.ctx.AuthVersion = user.ID, user.Name, user.Account, user.AuthVersion
	return nil
}

func authFailure() *imap.Error {
	return &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeAuthenticationFailed, Text: "Invalid credentials or expired session"}
}

func (s *serverSession) requireSession() error {
	if err := auth.ValidateMailSession(s.ctx.UserID, s.ctx.AuthVersion, s.ctx.FactorVersion, s.ctx.AppPasswordID, nil); err != nil {
		return authFailure()
	}
	return nil
}
