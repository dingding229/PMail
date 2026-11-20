package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	pcontext "github.com/Jinnrry/pmail/utils/context"
	smtp "github.com/emersion/go-smtp"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

const (
	KindSend    = "send"
	KindForward = "forward"
	KindBounce  = "bounce"

	StatusPending = "pending"
	StatusSending = "sending"
	StatusSent    = "sent"
	StatusFailed  = "failed"
)

// ErrQueued is returned in the per-recipient result map for an accepted queue item.
var ErrQueued = errors.New("delivery queued")
var ErrNotFound = errors.New("queue item not found")
var ErrLocalUnavailable = errors.New("local delivery handler is unavailable")

var retryDelays = [...]time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	time.Hour,
	2 * time.Hour,
	4 * time.Hour,
	8 * time.Hour,
}

// ExternalDelivery is intentionally a callback to keep the queue independent
// from the SMTP client package (which enqueues outbound messages).
type ExternalDelivery func(*pcontext.Context, string, []byte, string) error
type BounceHandler func(*pcontext.Context, models.OutboundQueue)
type LocalDeliveryHandler func(*pcontext.Context, *parsemail.Email, *parsemail.User) error

var handlers struct {
	sync.RWMutex
	external ExternalDelivery
	bounce   BounceHandler
	local    LocalDeliveryHandler
}

func SetExternalDelivery(fn ExternalDelivery) {
	handlers.Lock()
	handlers.external = fn
	handlers.Unlock()
}
func SetBounceHandler(fn BounceHandler) { handlers.Lock(); handlers.bounce = fn; handlers.Unlock() }
func SetLocalDelivery(fn LocalDeliveryHandler) {
	handlers.Lock()
	handlers.local = fn
	handlers.Unlock()
}

// ShouldSuppressBounce prevents DSNs from generating another DSN.
func ShouldSuppressBounce(row models.OutboundQueue) bool {
	if strings.TrimSpace(row.MailFrom) == "" || isDaemonEnvelope(row.MailFrom) {
		return true
	}
	return parsemail.IsAutomaticMessage(row.RawData)
}

func isDaemonEnvelope(value string) bool {
	value = strings.Trim(strings.TrimSpace(value), "<>")
	if value == "" {
		return true
	}
	local, _, ok := strings.Cut(value, "@")
	if !ok && strings.HasPrefix(strings.ToUpper(local), "SRS0=") {
		return true
	}
	if strings.HasPrefix(strings.ToUpper(local), "SRS0=") {
		parts := strings.Split(local, "=")
		if len(parts) >= 5 {
			local = parts[len(parts)-1]
		}
	}
	return strings.EqualFold(local, "mailer-daemon") || strings.EqualFold(local, "postmaster")
}

// ReserveBounceNotice atomically records the 24-hour forwarding failure notice window.
func bounceNoticeRecent(created, now time.Time) bool {
	return created.After(now.Add(-24 * time.Hour))
}

func ReserveBounceNotice(userID int, rcptTo string) (bool, error) {
	if userID <= 0 || strings.TrimSpace(rcptTo) == "" {
		return false, nil
	}
	now := time.Now()
	key := fmt.Sprintf("%d:%s", userID, strings.ToLower(strings.TrimSpace(rcptTo)))
	var notice models.BounceNotice
	found, err := db.Instance.Where("notice_key=?", key).Get(&notice)
	if err != nil {
		return false, err
	}
	if found {
		if bounceNoticeRecent(notice.Created, now) {
			return false, nil
		}
		_, err = db.Instance.Table(&models.BounceNotice{}).ID(notice.ID).Cols("created").Update(&models.BounceNotice{Created: now})
		return err == nil, err
	}
	_, err = db.Instance.Insert(&models.BounceNotice{
		NoticeKey: key,
		UserID:    userID,
		RcptTo:    strings.TrimSpace(rcptTo),
		Created:   now,
	})
	if err == nil {
		return true, nil
	}
	// Another worker may have won the unique-key race. Re-read it and apply
	// the same window decision rather than emitting a duplicate notice.
	notice = models.BounceNotice{}
	found, readErr := db.Instance.Where("notice_key=?", key).Get(&notice)
	if readErr != nil {
		return false, readErr
	}
	if found {
		if bounceNoticeRecent(notice.Created, now) {
			return false, nil
		}
		_, updateErr := db.Instance.Table(&models.BounceNotice{}).ID(notice.ID).Cols("created").Update(&models.BounceNotice{Created: now})
		return updateErr == nil, updateErr
	}
	return false, err
}

