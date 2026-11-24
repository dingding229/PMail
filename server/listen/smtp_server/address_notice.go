package smtp_server

import (
	"context"
	"net/textproto"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	pctx "github.com/Jinnrry/pmail/utils/context"
)

// Use the same local mailbox storage and quota transaction as inbound mail,
// pinned to the applicant ID. Do not resolve recipients, run forwarding/rules,
// or fall back to catch-all; a review result is private to the applicant.
func DeliverAddressReview(ctx context.Context, reviewed address.View) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var owner models.User
	ok, err := db.Instance.Context(ctx).ID(reviewed.UserID).Where("disabled=0").Cols("lang").Get(&owner)
	if err != nil {
		return err
	}
	if !ok {
		return address.ErrNotFound
	}
	primary, err := address.Primary(reviewed.UserID)
	if err != nil {
		return err
	}
	locale := &pctx.Context{Context: ctx, UserID: reviewed.UserID, Lang: owner.Lang}
	action := "address_notice_approved"
	if reviewed.Status == models.AddressRejected {
		action = "address_notice_rejected"
	}
	text := i18n.T(locale, action, reviewed.Address)
	if reviewed.RejectReason != "" {
		text += "\n\n" + i18n.T(locale, "address_notice_reason", reviewed.RejectReason)
	}
	msg := &parsemail.Email{
		From:    &parsemail.User{EmailAddress: "mailer-daemon@" + primary.Domain},
		To:      []*parsemail.User{{EmailAddress: primary.Address}},
		Subject: i18n.T(locale, "address_notice_subject"),
		Text:    []byte(text),
		Headers: textproto.MIMEHeader{"Auto-Submitted": {"auto-generated"}},
	}
	msg.MsgID = parsemail.GenerateMsgID(primary.Domain)
	wire, err := msg.BuildBytes(locale, false)
	if err != nil {
		return err
	}
	msg.Size = len(wire)
	row := storedEmail(msg, 0, int(consts.EmailTypeReceive), msg.MsgID, true, true)
	_, _, err = persistInitialAuditWithQuota(locale, row, []int{reviewed.UserID}, nil, nil, []string{primary.Address})
	return err
}
