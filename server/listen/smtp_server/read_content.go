package smtp_server

import (
	"bytes"
	stdctx "context"
	"crypto/sha256"
	"database/sql"
	oerrors "errors"
	"io"
	"net/netip"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/listen/imap_server"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/services/group"
	"github.com/Jinnrry/pmail/services/inboundauth"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/services/rule"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	"github.com/Jinnrry/pmail/utils/send"
	smtp "github.com/emersion/go-smtp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"xorm.io/xorm"
)

// DropUnknownRecipientEmails gates SPF/DKIM filtering before catch-all
// delivery. Known recipients are returned before this policy is consulted;
// the actual filter level is controlled by PMAIL_SPAM_FILTER_LEVEL.
const DropUnknownRecipientEmails = true

// ImportExternal commits the message, its owner link and the caller's remote
// deduplication marker together. Rule side effects happen only after commit.
func ImportExternal(ctx *context.Context, raw []byte, recipient string, targetGroupID int, seen func(*xorm.Session) (bool, error)) (*models.Email, error) {
	if ctx.UserID <= 0 || len(raw) == 0 || strings.TrimSpace(recipient) == "" {
		return nil, oerrors.New("external message or recipient is missing")
	}
	email, err := parsemail.ParseLimited([]string{recipient}, bytes.NewReader(raw), len(raw))
	if err != nil {
		return nil, err
	}
	if email.From == nil || email.From.EmailAddress == "" {
		return nil, oerrors.New("external sender is missing")
	}
	email.Authentication = parsemail.NewEmailAuthentication(false, false)
	email.AuthUnchecked = true
	if email.MsgID == "" {
		email.MsgID = parsemail.GenerateMsgID(domain.DefaultName())
	}
	email.Size = len(raw)
	var saved *models.Email
	var owner models.User
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		saved = nil
		if ctx.Context != nil {
			tx.Context(ctx.Context)
		}
		duplicate, err := seen(tx)
		if err != nil || duplicate {
			return err
		}
		if err = quota.LockAndCheck(tx, ctx.UserID, int64(email.Size)); err != nil {
			return err
		}
		found, err := tx.ID(ctx.UserID).Get(&owner)
		if err != nil {
			return err
		}
		if !found {
			return address.ErrNotFound
		}
		if targetGroupID != 0 {
			ok, err := tx.ID(targetGroupID).Where("user_id=?", ctx.UserID).Exist(&models.Group{})
			if err != nil {
				return err
			}
			if !ok {
				return group.ErrInvalidParent
			}
		}
		row := storedEmail(email, 0, int(consts.EmailTypeReceive), email.MsgID, false, false)
		if _, err = tx.Insert(&row); err != nil {
			return err
		}
		if _, err = tx.Insert(&models.UserEmail{UserID: ctx.UserID, EmailID: row.Id, GroupId: targetGroupID}); err != nil {
			return err
		}
		saved = &row
		return nil
	})
	if err != nil || saved == nil {
		return saved, err
	}
	email.MessageId = int64(saved.Id)
	processIncomingDelivery(ctx, email, saved, []*models.User{&owner}, raw)
	return saved, nil
}

// LocalDelivery is shared by SMTP submission and Web send to keep local mail
// on the same persistence/rule/hook path as SMTP 25 receive.
func LocalDelivery(ctx *context.Context, email *parsemail.Email, recipients []*parsemail.User) (error, map[string]error) {
	var first error
	errs := map[string]error{}
	accepted := 0
	raw := email.RawData
	if len(raw) == 0 {
		var err error
		raw, err = email.BuildBytes(ctx, false)
		if err != nil {
			return err, errs
		}
	}
	for _, recipient := range recipients {
		// saveEmail assigns MessageId/MsgID on the parsed email. Use a per-
		// recipient copy so local persistence cannot mutate the sender's parent
		// audit email that outbound queue rows reference.
		localEmail := *email
		users, saved, err := saveEmail(ctx, len(raw), &localEmail, 0, int(consts.EmailTypeReceive), []string{recipient.EmailAddress}, true, true)
		if err == nil && len(users) == 0 && !config.Instance.CatchAll {
			err = address.ErrNotFound
		}
		if err != nil {
			errs[recipient.EmailAddress] = err
			if first == nil {
				first = err
			}
			continue
		}
		accepted++
		processIncomingDelivery(ctx, &localEmail, saved, users, raw)
	}
	// SMTP DATA cannot report a separate result for each RCPT. Return a
	// recipient error only when every local recipient was rejected; partial
	// local delivery remains accepted and is reported through the per-address
	// error map instead of making accepted recipients retry.
	if accepted > 0 {
		return nil, errs
	}
	return first, errs
}