func Enqueue(kind string, userID, emailID int, mailFrom, rcptTo string, raw []byte) (models.OutboundQueue, error) {
	if kind != KindSend && kind != KindForward && kind != KindBounce {
		return models.OutboundQueue{}, fmt.Errorf("invalid queue kind %q", kind)
	}
	if strings.TrimSpace(rcptTo) == "" {
		return models.OutboundQueue{}, errors.New("queue recipient is empty")
	}
	now := time.Now()
	row := models.OutboundQueue{
		Kind: kind, UserID: userID, MailFrom: mailFrom, RcptTo: rcptTo,
		RawData: append([]byte(nil), raw...), Attempts: 0, MaxAttempts: len(retryDelays),
		NextRetryAt: now, Status: StatusPending, Created: now, Updated: now,
	}
	if emailID > 0 {
		row.EmailID = &emailID
	}
	_, err := db.Instance.Insert(&row)
	return row, err
}

func List(userID int, admin bool) ([]models.OutboundQueue, error) {
	var rows []models.OutboundQueue
	q := db.Instance.Desc("id")
	if !admin {
		q = q.Where("user_id=?", userID)
	}
	err := q.Limit(200).Find(&rows)
	for i := range rows {
		rows[i].RawData = nil
	}
	return rows, err
}

func Retry(id, userID int, admin bool) error {
	q := db.Instance.ID(id)
	if !admin {
		q = q.Where("user_id=?", userID)
	}
	updated, err := q.Cols("status", "attempts", "next_retry_at", "last_error", "lease_until", "updated").Update(&models.OutboundQueue{
		Status: StatusPending, Attempts: 0, NextRetryAt: time.Now(), LastError: "", LeaseUntil: nil, Updated: time.Now(),
	})
	if err == nil && updated == 0 {
		return ErrNotFound
	}
	return err
}

func Delete(id, userID int, admin bool) error {
	q := db.Instance.ID(id)
	if !admin {
		q = q.Where("user_id=?", userID)
	}
	deleted, err := q.Delete(&models.OutboundQueue{})
	if err == nil && deleted == 0 {
		return ErrNotFound
	}
	return err
}

func DeliverLocal(ctx *pcontext.Context, email *parsemail.Email, recipient string) error {
	handlers.RLock()
	fn := handlers.local
	handlers.RUnlock()
	if fn == nil {
		return ErrLocalUnavailable
	}
	return fn(ctx, email, parsemail.BuilderUser(recipient))
}

func Start(ctx context.Context) {
	workers := workerCount()
	for i := 0; i < workers; i++ {
		go worker(ctx, i)
	}
	go leaseRecovery(ctx)
	go parentReconcileLoop(ctx)
}

func worker(ctx context.Context, id int) {
	interval := time.Second
	for ctx.Err() == nil {
		row, ok, err := claimOne()
		if err != nil {
			log.Errorf("outbound queue worker %d claim failed: %v", id, err)
			wait(ctx, 5*time.Second)
			continue
		}
		if !ok {
			wait(ctx, interval)
			continue
		}
		if err := deliver(ctx, row); err != nil {
			handleFailure(ctx, row, err)
		} else {
			handleSuccess(row)
		}
	}
}

func workerCount() int {
	value, _ := strconv.Atoi(os.Getenv("PMAIL_QUEUE_WORKERS"))
	if value <= 0 {
		value = 4
	}
	if value > 32 {
		value = 32
	}
	return value
}

func wait(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func leaseRecovery(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := recoverExpiredLeases(time.Now()); err != nil {
				log.Errorf("outbound queue lease recovery failed: %v", err)
			}
		}
	}
}

