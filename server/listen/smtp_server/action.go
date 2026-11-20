package smtp_server

import (
	"errors"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/id"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	log "github.com/sirupsen/logrus"
	"net"
	"strings"
)

// The Backend implements SMTP server methods.
type Backend struct{}

func (bkd *Backend) NewSession(conn *smtp.Conn) (smtp.Session, error) {

	remoteAddress := conn.Conn().RemoteAddr()
	ctx := &context.Context{}
	ctx.SetValue(context.LogID, id.GenLogID())
	log.WithContext(ctx).Debugf("新SMTP连接")

	return &Session{
		RemoteAddress: remoteAddress,
		Ctx:           ctx,
	}, nil
}

// A Session is returned after EHLO.
type Session struct {
	RemoteAddress net.Addr
	User          string
	From          string
	To            []string
	Size          int64
	Ctx           *context.Context
}

// AuthMechanisms returns a slice of available auth mechanisms
// supported in this example.
func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain, sasl.Login}
}

// Auth is the handler for supported authenticators.
func (s *Session) Auth(mech string) (sasl.Server, error) {
	log.WithContext(s.Ctx).Debugf("Auth :%s", mech)
	if mech == sasl.Plain {
		return sasl.NewPlainServer(func(identity, username, password string) error {
			return s.AuthPlain(username, password)
		}), nil
	}

	if mech == sasl.Login {
		return NewLoginServer(func(username, password string) error {
			return s.AuthPlain(username, password)
		}), nil
	}

	return nil, errors.New("Auth Not Supported")
}

func (s *Session) AuthPlain(username, pwd string) error {
	remote := ""
	if s.RemoteAddress != nil {
		remote = s.RemoteAddress.String()
	}
	user, err := auth.Authenticate(username, pwd, remote)
	if err != nil {
		return err
	}
	s.User = username
	s.Ctx.UserAccount, s.Ctx.UserID, s.Ctx.UserName, s.Ctx.IsAdmin = user.Account, user.ID, user.Name, user.IsAdmin == 1
	return nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	if s.Ctx.UserID > 0 && !s.Ctx.IsAdmin {
		if _, err := address.SendAddress(s.Ctx.UserID, from); err != nil {
			return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "sender address is not owned by authenticated user"}
		}
	}
	if opts != nil {
		s.Size = opts.Size
	}
	s.From = from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	log.WithContext(s.Ctx).Debugf("Rcpt Success %+v", to)
	_, host, ok := strings.Cut(strings.TrimSpace(to), "@")
	if ok && domain.IsLocal(host) {
		if user, found, err := address.Resolve(to); err != nil {
			return err
		} else if found {
			if err := quota.Check(user, s.Size); err != nil {
				if errors.Is(err, quota.ErrExceeded) {
					return &smtp.SMTPError{Code: 452, EnhancedCode: smtp.EnhancedCode{4, 2, 2}, Message: "Mailbox storage quota exceeded"}
				}
				return err
			}
		} else if !config.Instance.CatchAll {
			return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "User unknown"}
		}
	}
	s.To = append(s.To, to)
	return nil
}

func (s *Session) Reset() {
	s.From = ""
	s.To = nil
	s.Size = 0
}

func (s *Session) Logout() error {
	return nil
}