func processIncomingDelivery(ctx *context.Context, email *parsemail.Email, dbEmail *models.Email, users []*models.User, emailData []byte) {
	if dbEmail == nil || email == nil {
		return
	}
	log.WithContext(ctx).Debugf("开始执行邮件规则！")
	for _, user := range users {
		for _, ruleItem := range rule.GetAllRules(ctx, user.ID) {
			if rule.MatchRule(ctx, ruleItem, email) {
				rule.DoRule(ctx, ruleItem, email, user, emailData)
			}
		}
		// Rules run first. Apply skips users whose rule moved the source to
		// Deleted, and therefore does not forward mail deleted by a rule.
		forward.Apply(ctx, user.ID, email, emailData)
	}

	for _, user := range users {
		_ = imap_server.IdleNotice(ctx, user.ID, dbEmail)
	}
}

func (s *Session) Data(r io.Reader) error {
	if err := s.validateSubmission(); err != nil {
		return err
	}

	ctx := s.Ctx

	log.WithContext(ctx).Debugf("收到邮件")

	emailData, err := mailbudget.Read(r, mailbudget.Message)
	if oerrors.Is(err, mailbudget.ErrLimit) {
		return sizeSMTPError()
	}
	if err != nil {
		log.WithContext(ctx).Error("邮件内容无法读取", err)
		return err
	}

	log.WithContext(ctx).Debugf("smtp data received bytes=%d sha256=%x", len(emailData), sha256.Sum256(emailData))

	email, err := parsemail.ParseLimited(s.To, bytes.NewReader(emailData), len(emailData))
	if oerrors.Is(err, mailbudget.ErrLimit) {
		return sizeSMTPError()
	}
	if err != nil {
		return err
	}
	email.EnvelopeFrom = s.From
	email.EnvelopeFromKnown = true
	email.RawData = append([]byte(nil), emailData...)

	if s.From != "" {
		from := parsemail.BuilderUser(s.From)
		if email.From == nil {
			email.From = from
		}
		if email.From.EmailAddress != from.EmailAddress {
			// 协议中的from和邮件内容中的from不匹配，当成垃圾邮件处理
			//log.WithContext(s.Ctx).Infof("垃圾邮件，拒信")
			//return nil
		}
	}

	// 判断是收信还是转发，只要是登陆了，都当成转发处理
	if s.Ctx.UserID > 0 {
		if email.From == nil {
			return &smtp.SMTPError{Code: 550, Message: "missing sender"}
		}
		if !ctx.IsAdmin {
			if _, err := address.SendAddress(ctx.UserID, email.From.EmailAddress); err != nil {
				return oerrors.New("No Auth")
			}
		}

		return s.deliverOutgoingEmail(email, send.Send)

	} else {
		var peer netip.Addr
		if s.RemoteAddress != nil {
			if endpoint, err := netip.ParseAddrPort(s.RemoteAddress.String()); err == nil {
				peer = endpoint.Addr()
			}
		}
		verify := s.AuthenticateInbound
		if verify == nil {
			verify = inboundauth.Verify
		}
		hello := s.HELO
		if s.currentHELO != nil {
			hello = s.currentHELO()
		}
		evidence := verify(stdctx.Background(), peer, s.From, hello, emailData)
		email.AuthResults = &evidence
		SPFStatus, dkimStatus := evidence.SPF.Aligned, evidence.DKIM.Aligned
		spoofed := false
		if email.From != nil {
			_, host := email.From.GetDomainAccount()
			spoofed = domain.IsLocal(host) && evidence.Status() != "pass" && evidence.DMARC.Result != "temperror" && evidence.SPF.Result != "temperror" && evidence.DKIM.Result != "temperror"
		}
		email.Authentication = parsemail.NewEmailAuthentication(SPFStatus, dkimStatus)

		if spoofed {
			email.Status = 3
		}

		users, dbEmail, saveErr := saveEmail(ctx, len(emailData), email, 0, 0, s.To, SPFStatus, dkimStatus)
		if saveErr != nil {
			if oerrors.Is(saveErr, quota.ErrExceeded) {
				return quotaSMTPError()
			}
			if oerrors.Is(saveErr, forward.ErrInvalidSRS) {
				return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "Invalid SRS recipient"}
			}
			log.WithContext(ctx).Errorf("CRITICAL audit_event=initial_persist_failed direction=inbound msg_id=%q error=%v", email.MsgID, saveErr)
			return localPersistenceSMTPError()
		}

		processIncomingDelivery(ctx, email, dbEmail, users, emailData)

	}

	return nil
}

