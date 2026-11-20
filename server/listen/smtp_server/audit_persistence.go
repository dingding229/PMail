package smtp_server

import (
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/hooks"
	"github.com/Jinnrry/pmail/hooks/framework"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/Jinnrry/pmail/utils/async"
	"github.com/Jinnrry/pmail/utils/context"
	smtp "github.com/emersion/go-smtp"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

const deliveryPendingError = "delivery result pending; outcome unknown after interruption"

type outgoingSender func(*context.Context, *parsemail.Email) (error, map[string]error)

func (s *Session) deliverOutgoingEmail(email *parsemail.Email, sender outgoingSender) error {
	ctx := s.Ctx
	// Submission records and local deliveries must account for the same
	// normalized message bytes that will be sent downstream, not a zero or
	// parser-only size.
	email.Size = len(email.BuildBytes(ctx, true))
	_, saved, err := saveEmail(ctx, email.Size, email, ctx.UserID, int(consts.EmailTypeSend), nil, true, true)
	if err != nil {
		log.WithContext(ctx).Errorf("CRITICAL audit_event=initial_persist_failed direction=outbound msg_id=%q error=%v", email.MsgID, err)
		return localPersistenceSMTPError()
	}

	deliveryErr, domainErrors := sender(ctx, email)
	runSendAfterHooks(ctx, email, domainErrors)

	targetStatus := consts.EmailStatusSent
	var errorText any
	if deliveryErr != nil {
		targetStatus = consts.EmailStatusFail
		errorText = deliveryErr.Error()
	}

	if err = persistDeliveryResult(ctx, saved.Id, targetStatus, errorText); err != nil {
		log.WithContext(ctx).Errorf(
			"CRITICAL audit_event=delivery_result_persist_failed email_id=%d msg_id=%q target_status=%d downstream_accepted=%t error=%v",
			saved.Id, saved.MsgID, targetStatus, deliveryErr == nil, err,
		)
	}

	// If the remote MX accepted DATA, returning a local 451 here would cause a
	// duplicate delivery. The persistence failure is retained as pending state
	// and a CRITICAL log; the SMTP result must still reflect the remote result.
	return downstreamDeliverySMTPError(deliveryErr, domainErrors)
}

func runSendAfterHooks(ctx *context.Context, email *parsemail.Email, domainErrors map[string]error) {
	process := async.New(ctx)
	for _, hook := range hooks.AllHooks() {
		if hook == nil {
			continue
		}
		process.WaitProcess(func(value any) {
			value.(framework.EmailHook).SendAfter(ctx, email, domainErrors)
		}, hook)
	}
	process.Wait()
}

func persistDeliveryResult(ctx *context.Context, emailID int, status int8, errorText any) error {
	return db.Transaction(db.Instance, func(session *xorm.Session) error {
		result, err := session.Exec(db.WithContext(ctx, `update email
			set status=?, error=?, update_time=CURRENT_TIMESTAMP
			where id=?`), status, errorText, emailID)
		if err != nil {
			return err
		}
		if err = requireOneAffected(result, "update email delivery result"); err != nil {
			return err
		}

		result, err = session.Exec(db.WithContext(ctx, `update user_email
			set status=? where email_id=?`), status, emailID)
		if err != nil {
			return err
		}
		return requireOneAffected(result, "update user_email delivery result")
	})
}

func requireOneAffected(result sql.Result, operation string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: read affected rows: %w", operation, err)
	}
	if affected != 1 {
		return fmt.Errorf("%s: affected %d rows, want 1", operation, affected)
	}
	return nil
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

func persistInitialAuditWithQuota(ctx *context.Context, modelEmail models.Email, userIDs []int) (*models.Email, []int, error) {
	var saved models.Email
	accepted := make([]int, 0, len(userIDs))
	err := db.Transaction(db.Instance, func(session *xorm.Session) error {
		candidate := modelEmail
		candidate.Id = 0
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
		if candidate.Id <= 0 {
			return stderrors.New("email insert returned no id")
		}
		for _, id := range accepted {
			if _, err := session.Insert(&models.UserEmail{EmailID: candidate.Id, UserID: id, Status: candidate.Status}); err != nil {
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
	var saved models.Email
	err := db.Transaction(db.Instance, func(session *xorm.Session) error {
		candidate := modelEmail
		candidate.Id = 0
		if _, err := session.Insert(&candidate); err != nil {
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
	for _, raw := range recipients {
		user, found, err := address.Resolve(raw)
		if err != nil {
			return nil, false, err
		}
		if found && !seen[user.ID] {
			copy := user
			users = append(users, &copy)
			seen[user.ID] = true
		}
	}
	if len(users) > 0 {
		return users, false, nil
	}
	if !config.Instance.CatchAll {
		return nil, true, nil
	}
	if DropUnknownRecipientEmails && ((config.Instance.SpamFilterLevel == 1 && !SPFStatus && !dkimStatus) || (config.Instance.SpamFilterLevel == 2 && !SPFStatus) || (config.Instance.SpamFilterLevel == 3 && !dkimStatus)) {
		log.WithContext(ctx).Infof("unknown recipient rejected by catch-all spam policy: %v", recipients)
		return nil, true, nil
	}
	if err := db.Instance.Table(&models.User{}).Where("is_admin=1 AND disabled=0").Find(&users); err != nil {
		return nil, false, err
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
