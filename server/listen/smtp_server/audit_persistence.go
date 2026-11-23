package smtp_server

import (
	"encoding/json"
	stderrors "errors"
	"slices"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/services/forward"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/services/queue"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/context"
	smtp "github.com/emersion/go-smtp"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

const deliveryPendingError = "delivery result pending; outcome unknown after interruption"

type outgoingSender func(*context.Context, *parsemail.Email) (map[string]error, error)

func (s *Session) deliverOutgoingEmail(email *parsemail.Email, sender outgoingSender) error {
	ctx := s.Ctx
	// Submission records and local deliveries must account for the same
	// normalized message bytes that will be sent downstream, not a zero or
	// parser-only size.
	if email.MsgID == "" {
		email.MsgID = parsemail.GenerateMsgID(domain.DefaultName())
	}
	raw, err := email.BuildBytes(ctx, true)
	if err != nil {
		return &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 6, 0}, Message: "message headers or MIME serialization are invalid"}
	}
	email.Size = len(raw)
	_, saved, err := saveEmail(ctx, email.Size, email, ctx.UserID, int(consts.EmailTypeSend), nil, true, true)
	if err != nil {
		log.WithContext(ctx).Errorf("CRITICAL audit_event=initial_persist_failed direction=outbound msg_id=%q error=%v", email.MsgID, err)
		return localPersistenceSMTPError()
	}

	domainErrors, deliveryErr := sender(ctx, email)

	var errorText string
	if deliveryErr != nil {
		errorText = deliveryErr.Error()
	} else {
		for _, deliveryResult := range domainErrors {
			if deliveryResult != nil && !stderrors.Is(deliveryResult, queue.ErrQueued) {
				errorText = deliveryResult.Error()
				break
			}
		}
	}
	if errorText != "" {
		if _, err := db.Instance.Table(&models.Email{}).ID(saved.Id).Cols("error").Update(map[string]interface{}{"error": errorText}); err != nil {
			log.WithContext(ctx).Errorf("delivery error update failed: %v", err)
		}
	}
	if err := queue.ReconcileParent(saved.Id, ctx.UserID); err != nil {
		log.WithContext(ctx).Errorf("CRITICAL audit_event=delivery_result_persist_failed direction=outbound msg_id=%q error=%v", email.MsgID, err)
	}

	// If the remote MX accepted DATA, returning a local 451 here would cause a
	// duplicate delivery. The persistence failure is retained as pending state
	// and a CRITICAL log; the SMTP result must still reflect the remote result.
	return downstreamDeliverySMTPError(deliveryErr, domainErrors)
}

func quotaSMTPError() error {
	return &smtp.SMTPError{Code: 452, EnhancedCode: smtp.EnhancedCode{4, 2, 2}, Message: "Mailbox storage quota exceeded"}
}

func localPersistenceSMTPError() error {
	return &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 3, 0},
		Message:      "Temporary local persistence failure",
	}
}

func persistInitialAuditWithQuota(ctx *context.Context, modelEmail models.Email, userIDs []int, scores map[int]*inbound.Spam) (*models.Email, []int, error) {
	var saved models.Email
	userIDs = sortedOwners(userIDs)
	accepted := make([]int, 0, len(userIDs))
	err := db.Transaction(db.Instance, func(session *xorm.Session) error {
		accepted = accepted[:0]
		candidate := modelEmail
		candidate.Id = 0
		mailstore.Prepare(&candidate)
		mailstore.SetPOPSize(ctx, &candidate)
		for _, id := range userIDs {
			if err := quota.LockAndCheck(session, id, int64(candidate.Size)); err != nil {
				if stderrors.Is(err, quota.ErrExceeded) {
					log.WithContext(ctx).Warnf("quota exceeded for user %d; skipping recipient", id)
					continue
				}
				return err
			}
			accepted = append(accepted, id)
		}
		if len(accepted) == 0 {
			return quota.ErrExceeded
		}
		if _, err := session.Insert(&candidate); err != nil {
			return err
		}
		if err := mailstore.WriteParts(session, &candidate, false); err != nil {
			return err
		}
		if candidate.Id <= 0 {
			return stderrors.New("email insert returned no id")
		}
		for _, id := range accepted {
			link := initialLink(&candidate, id, 0, scores[id])
			if _, err := session.Insert(&link); err != nil {
				return err
			}
			if err := quota.Adjust(session, id, int64(candidate.Size)); err != nil {
				return err
			}
		}
		saved = candidate
		return nil
	})
	if err != nil {
		return nil, accepted, err
	}
	return &saved, accepted, nil
}

