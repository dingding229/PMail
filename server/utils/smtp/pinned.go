package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"

	"github.com/Jinnrry/pmail/utils/wiretext"
)

type TransportMode int

const (
	VerifySTARTTLS TransportMode = iota
	OpportunisticSTARTTLS
	CleartextFallback
)

var ErrTLSNegotiation = errors.New("SMTP TLS negotiation failed")

// SendPinned never resolves or reconnects to any address. It owns conn and
// only labels a pre-DATA TLS negotiation error as eligible for downgrade.
func SendPinned(ctx context.Context, conn net.Conn, mx, from, fromDomain string, to []string, raw []byte, mode TransportMode) error {
	defer conn.Close()
	if (from != "" && !wiretext.Address(from)) || !wiretext.Line(fromDomain) {
		return errors.New("invalid SMTP sender")
	}
	for _, rcpt := range to {
		if !wiretext.Address(rcpt) {
			return errors.New("invalid SMTP recipient")
		}
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	c, err := NewClient(conn, mx, fromDomain)
	if err != nil {
		return err
	}
	defer c.Close()
	if err = c.hello(); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); ok && mode != CleartextFallback {
		tlsConfig := &tls.Config{ServerName: mx, MinVersion: tls.VersionTLS12, InsecureSkipVerify: mode == OpportunisticSTARTTLS}
		if err = c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("%w: %v", ErrTLSNegotiation, err)
		}
	} else if mode == VerifySTARTTLS {
		return fmt.Errorf("%w: %v", ErrTLSNegotiation, NoSupportSTARTTLSError)
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err = c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	return finishDelivery(c, w, raw)
}
