package send

import (
	stdctx "context"
	"errors"
	"fmt"
	"net/textproto"
	"strings"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/utils/array"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

type LocalDeliveryFunc func(*context.Context, *parsemail.Email, []*parsemail.User) (map[string]error, error)

var localDelivery LocalDeliveryFunc

func SetLocalDelivery(fn LocalDeliveryFunc) { localDelivery = fn }

func Send(ctx *context.Context, e *parsemail.Email) (map[string]error, error) {
	// Capture the sender's audit row before local delivery. saveEmail assigns a
	// new MessageId to each local copy, so queue rows must never derive their
	// parent id from the mutable delivery email after local persistence.
	parentEmailID := int(e.MessageId)
	// The signed bytes are the actual message delivered to SMTP and are also
	// the authoritative size for local quota accounting and persisted records.
	data, err := e.BuildBytes(ctx, true)
	if err != nil {
		return nil, err
	}
	e.Size = len(data)
	all := append(append(append([]*parsemail.User{}, e.To...), e.Cc...), e.Bcc...)
	var local, external []*parsemail.User
	for _, recipient := range all {
		_, host, ok := strings.Cut(recipient.EmailAddress, "@")
		if ok && domain.IsLocal(host) {
			local = append(local, recipient)
		} else {
			external = append(external, recipient)
		}
	}
	var first error
	deliveryErrors := make(map[string]error)
	if len(local) > 0 {
		var err error
		var localErrors map[string]error
		if localDelivery == nil {
			return deliveryErrors, errors.New("local delivery handler is unavailable")
		}
		localEmail := *e
		localErrors, err = localDelivery(ctx, &localEmail, local)
		for _, recipient := range local {
			if deliveryErr, failed := localErrors[recipient.EmailAddress]; failed {
				deliveryErrors[recipient.EmailAddress] = deliveryErr
			} else {
				deliveryErrors[recipient.EmailAddress] = err
			}
		}
		if err != nil {
			first = err
		}
	}
	for _, recipient := range external {
		_, err := queue.Enqueue(queue.KindSend, ctx.UserID, parentEmailID, e.From.EmailAddress, recipient.EmailAddress, data)
		if err != nil {
			deliveryErrors[recipient.EmailAddress] = err
			if first == nil {
				first = err
			}
			continue
		}
		deliveryErrors[recipient.EmailAddress] = queue.ErrQueued
	}
	return deliveryErrors, first
}

func External(ctx *context.Context, fromDomain, envelope string, raw []byte, recipients []*parsemail.User) (map[string]error, error) {
	return doSend(ctx, fromDomain, raw, recipients, envelope)
}

func doSend(ctx *context.Context, fromDomain string, data []byte, to []*parsemail.User, from string) (map[string]error, error) {
	p, err := outboundPolicy()
	if err != nil {
		return map[string]error{"policy": err}, err
	}
	base := stdctx.Background()
	if ctx != nil && ctx.Context != nil {
		base = ctx.Context
	}
	recipients := map[string][]string{}
	failures := map[string]error{}
	var failAddrs []string
	for _, item := range to {
		if item == nil {
			failures[""] = errors.New("invalid SMTP recipient")
			failAddrs = append(failAddrs, "invalid recipient")
			continue
		}
		_, host, ok := strings.Cut(item.EmailAddress, "@")
		host = hostKey(host)
		if !ok || host == "" {
			failures[item.EmailAddress] = errors.New("invalid SMTP recipient")
			failAddrs = append(failAddrs, item.EmailAddress)
			continue
		}
		recipients[host] = append(recipients[host], item.EmailAddress)
	}
	for host, list := range recipients {
		if err := p.deliver(base, host, from, fromDomain, list, data); err != nil {
			log.WithContext(ctx).Warnf("SMTP delivery failed recipient_domain=%q error=%v", host, err)
			failures[host] = err
			failAddrs = append(failAddrs, list...)
		}
	}
	if len(failures) > 0 {
		return failures, fmt.Errorf("delivery failed for %s", array.Join(failAddrs, ","))
	}
	return failures, nil
}

func isPermanentSMTPResponse(err error) bool {
	var protocolErr *textproto.Error
	return errors.As(err, &protocolErr) && protocolErr.Code >= 500 && protocolErr.Code <= 599
}
