package smtp_server

import (
	"bytes"
	stdctx "context"
	"crypto/sha256"
	"database/sql"
	oerrors "errors"
	"io"
	"net/netip"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/listen/imap_server"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/antivirus"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/inboundauth"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/services/rule"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	"github.com/Jinnrry/pmail/utils/send"
	smtp "github.com/emersion/go-smtp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

// DropUnknownRecipientEmails gates SPF/DKIM filtering before catch-all
// delivery. Known recipients are returned before this policy is consulted;
// the actual filter level is controlled by PMAIL_SPAM_FILTER_LEVEL.
const DropUnknownRecipientEmails = true

type inboundDelivery struct {
	prepared  bool
	antivirus *antivirus.Result
	envelope  inbound.Envelope
	analysis  *inbound.Analysis
	scores    map[int]*inbound.Spam
}

func (d *inboundDelivery) analyze(ctx *context.Context, email *parsemail.Email, owners []int) {
	if d.prepared {
		return
	}
	d.prepared = true
	d.analysis, d.scores = inbound.AnalyzeDelivery(ctx.Context, email, d.envelope, email.AuthResults, owners)
	if d.antivirus != nil {
		// Persist the scan status even if advisory risk analysis is busy or times out.
		if d.analysis == nil {
			d.analysis = &inbound.Analysis{Version: 1}
		}
		d.analysis.Antivirus = d.antivirus
	}
}
func (d *inboundDelivery) recipient(uid int) *inbound.Analysis {
	if d == nil || d.analysis == nil {
		return nil
	}
	a := *d.analysis
	a.Recipient = d.scores[uid]
	return &a
}
func initialLink(row *models.Email, uid, groupID int, score *inbound.Spam) models.UserEmail {
	link := models.UserEmail{UserID: uid, EmailID: row.Id, GroupId: groupID, Status: row.Status, InboundSpam: inbound.Storage(score)}
	if link.Status == consts.EmailStatusWait && score != nil && score.Verdict == "spam" {
		link.Status = consts.EmailStatusJunk
		link.GroupId = 0
	}
	return link
}

// LocalDelivery is shared by SMTP submission and Web send to keep local mail
// on the same persistence/rule/hook path as SMTP 25 receive.
func LocalDelivery(ctx *context.Context, email *parsemail.Email, recipients []*parsemail.User) (map[string]error, error) {
	var first error
	errs := map[string]error{}
	if len(recipients) == 0 {
		return errs, nil
	}
	accepted := 0
	raw := email.RawData
	if len(raw) == 0 {
		var err error
		raw, err = email.BuildBytes(ctx, false)
		if err != nil {
			return errs, err
		}
	}
	// Group one local submission by recipient owner, not by untrusted Message-ID.
	// A single owner who receives through two aliases gets one copy and two links.
	delivery := inboundDelivery{envelope: inbound.Envelope{MailFrom: email.EnvelopeFrom}}
	var owners []int
	byOwner := map[int][]string{}
	for _, recipient := range recipients {
		if recipient == nil {
			continue
		}
		rcpt := recipient.EmailAddress
		users, _, _, err := resolveIncomingUsers(ctx, email, int(consts.EmailTypeReceive), []string{rcpt}, true, true)
		if err == nil && len(users) == 0 {
			err = address.ErrNotFound
		}
		if err != nil {
			errs[rcpt] = err
			if first == nil {
				first = err
			}
			continue
		}
		owner := users[0].ID
		if _, exists := byOwner[owner]; !exists {
			owners = append(owners, owner)
		}
		byOwner[owner] = append(byOwner[owner], rcpt)
	}
	delivery.analyze(ctx, email, owners)
	for _, owner := range owners {
		targets := byOwner[owner]
		localEmail := *email // do not mutate the outbound audit parent
		users, saved, err := saveEmail(ctx, len(raw), &localEmail, 0, int(consts.EmailTypeReceive), targets, true, true, &delivery)
		if err == nil && len(users) == 0 {
			err = address.ErrNotFound
		}
		if err != nil {
			for _, rcpt := range targets {
				errs[rcpt] = err
			}
			if first == nil {
				first = err
			}
			continue
		}
		accepted++
		processIncomingDelivery(ctx, &localEmail, saved, users, raw, &delivery)
	}
	// SMTP DATA cannot report a separate result for each RCPT. Return a
	// recipient error only when every local recipient was rejected; partial
	// local delivery remains accepted and is reported through the per-address
	// error map instead of making accepted recipients retry.
	if accepted > 0 {
		return errs, nil
	}
	return errs, first
}

func processIncomingDelivery(ctx *context.Context, email *parsemail.Email, dbEmail *models.Email, users []*models.User, emailData []byte, delivery ...*inboundDelivery) {
	if dbEmail == nil || email == nil {
		return
	}
	log.WithContext(ctx).Debugf("开始执行邮件规则！")
	var info *inboundDelivery
	if len(delivery) > 0 {
		info = delivery[0]
	}
	for _, user := range users {
		result := info.recipient(user.ID)
		// Junk is not auto-forwarded or moved out by ordinary mailbox rules.
		// Keep the existing spoofed/deleted path separate from the new classifier.
		if dbEmail.Status == consts.EmailStatusWait && result != nil && result.Recipient != nil && result.Recipient.Verdict == "spam" {
			continue
		}
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
		inbound.Notify(user.ID, dbEmail.Id, info.recipient(user.ID))
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

	var scanResult *antivirus.Result
	if ctx.UserID <= 0 {
		scan := s.scanInbound
		if scan == nil {
			if m := antivirus.Active(); m != nil {
				scan = m.Scan
			}
		}
		if scan != nil {
			scanContext := ctx.Context
			if scanContext == nil {
				scanContext = stdctx.Background()
			}
			scanResult = scan(scanContext, emailData)
			if scanResult != nil && scanResult.Status == "infected" {
				// Rejected DATA has no mailbox row. Use the existing structured inbound audit log.
				// The installed log formatter writes Message, not logrus fields.
				log.WithContext(ctx).Warnf("audit_event=inbound_virus_rejected direction=inbound virus=%q mail_from=%q rcpt_to=%q", scanResult.Virus, s.From, s.To)
				return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 7, 1}, Message: "Message rejected: malware detected"}
			}
		}
	}

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
				return oerrors.New("sender address is not authorized")
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

		delivery := inboundDelivery{antivirus: scanResult, envelope: inbound.Envelope{SourceIP: inbound.SourceIP(peer, emailData), MailFrom: s.From}}
		users, dbEmail, saveErr := saveEmail(ctx, len(emailData), email, 0, 0, s.To, SPFStatus, dkimStatus, &delivery)
		if saveErr != nil {
			if oerrors.Is(saveErr, quota.ErrExceeded) {
				return quotaSMTPError()
			}
			if oerrors.Is(saveErr, address.ErrNotFound) {
				return unknownRecipientSMTPError()
			}
			if oerrors.Is(saveErr, forward.ErrInvalidSRS) {
				return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "Invalid SRS recipient"}
			}
			log.WithContext(ctx).Errorf("CRITICAL audit_event=initial_persist_failed direction=inbound msg_id=%q error=%v", email.MsgID, saveErr)
			return localPersistenceSMTPError()
		}

		processIncomingDelivery(ctx, email, dbEmail, users, emailData, &delivery)

	}

	return nil
}

