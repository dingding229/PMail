package smtp_server

import (
	"bytes"
	"database/sql"
	oerrors "errors"
	"io"
	"net"
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
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/services/rule"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/send"
	smtp "github.com/emersion/go-smtp"
	"github.com/mileusna/spf"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
)

// DropUnknownRecipientEmails gates SPF/DKIM filtering before catch-all
// delivery. Known recipients are returned before this policy is consulted;
// the actual filter level is controlled by PMAIL_SPAM_FILTER_LEVEL.
const DropUnknownRecipientEmails = true

// ImportExternal persists a message fetched from an external mailbox through
// the same receive/rule/forward/hook path as SMTP, without SPF enforcement.
func ImportExternal(ctx *context.Context, raw []byte, recipient string, targetGroupID int) (*models.Email, error) {
	if len(raw) == 0 || strings.TrimSpace(recipient) == "" {
		return nil, oerrors.New("external message or recipient is missing")
	}
	email := parsemail.NewEmailFromReader([]string{recipient}, bytes.NewReader(raw), len(raw))
	if email.MsgID != "" {
		var existing models.Email
		if found, err := db.Instance.Where("msg_id=?", email.MsgID).Get(&existing); err != nil {
			return nil, err
		} else if found {
			return &existing, nil
		}
	}
	email.Authentication = parsemail.NewEmailAuthentication(false, false)
	users, saved, err := saveEmail(ctx, len(raw), email, 0, int(consts.EmailTypeReceive), []string{recipient}, false, false)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 || saved == nil {
		return nil, oerrors.New("external recipient was not resolved")
	}
	if targetGroupID != 0 {
		if _, err := db.Instance.Table(&models.UserEmail{}).Where("email_id=? AND user_id=?", saved.Id, users[0].ID).Cols("group_id").Update(map[string]interface{}{"group_id": targetGroupID}); err != nil {
			return nil, err
		}
	}
	processIncomingDelivery(ctx, email, saved, users, raw)
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
		raw = email.BuildBytes(ctx, false)
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

	ctx := s.Ctx

	log.WithContext(ctx).Debugf("收到邮件")

	emailData, err := io.ReadAll(r)
	if err != nil {
		log.WithContext(ctx).Error("邮件内容无法读取", err)
		return err
	}

	log.WithContext(ctx).Debugf("%s", string(emailData))

	email := parsemail.NewEmailFromReader(s.To, bytes.NewReader(emailData), len(emailData))
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
		if !ctx.IsAdmin {
			if _, err := address.SendAddress(ctx.UserID, email.From.EmailAddress); err != nil {
				return oerrors.New("No Auth")
			}
		}

		return s.deliverOutgoingEmail(email, send.Send)

	} else {
		// 收件
		var dkimStatus, SPFStatus bool

		// DKIM校验
		dkimStatus = parsemail.Check(ctx, bytes.NewReader(emailData))

		SPFStatus = spfCheck(s.RemoteAddress.String(), email.Sender, email.Sender.EmailAddress)

		_, formDomain := email.From.GetDomainAccount()
		spoofed := domain.IsLocal(formDomain) && SPFStatus == false
		if spoofed {
			dkimStatus = false
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

	modelEmail := models.Email{
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
		SendUserID:   sendUserID,
		SendDate:     time.Now(),
		Status:       cast.ToInt8(email.Status),
		CreateTime:   time.Now(),
		CronSendTime: time.Now(),
		MsgID:        msgID,
	}

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

func spfCheck(remoteAddress string, sender *parsemail.User, senderString string) bool {
	//spf校验
	ipAddress, _ := netip.ParseAddrPort(remoteAddress)

	ip := net.ParseIP(ipAddress.Addr().String())
	if ip.IsPrivate() {
		return true
	}

	tmp := strings.Split(sender.EmailAddress, "@")
	if len(tmp) < 2 {
		return false
	}

	res := spf.CheckHost(ip, tmp[1], senderString, "")

	if res == spf.None || res == spf.Pass {
		// spf校验通过
		return true
	}
	return false
}