func recoverExpiredLeases(now time.Time) error {
	_, err := db.Instance.Exec("UPDATE outbound_queue SET status=?, lease_until=NULL, updated=? WHERE status=? AND lease_until IS NOT NULL AND lease_until<?", StatusPending, now, StatusSending, now)
	return err
}

func claimOne() (models.OutboundQueue, bool, error) {
	var row models.OutboundQueue
	now := time.Now()
	lease := now.Add(10 * time.Minute)
	// Use the shared transaction helper with a concrete xorm session below.
	var claimErr error
	claimErr = db.Transaction(db.Instance, func(session *xorm.Session) error {
		var found bool
		if db.Instance.DriverName() == "sqlite" {
			found, claimErr = session.Where("status=? AND next_retry_at<=?", StatusPending, now).Asc("id").Limit(1).Get(&row)
		} else {
			found, claimErr = session.SQL("SELECT * FROM outbound_queue WHERE status=? AND next_retry_at<=? ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED", StatusPending, now).Get(&row)
		}
		if claimErr != nil || !found {
			if claimErr != nil {
				return claimErr
			}
			row = models.OutboundQueue{}
			return nil
		}
		_, claimErr = session.Table(&models.OutboundQueue{}).ID(row.ID).Cols("status", "attempts", "lease_until", "updated").Update(map[string]interface{}{
			"status": StatusSending, "attempts": row.Attempts + 1, "lease_until": lease, "updated": now,
		})
		return claimErr
	})
	if claimErr != nil {
		return models.OutboundQueue{}, false, claimErr
	}
	if row.ID == 0 {
		return models.OutboundQueue{}, false, nil
	}
	row.Attempts++
	row.Status = StatusSending
	row.LeaseUntil = &lease
	return row, true, nil
}

func deliver(ctx context.Context, row models.OutboundQueue) error {
	handlers.RLock()
	fn := handlers.external
	handlers.RUnlock()
	if fn == nil {
		return errors.New("external delivery handler is unavailable")
	}
	pctx := &pcontext.Context{UserID: row.UserID}
	return fn(pctx, row.MailFrom, row.RawData, row.RcptTo)
}

func handleSuccess(row models.OutboundQueue) {
	now := time.Now()
	_, err := db.Instance.Table(&models.OutboundQueue{}).ID(row.ID).Cols("status", "lease_until", "last_error", "updated").Update(map[string]interface{}{
		"status": StatusSent, "lease_until": nil, "last_error": "", "updated": now,
	})
	if err != nil {
		log.Errorf("mark outbound queue %d sent failed: %v", row.ID, err)
	}
	updateParent(row, false)
}

func handleFailure(ctx context.Context, row models.OutboundQueue, deliveryErr error) {
	permanent := permanentError(deliveryErr)
	final := permanent || row.Attempts >= row.MaxAttempts
	now := time.Now()
	status := StatusPending
	next := now.Add(retryDelays[min(row.Attempts-1, len(retryDelays)-1)])
	if final {
		status = StatusFailed
	}
	_, err := db.Instance.Table(&models.OutboundQueue{}).ID(row.ID).Cols("status", "next_retry_at", "last_error", "lease_until", "updated").Update(map[string]interface{}{
		"status": status, "next_retry_at": next, "last_error": deliveryErr.Error(), "lease_until": nil, "updated": now,
	})
	if err != nil {
		log.Errorf("mark outbound queue %d failure failed: %v", row.ID, err)
		return
	}
	if final {
		row.LastError = deliveryErr.Error()
		updateParent(row, true)
		handlers.RLock()
		bounce := handlers.bounce
		handlers.RUnlock()
		if bounce != nil && row.UserID > 0 {
			if ShouldSuppressBounce(row) {
				log.WithContext(ctx).Infof("suppress bounce for queue=%d: source is empty, daemon, or automatic", row.ID)
			} else {
				bounce(&pcontext.Context{UserID: row.UserID}, row)
			}
		}
	}
}

func permanentError(err error) bool {
	var te *textproto.Error
	if errors.As(err, &te) {
		return te.Code >= 500 && te.Code <= 599
	}
	var se *smtp.SMTPError
	if errors.As(err, &se) {
		return se.Code >= 500 && se.Code <= 599
	}
	var de *net.DNSError
	return errors.As(err, &de) && de.IsNotFound
}

func parentReconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var parents []models.UserEmail
			if err := db.Instance.Where("status=?", consts.EmailStatusDeliveryPending).Limit(200).Find(&parents); err != nil {
				log.Errorf("queue parent reconciliation scan failed: %v", err)
				continue
			}
			for _, parent := range parents {
				if err := ReconcileParent(parent.EmailID, parent.UserID); err != nil {
					log.Errorf("queue parent reconciliation email=%d user=%d failed: %v", parent.EmailID, parent.UserID, err)
				}
			}
		}
	}
}

// ReconcileParent is idempotent and can safely run before or after queue workers
// finish. It only changes the sender's pending user_email relation.
func ReconcileParent(emailID, userID int) error {
	var email models.Email
	found, err := db.Instance.ID(emailID).Get(&email)
	if err != nil || !found {
		return err
	}
	pending, err := db.Instance.Where("email_id=? AND status IN (?,?)", emailID, StatusPending, StatusSending).Count(&models.OutboundQueue{})
	if err != nil {
		return err
	}
	status := int8(consts.EmailStatusSent)
	// This marker is written while an SMTP submission is still in flight so
	// crash recovery can distinguish an unfinished audit from a normal send.
	// It is not a delivery failure and must not turn a fully delivered parent
	// into status=failed once all queue children are sent.
	const pendingAuditPlaceholder = "delivery result pending; outcome unknown after interruption"
	errorText := email.Error.String
	if strings.TrimSpace(errorText) == pendingAuditPlaceholder {
		errorText = ""
	}
	if pending > 0 {
		status = consts.EmailStatusDeliveryPending
		if email.Error.Valid && strings.TrimSpace(email.Error.String) == pendingAuditPlaceholder {
			errorText = email.Error.String
		}
	} else {
		var failed []models.OutboundQueue
		if err := db.Instance.Where("email_id=? AND status=?", emailID, StatusFailed).Find(&failed); err != nil {
			return err
		}
		if len(failed) > 0 || strings.TrimSpace(errorText) != "" {
			status = consts.EmailStatusFail
			parts := make([]string, 0, len(failed)+1)
			if strings.TrimSpace(errorText) != "" {
				parts = append(parts, errorText)
			}
			for _, row := range failed {
				if strings.TrimSpace(row.LastError) != "" {
					parts = append(parts, row.RcptTo+": "+row.LastError)
				}
			}
			errorText = strings.Join(parts, "; ")
		}
	}
	// Parent email and the sender's mailbox relation are one audit state.
	// Keep both updates in db.Transaction so SQLite BUSY/LOCKED retries the
	// whole pair and a trigger/constraint failure rolls both changes back.
	var nullableError any
	if strings.TrimSpace(errorText) != "" {
		nullableError = errorText
	}
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if _, err := tx.Table(&models.Email{}).ID(emailID).Cols("status", "error", "update_time").Update(map[string]interface{}{"status": status, "error": nullableError, "update_time": time.Now()}); err != nil {
			return err
		}
		if _, err := tx.Table(&models.UserEmail{}).Where("email_id=? AND user_id=? AND status=?", emailID, userID, consts.EmailStatusDeliveryPending).Cols("status").Update(map[string]interface{}{"status": status}); err != nil {
			return err
		}
		return nil
	})
}

func updateParent(row models.OutboundQueue, failed bool) {
	if row.Kind != KindSend || row.EmailID == nil {
		return
	}
	if failed && strings.TrimSpace(row.LastError) != "" {
		if _, err := db.Instance.Table(&models.Email{}).ID(*row.EmailID).Cols("error").Update(map[string]interface{}{"error": row.LastError}); err != nil {
			log.Errorf("update parent email error %d failed: %v", *row.EmailID, err)
		}
	}
	if err := ReconcileParent(*row.EmailID, row.UserID); err != nil {
		log.Errorf("reconcile parent email=%d user=%d failed: %v", *row.EmailID, row.UserID, err)
	}
}

// Fingerprint is useful to diagnose duplicate queue rows without exposing raw mail.
func Fingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}