func persistInitialAudit(ctx *context.Context, modelEmail models.Email, userIDs []int) (*models.Email, error) {
	userIDs = sortedOwners(userIDs)
	var saved models.Email
	err := db.Transaction(db.Instance, func(session *xorm.Session) error {
		for _, id := range userIDs {
			if _, err := quota.Lock(session, id); err != nil {
				return err
			}
		}
		candidate := modelEmail
		candidate.Id = 0
		mailstore.Prepare(&candidate)
		mailstore.SetPOPSize(ctx, &candidate)
		if _, err := session.Insert(&candidate); err != nil {
			return err
		}
		if err := mailstore.WriteParts(session, &candidate, false); err != nil {
			return err
		}
		if candidate.Id <= 0 {
			return stderrors.New("email insert returned no id")
		}
		for _, userID := range userIDs {
			relation := models.UserEmail{EmailID: candidate.Id, UserID: userID, Status: candidate.Status}
			if _, err := session.Insert(&relation); err != nil {
				return err
			}
			if err := quota.Adjust(session, userID, int64(candidate.Size)); err != nil {
				return err
			}
		}
		saved = candidate
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func resolveIncomingUsers(ctx *context.Context, email *parsemail.Email, emailType int, reallyTo []string, SPFStatus, dkimStatus bool) ([]*models.User, bool, error) {
	if emailType != int(consts.EmailTypeReceive) {
		return nil, false, nil
	}
	recipients := incomingAddresses(email, reallyTo)
	seen := map[int]bool{}
	users := make([]*models.User, 0)
	unknown := false
	for _, raw := range recipients {
		_, host, ok := strings.Cut(strings.TrimSpace(raw), "@")
		if !ok || !domain.IsLocal(host) {
			continue
		}
		if srsUser, isSRS, srsErr := forward.ResolveSRS(raw); isSRS {
			if srsErr != nil {
				return nil, false, srsErr
			}
			if !seen[srsUser.ID] {
				copy := srsUser
				users = append(users, &copy)
				seen[srsUser.ID] = true
			}
			continue
		}
		user, found, err := address.Resolve(raw)
		if err != nil {
			return nil, false, err
		}
		if !found {
			unknown = true
			continue
		}
		if !seen[user.ID] {
			copy := user
			users = append(users, &copy)
			seen[user.ID] = true
		}
	}
	if !unknown || !config.Instance.CatchAll {
		return users, len(users) == 0, nil
	}
	if DropUnknownRecipientEmails && ((config.Instance.SpamFilterLevel == 1 && !SPFStatus && !dkimStatus) || (config.Instance.SpamFilterLevel == 2 && !SPFStatus) || (config.Instance.SpamFilterLevel == 3 && !dkimStatus)) {
		log.WithContext(ctx).Infof("unknown recipient rejected by catch-all spam policy: %v", recipients)
		return users, len(users) == 0, nil
	}
	var admins []*models.User
	if err := db.Instance.Table(&models.User{}).Where("is_admin=1 AND disabled=0").Find(&admins); err != nil {
		return nil, false, err
	}
	for _, admin := range admins {
		if !seen[admin.ID] {
			users = append(users, admin)
			seen[admin.ID] = true
		}
	}
	if len(users) == 0 {
		return nil, false, stderrors.New("no administrator available for unknown recipient")
	}
	return users, false, nil
}

func incomingAddresses(email *parsemail.Email, reallyTo []string) []string {
	if len(reallyTo) > 0 {
		return reallyTo
	}
	var result []string
	for _, list := range [][]*parsemail.User{email.To, email.Cc, email.Bcc} {
		for _, recipient := range list {
			if recipient != nil && recipient.EmailAddress != "" {
				result = append(result, recipient.EmailAddress)
			}
		}
	}
	return result
}

func boolInt8(value bool) int8 {
	if value {
		return 1
	}
	return 0
}

func json2string(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func sortedOwners(ids []int) []int {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	return slices.Compact(ids)
}