func saveEmail(ctx *context.Context, size int, email *parsemail.Email, sendUserID int, emailType int, reallyTo []string, SPFStatus, dkimStatus bool, delivery ...*inboundDelivery) ([]*models.User, *models.Email, error) {
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

	users, catchAll, drop, err := resolveIncomingUsers(ctx, email, emailType, reallyTo, SPFStatus, dkimStatus)
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
		var scores map[int]*inbound.Spam
		if len(delivery) > 0 && delivery[0] != nil {
			delivery[0].analyze(ctx, email, userIDs)
			modelEmail.InboundAnalysis = inbound.Storage(delivery[0].analysis)
			scores = delivery[0].scores
		}
		saved, accepted, err = persistInitialAuditWithQuota(ctx, modelEmail, userIDs, scores, catchAll, reallyTo)
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
		Type:         cast.ToInt8(emailType),
		Subject:      email.Subject,
		ReplyTo:      json2string(email.ReplyTo),
		FromName:     email.From.Name,
		FromAddress:  email.From.EmailAddress,
		To:           json2string(email.To),
		Bcc:          json2string(email.Bcc),
		Cc:           json2string(email.Cc),
		Text:         sql.NullString{String: string(email.Text), Valid: true},
		TextFormat:   email.TextFormat,
		Html:         sql.NullString{String: string(email.HTML), Valid: true},
		Sender:       json2string(email.Sender),
		Attachments:  json2string(email.Attachments),
		Size:         email.Size,
		SPFCheck:     boolInt8(SPFStatus),
		DKIMCheck:    boolInt8(dkimStatus),
		AuthResults:  sql.NullString{String: email.AuthResults.Storage(), Valid: email.AuthResults != nil},
		SendUserID:   sendUserID,
		SendDate:     time.Now(),
		Status:       cast.ToInt8(email.Status),
		CreateTime:   time.Now(),
		CronSendTime: time.Now(),
		MsgID:        msgID,
	}

}

func sizeSMTPError() error {
	return &smtp.SMTPError{Code: 552, EnhancedCode: smtp.EnhancedCode{5, 3, 4}, Message: "message exceeds configured resource limit"}
}
