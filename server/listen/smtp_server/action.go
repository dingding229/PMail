package smtp_server

import (
	stdctx "context"
	"errors"
	"net"
	"net/netip"
	"strings"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/antivirus"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/id"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	log "github.com/sirupsen/logrus"
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
		currentHELO:   conn.Hostname,
		Ctx:           ctx,
	}, nil
}

// A Session is returned after EHLO.
type Session struct {
	RemoteAddress       net.Addr
	User                string
	HELO                string
	currentHELO         func() string
	scanInbound         func(stdctx.Context, []byte) *antivirus.Result
	AuthenticateInbound func(stdctx.Context, netip.Addr, string, string, []byte) models.MailAuthResults
	From                string
	To                  []string
	Size                int64
	Ctx                 *context.Context
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
	user, appID, err := auth.AuthenticateMail(username, pwd, remote)
	if err != nil {
		return smtpAuthenticationError(err)
	}
	s.User = username
	s.Ctx.UserAccount, s.Ctx.UserID, s.Ctx.UserName, s.Ctx.IsAdmin = user.Account, user.ID, user.Name, user.IsAdmin == 1
	s.Ctx.AuthVersion, s.Ctx.FactorVersion, s.Ctx.AppPasswordID = user.AuthVersion, user.FactorVersion, appID
	return nil
}

// SMTP clients should stop retrying invalid credentials, but can retry a
// rate-limited request or an unavailable authentication backend later.
func smtpAuthenticationError(err error) *smtp.SMTPError {
	switch {
	case errors.Is(err, auth.ErrCredentials):
		return &smtp.SMTPError{Code: 535, EnhancedCode: smtp.EnhancedCode{5, 7, 8}, Message: "authentication credentials invalid"}
	case errors.Is(err, auth.ErrLocked):
		return &smtp.SMTPError{Code: 454, EnhancedCode: smtp.EnhancedCode{4, 7, 0}, Message: "too many authentication attempts; try again later"}
	default:
		return &smtp.SMTPError{Code: 454, EnhancedCode: smtp.EnhancedCode{4, 7, 0}, Message: "authentication temporarily unavailable; try again later"}
	}
}

func (s *Session) validateSubmission() error {
	if s.Ctx.UserID > 0 && auth.ValidateMailSession(s.Ctx.UserID, s.Ctx.AuthVersion, s.Ctx.FactorVersion, s.Ctx.AppPasswordID, &s.Ctx.IsAdmin) != nil {
		return &smtp.SMTPError{Code: 535, EnhancedCode: smtp.EnhancedCode{5, 7, 8}, Message: "authentication expired; reconnect and authenticate"}
	}
	return nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	if err := s.validateSubmission(); err != nil {
		return err
	}
	if s.Ctx.UserID > 0 {
		if err := address.CheckSubmission(s.Ctx.UserID, from, s.Ctx.IsAdmin); err != nil {
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
	if s.Ctx.UserID <= 0 && (!ok || !domain.IsLocal(host)) {
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "relaying denied"}
	}
	if ok && domain.IsLocal(host) {
		if forward.SRSAddress(to) && !forward.AllowSRSLookup(s.RemoteAddress) {
			return &smtp.SMTPError{Code: 452, EnhancedCode: smtp.EnhancedCode{4, 7, 0}, Message: "bounce lookup rate exceeded"}
		}
		if _, isSRS, srsErr := forward.ResolveSRS(to); isSRS {
			if srsErr != nil {
				return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "Invalid SRS recipient"}
			}
			s.To = append(s.To, to)
			return nil
		}
		user, found, err := address.Resolve(to)
		if err != nil {
			return localPersistenceSMTPError()
		}
		if !found {
			if _, _, err := address.Split(to); err != nil {
				return unknownRecipientSMTPError()
			}
			user, found, err = inbound.CatchAllRecipient(s.Ctx.Context)
			if err != nil {
				return localPersistenceSMTPError()
			}
			if !found {
				return unknownRecipientSMTPError()
			}
		}
		if err := quota.Check(user, s.Size); err != nil {
			if errors.Is(err, quota.ErrExceeded) {
				return quotaSMTPError()
			}
			return err
		}
	}
	s.To = append(s.To, to)
	return nil
}

func unknownRecipientSMTPError() error {
	return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "User unknown"}
}

func (s *Session) Reset() {
	s.From = ""
	s.To = nil
	s.Size = 0
}

func (s *Session) Logout() error {
	return nil
}