func saveEmail(ctx *context.Context, size int, email *parsemail.Email, sendUserID int, emailType int, reallyTo []string, SPFStatus, dkimStatus bool) ([]*models.User, *models.Email, error) {
	if email == nil {
		return nil, nil, oerrors.New("email is missing")
	}
	if email.From == nil {
		return nil, nil, oerrors.New("email sender is missing")
	}
	if emailType == int(consts.EmailTypeReceive) && email.Authentication != nil {
		SPFStatus = email.Authentication.SPFPassed
		dkimStatus = email.Authentication.DKIMPassed
	}

	users, drop, err := resolveIncomingUsers(ctx, email, emailType, reallyTo, SPFStatus, dkimStatus)
	if err != nil || drop {
		return users, nil, err
	}

	msgID := email.MsgID
	if msgID == "" {
		msgID = parsemail.GenerateMsgID(domain.DefaultName())
	}
	if email.Size == 0 {
		email.Size = size
	}

	modelEmail := storedEmail(email, sendUserID, emailType, msgID, SPFStatus, dkimStatus)

	var userIDs []int
	if emailType == int(consts.EmailTypeReceive) {
		for _, user := range users {
			userIDs = append(userIDs, user.ID)
		}
	} else {
		modelEmail.Status = consts.EmailStatusDeliveryPending
		modelEmail.Error = sql.NullString{String: deliveryPendingError, Valid: true}
		userIDs = []int{sendUserID}
	}

	log.WithContext(ctx).Debugf("开始入库！")
	var saved *models.Email
	var accepted []int
	if emailType == int(consts.EmailTypeReceive) {
		var err error
		saved, accepted, err = persistInitialAuditWithQuota(ctx, modelEmail, userIDs)
		if err != nil {
			return users, nil, err
		}
		if len(accepted) == 0 {
			return users, nil, quota.ErrExceeded
		}
		allowed := map[int]bool{}
		for _, id := range accepted {
			allowed[id] = true
		}
		filtered := users[:0]
		for _, u := range users {
			if allowed[u.ID] {
				filtered = append(filtered, u)
			}
		}
		users = filtered
	} else {
		var err error
		saved, err = persistInitialAudit(ctx, modelEmail, userIDs)
		if err != nil {
			return users, nil, err
		}
	}
	email.MessageId = cast.ToInt64(saved.Id)
	email.MsgID = saved.MsgID
	return users, saved, nil
}

func storedEmail(email *parsemail.Email, sendUserID, emailType int, msgID string, SPFStatus, dkimStatus bool) models.Email {
	return models.Email{
		Type:          cast.ToInt8(emailType),
		Subject:       email.Subject,
		ReplyTo:       json2string(email.ReplyTo),
		FromName:      email.From.Name,
		FromAddress:   email.From.EmailAddress,
		To:            json2string(email.To),
		Bcc:           json2string(email.Bcc),
		Cc:            json2string(email.Cc),
		Text:          sql.NullString{String: string(email.Text), Valid: true},
		TextFormat:    email.TextFormat,
		Html:          sql.NullString{String: string(email.HTML), Valid: true},
		Sender:        json2string(email.Sender),
		Attachments:   json2string(email.Attachments),
		Size:          email.Size,
		SPFCheck:      boolInt8(SPFStatus),
		DKIMCheck:     boolInt8(dkimStatus),
		AuthUnchecked: email.AuthUnchecked,
		AuthResults:   sql.NullString{String: email.AuthResults.Storage(), Valid: email.AuthResults != nil},
		SendUserID:    sendUserID,
		SendDate:      time.Now(),
		Status:        cast.ToInt8(email.Status),
		CreateTime:    time.Now(),
		CronSendTime:  time.Now(),
		MsgID:         msgID,
	}

}

func sizeSMTPError() error {
	return &smtp.SMTPError{Code: 552, EnhancedCode: smtp.EnhancedCode{5, 3, 4}, Message: "message exceeds configured resource limit"}
}
