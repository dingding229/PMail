package external

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/listen/smtp_server"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/quota"
	pcontext "github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/intbounds"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	"github.com/Jinnrry/pmail/utils/wiretext"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	log "github.com/sirupsen/logrus"
	"xorm.io/xorm"
)

var (
	ErrNotFound          = errors.New("external account not found")
	ErrProtocol          = errors.New("protocol must be imap or pop3")
	ErrTLSMode           = errors.New("tls_mode must be tls, starttls or none")
	ErrPort              = errors.New("invalid external port")
	ErrPasswordRequired  = errors.New("external endpoint change requires a new password")
	ErrEndpointOwner     = errors.New("only the account owner may change the external endpoint")
	ErrCredentialRace    = errors.New("external settings changed concurrently; retry")
	ErrControlCharacters = errors.New("external credentials or folder contain control characters")
	ErrAuth              = errors.New("external mailbox authentication failed")
)

const syncDeadline = 5 * time.Minute

type syncCounterKey struct{}

func List(userID int, admin bool) ([]models.ExternalAccount, error) {
	var rows []models.ExternalAccount
	q := db.Instance.Desc("id")
	if !admin {
		q = q.Where("user_id=?", userID)
	}
	if err := q.Find(&rows); err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].PasswordEnc = ""
	}
	return rows, nil
}

func Create(userID int, input models.ExternalAccount, password string) (models.ExternalAccount, error) {
	if password == "" {
		return models.ExternalAccount{}, ErrPasswordRequired
	}
	if err := normalize(&input); err != nil {
		return models.ExternalAccount{}, err
	}
	if !wiretext.Line(password) {
		return models.ExternalAccount{}, ErrControlCharacters
	}
	if err := validateHost(input.Host); err != nil {
		return models.ExternalAccount{}, err
	}
	var stored models.ExternalAccount
	err := db.Transaction(db.Instance, func(tx *xorm.Session) error {
		row := input
		row.ID, row.UserID, row.PasswordEnc = 0, userID, ""
		row.Created = time.Now()
		row.Updated = row.Created
		if _, err := tx.Insert(&row); err != nil {
			return err
		}
		encoded, err := encryptAccount(row, password)
		if err != nil {
			return err
		}
		if _, err = tx.ID(row.ID).NoAutoTime().Cols("password_enc").Update(&models.ExternalAccount{PasswordEnc: encoded}); err != nil {
			return err
		}
		stored = row
		return nil
	})
	if err == nil {
		log.Infof("audit_event=external_credential_created actor_id=%d external_id=%d", userID, stored.ID)
	}
	return stored, err
}

func Update(userID int, admin bool, input models.ExternalAccount, password string) (models.ExternalAccount, error) {
	empty := models.ExternalAccount{}
	q := db.Instance.ID(input.ID)
	if !admin {
		q.Where("user_id=?", userID)
	}
	var current models.ExternalAccount
	found, err := q.Get(&current)
	if err != nil {
		return empty, err
	}
	if !found {
		return empty, ErrNotFound
	}
	if input.Name == "" {
		input.Name = current.Name
	}
	if input.Host == "" {
		input.Host = current.Host
	}
	if input.Username == "" {
		input.Username = current.Username
	}
	if input.Folder == "" {
		input.Folder = current.Folder
	}
	if input.Protocol == "" {
		input.Protocol = current.Protocol
	}
	if input.TLSMode == "" {
		input.TLSMode = current.TLSMode
	}
	if input.Port == 0 {
		input.Port = current.Port
	}
	if input.IntervalMinutes == 0 {
		input.IntervalMinutes = current.IntervalMinutes
	}
	if err := normalize(&input); err != nil {
		return empty, err
	}
	if changed := binding(input) != binding(current); changed {
		if userID != current.UserID {
			log.Warnf("audit_event=external_endpoint_change_denied actor_id=%d owner_id=%d external_id=%d", userID, current.UserID, current.ID)
			return empty, ErrEndpointOwner
		}
		if password == "" {
			return empty, ErrPasswordRequired
		}
		if err := validateHost(input.Host); err != nil {
			return empty, err
		}
	}
	input.UserID = current.UserID
	input.PasswordEnc = current.PasswordEnc
	if password != "" {
		if !wiretext.Line(password) {
			return empty, ErrControlCharacters
		}
		input.PasswordEnc, err = encryptAccount(input, password)
		if err != nil {
			return empty, err
		}
	}
	input.Created = current.Created
	input.Updated = time.Now()
	n, err := db.Instance.ID(input.ID).Where("user_id=? AND password_enc=?", current.UserID, current.PasswordEnc).NoAutoTime().Cols("name", "protocol", "host", "port", "tls_mode", "username", "password_enc", "folder", "interval_minutes", "initial_days", "delete_remote", "target_group_id", "enabled", "insecure_skip_verify", "updated").Update(&input)
	input.PasswordEnc = ""
	if err == nil && n == 0 {
		return empty, ErrCredentialRace
	}
	if err == nil && password != "" {
		log.Infof("audit_event=external_credential_updated actor_id=%d owner_id=%d external_id=%d endpoint_changed=%t", userID, current.UserID, current.ID, binding(input) != binding(current))
	}
	return input, err
}

func Delete(userID int, admin bool, id int) error {
	q := db.Instance.ID(id)
	if !admin {
		q = q.Where("user_id=?", userID)
	}
	n, err := q.Delete(&models.ExternalAccount{})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func SyncNow(ctx context.Context, userID int, admin bool, id int) error {
	q := db.Instance.ID(id)
	if !admin {
		q = q.Where("user_id=?", userID)
	}
	var account models.ExternalAccount
	found, err := q.Get(&account)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	// HTTP request contexts are cancelled after the response. Detach the
	// background sync while retaining the request context as its value parent.
	return scheduleNow(id)
}

func TestConnection(input models.ExternalAccount, password string) ([]string, error) {
	return TestConnectionContext(context.Background(), input, password)
}

func TestConnectionContext(ctx context.Context, input models.ExternalAccount, password string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if !wiretext.Line(password) {
		return nil, ErrControlCharacters
	}
	if err := normalize(&input); err != nil {
		return nil, err
	}

	if input.Protocol == "imap" {
		return testIMAP(ctx, input, password)
	}
	if input.Protocol == "pop3" {
		return testPOP3(ctx, input, password)
	}
	return nil, errors.New("unsupported external protocol")
}

func Presets() []map[string]any {
	return []map[string]any{
		{"name": "Gmail", "protocol": "imap", "host": "imap.gmail.com", "port": 993, "tls_mode": "tls"},
		{"name": "Outlook", "protocol": "imap", "host": "outlook.office365.com", "port": 993, "tls_mode": "tls"},
		{"name": "QQ", "protocol": "imap", "host": "imap.qq.com", "port": 993, "tls_mode": "tls"},
		{"name": "163", "protocol": "imap", "host": "imap.163.com", "port": 993, "tls_mode": "tls"},
		{"name": "iCloud", "protocol": "imap", "host": "imap.mail.me.com", "port": 993, "tls_mode": "tls"},
	}
}

func scan(s *scheduler) {
	var accounts []models.ExternalAccount
	if err := db.Instance.Context(s.ctx).Where("enabled=?", true).Find(&accounts); err != nil {
		log.Warnf("external mailbox scan: %v", err)
		return
	}
	now := time.Now()
	for _, a := range accounts {
		if a.LastSyncAt != nil && now.Before(a.LastSyncAt.Add(time.Duration(max(a.IntervalMinutes, 2))*time.Minute)) {
			continue
		}
		_ = s.launch(a.ID)
	}
}

func syncAccountLocked(ctx context.Context, id int) (resultErr error) {
	started := time.Now()
	var account models.ExternalAccount
	ok, err := db.Instance.Context(ctx).ID(id).Get(&account)
	if err != nil || !ok {
		if err == nil {
			err = ErrNotFound
		}
		log.WithFields(log.Fields{"account_id": id, "duration": time.Since(started), "error": err}).Error("external sync finished before account lookup")
		return err
	}
	log.WithFields(log.Fields{"account_id": account.ID, "protocol": account.Protocol}).Info("external sync started")
	var counter int64
	defer func() {
		count := atomic.LoadInt64(&counter)
		log.WithFields(log.Fields{
			"account_id": account.ID,
			"protocol":   account.Protocol,
			"duration":   time.Since(started),
			"imported":   count,
			"error":      resultErr,
		}).Info("external sync finished")
	}()
	attempted := time.Now()
	if _, err = db.Instance.Context(ctx).ID(id).Cols("last_sync_at", "last_error", "updated").Update(&models.ExternalAccount{LastSyncAt: &attempted, LastError: "", Updated: attempted}); err != nil {
		return err
	}
	syncCtx, cancel := context.WithTimeout(ctx, syncDeadline)
	defer cancel()
	syncCtx = context.WithValue(syncCtx, syncCounterKey{}, &counter)
	password, legacy, err := decryptAccount(account)
	if err != nil {
		resultErr = recordError(ctx, account, err, false)
		return resultErr
	}
	if !wiretext.Line(password) || !wiretext.Line(account.Username) || !wiretext.Line(account.Folder) {
		return recordError(ctx, account, ErrControlCharacters, false)
	}
	if legacy {
		replacement, err := encryptAccount(account, password)
		if err != nil {
			return recordError(ctx, account, err, false)
		}
		n, err := db.Instance.Context(ctx).ID(account.ID).Where("password_enc=?", account.PasswordEnc).NoAutoTime().Cols("password_enc").Update(&models.ExternalAccount{PasswordEnc: replacement})
		if err != nil {
			return recordError(ctx, account, err, false)
		}
		if n == 0 {
			return ErrCredentialRace
		}
		log.Infof("audit_event=external_credential_bound external_id=%d owner_id=%d", account.ID, account.UserID)
	}
	var syncErr error
	if account.Protocol == "imap" {
		syncErr = syncIMAP(syncCtx, account, password)
	} else {
		syncErr = syncPOP3(syncCtx, account, password)
	}
	if syncErr != nil {
		resultErr = recordError(ctx, account, syncErr, isAuthError(syncErr))
		return resultErr
	}
	success := time.Now()
	_, resultErr = db.Instance.Context(ctx).ID(account.ID).Cols("last_sync_at", "last_success_at", "last_error", "auth_failures", "updated").Update(&models.ExternalAccount{LastSyncAt: &attempted, LastSuccessAt: &success, LastError: "", AuthFailures: 0, Updated: success})
	return resultErr
}

func recordError(ctx context.Context, account models.ExternalAccount, syncErr error, authFailure bool) error {
	failures := account.AuthFailures
	if authFailure {
		failures++
	}
	enabled := account.Enabled
	if failures >= 3 || errors.Is(syncErr, quota.ErrExceeded) {
		enabled = false
	}
	_, err := db.Instance.Context(ctx).ID(account.ID).Cols("last_error", "auth_failures", "enabled", "updated").Update(&models.ExternalAccount{LastError: syncErr.Error(), AuthFailures: failures, Enabled: enabled, Updated: time.Now()})
	return syncErrOrDB(syncErr, err)
}

func syncIMAP(ctx context.Context, account models.ExternalAccount, password string) error {
	client, err := dialIMAP(ctx, account)
	if err != nil {
		return err
	}
	defer context.AfterFunc(ctx, client.abort)()
	defer client.Close()
	if err := client.Login(account.Username, password).Wait(); err != nil {
		return fmt.Errorf("%w: %w", ErrAuth, err)
	}
	folders := strings.Split(account.Folder, ",")
	for _, folder := range folders {
		folder = strings.TrimSpace(folder)
		if folder == "" {
			continue
		}
		selected, err := client.Select(folder, &imap.SelectOptions{ReadOnly: !account.DeleteRemote}).Wait()
		if err != nil {
			return err
		}
		var cursor models.ExternalCursor
		_, err = db.Instance.Context(ctx).Where("account_id=? AND folder=?", account.ID, folder).Get(&cursor)
		if err != nil {
			return err
		}
		validity := int64(selected.UIDValidity)
		if cursor.UIDValidity != validity {
			cursor.LastUID = 0
		}
		if _, ok := intbounds.Uint32(cursor.LastUID); !ok {
			return errors.New("invalid external IMAP cursor UID")
		}
		start := max(1, cursor.LastUID+1)
		high := int64(selected.UIDNext) - 1
		if high < start {
			continue
		}
		stop := min(high, start+199)
		first, ok1 := intbounds.Uint32(start)
		last, ok2 := intbounds.Uint32(stop)
		if !ok1 || !ok2 {
			return errors.New("external IMAP UID range out of bounds")
		}
		set := imap.UIDSet{{Start: imap.UID(first), Stop: imap.UID(last)}}
		if cursor.LastUID == 0 && account.InitialDays > 0 {
			search, err := client.UIDSearch(&imap.SearchCriteria{Since: time.Now().Add(-time.Duration(account.InitialDays) * 24 * time.Hour)}, nil).Wait()
			if err != nil {
				return err
			}
			uids := search.AllUIDs()
			slices.Sort(uids)
			uids = slices.Compact(uids)
			if len(uids) == 0 {
				if err := storeCursor(ctx, account.ID, folder, validity, high); err != nil {
					return err
				}
				continue
			}
			uids = uids[:min(200, len(uids))]
			stop = int64(uids[len(uids)-1])
			set = imap.UIDSetNum(uids...)
		}

		section := &imap.FetchItemBodySection{Peek: true}
		var imported imap.UIDSet
		cmd := client.Fetch(set, &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}})
		for {
			msg := cmd.Next()
			if msg == nil {
				break
			}
			uid, raw, err := readIMAPMail(msg)
			if err != nil {
				abortFetch(client, cmd)
				return err
			}
			if uid > 0 && raw != nil {
				if !set.Contains(uid) {
					abortFetch(client, cmd)
					return errors.New("unexpected remote UID")
				}
				if err := importMessage(ctx, account, folder, uint64(selected.UIDValidity), uint64(uid), "", raw); err != nil {
					abortFetch(client, cmd)
					return err
				}
				imported.AddNum(uid)
			}

		}
		if err := cmd.Close(); err != nil {
			return err
		}
		if account.DeleteRemote && len(imported) > 0 {
			if err := deleteImported(imported, client.Caps().Has(imap.CapUIDPlus),
				func(ids imap.UIDSet) error {
					return client.Store(ids, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close()
				},
				func(ids imap.UIDSet) error { _, err := client.UIDExpunge(ids).Collect(); return err }); err != nil {
				return err
			}
		}
		if err := storeCursor(ctx, account.ID, folder, validity, stop); err != nil {
			return err
		}
		if _, err := db.Instance.Context(ctx).ID(account.ID).Cols("last_uidvalidity", "last_uid").Update(&models.ExternalAccount{LastUIDValidity: strconv.FormatInt(validity, 10), LastUID: stop}); err != nil {
			return err
		}

	}
	return nil
}

func syncPOP3(ctx context.Context, account models.ExternalAccount, password string) error {
	conn, reader, err := connectPOP(ctx, account)
	if err != nil {
		return err
	}
	stopWatch := watchConnContext(ctx, conn)
	defer stopWatch()
	defer conn.Close()
	if err := popCommand(conn, reader, "USER "+account.Username); err != nil {
		return fmt.Errorf("%w: %w", ErrAuth, err)
	}
	if err := popCommand(conn, reader, "PASS "+password); err != nil {
		return fmt.Errorf("%w: %w", ErrAuth, err)
	}
	if _, err := fmt.Fprint(conn, "UIDL\r\n"); err != nil {
		return err
	}
	lines, err := readMultiPOP3(reader)
	if err != nil {
		return err
	}
	count := 0
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 || count >= 200 || !popSequence(parts[0]) {
			continue
		}
		uidl := parts[1]
		if seen(account.ID, "INBOX", 0, 0, uidl) {
			continue
		}
		raw, err := popRetrieve(conn, reader, parts[0])
		if err != nil {
			return err
		}
		if err := importMessage(ctx, account, "INBOX", 0, 0, uidl, raw); err != nil {
			return err
		}
		if account.DeleteRemote {
			if err := popCommand(conn, reader, "DELE "+parts[0]); err != nil {
				return err
			}
		}
		count++
	}
	return popCommand(conn, reader, "QUIT")
}

func importMessage(ctx context.Context, account models.ExternalAccount, folder string, uidValidity, uid uint64, uidl string, raw []byte) error {
	validitySQL, uidSQL, err := externalUIDs(uidValidity, uid)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	primary, err := address.Primary(account.UserID)
	if err != nil {
		return err
	}
	pctx := &pcontext.Context{Context: ctx, UserID: account.UserID}
	row, err := smtp_server.ImportExternal(pctx, raw, primary.Address, account.TargetGroupID, func(tx *xorm.Session) (bool, error) {
		q := tx.ID(account.ID).Where("user_id=?", account.UserID).Cols("id")
		if db.Instance.DriverName() != "sqlite" {
			q.ForUpdate()
		}
		var owner models.ExternalAccount
		ok, err := q.Get(&owner)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, ErrNotFound
		}
		rec := models.ExternalSeen{AccountID: account.ID, Folder: folder, UIDValidity: validitySQL, UID: uidSQL, UIDL: uidl, MessageID: messageID(raw), Status: "done"}
		var previous models.ExternalSeen
		found, err := tx.Where("account_id=? AND folder=? AND uidvalidity=? AND uid=? AND uidl=?", rec.AccountID, rec.Folder, rec.UIDValidity, rec.UID, rec.UIDL).Get(&previous)
		if err != nil {
			return false, err
		}
		if found {
			if previous.Status == "done" {
				return true, nil
			}
			// Pre-transaction versions may have left a pending marker. Re-import
			// rather than guessing whether the remote message is safe to delete.
			_, err = tx.ID(previous.ID).Cols("status", "message_id").Update(&rec)
		} else {
			_, err = tx.Insert(&rec)
		}
		return false, err
	})
	if err == nil && row != nil {
		if v := ctx.Value(syncCounterKey{}); v != nil {
			atomic.AddInt64(v.(*int64), 1)
		}
	}
	return err
}

func storeCursor(ctx context.Context, id int, folder string, validity, uid int64) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		tx.Context(ctx)
		q := tx.ID(id).Cols("id")
		if db.Instance.DriverName() != "sqlite" {
			q.ForUpdate()
		}
		found, err := q.Get(&models.ExternalAccount{})
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		var old models.ExternalCursor
		found, err = tx.Where("account_id=? AND folder=?", id, folder).Get(&old)
		if err != nil {
			return err
		}
		if !found {
			_, err = tx.Insert(&models.ExternalCursor{AccountID: id, Folder: folder, UIDValidity: validity, LastUID: uid})
			return err
		}
		if old.UIDValidity == validity && old.LastUID >= uid {
			return nil
		}
		_, err = tx.Where("account_id=? AND folder=?", id, folder).Cols("uidvalidity", "last_uid").Update(&models.ExternalCursor{UIDValidity: validity, LastUID: uid})
		return err
	})
}

func messageID(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(msg.Header.Get("Message-Id"))
}

func externalUIDs(validity, uid uint64) (int64, int64, error) {
	v, ok1 := intbounds.Int64(validity)
	u, ok2 := intbounds.Int64(uid)
	if !ok1 || !ok2 {
		return 0, 0, errors.New("external message identifier exceeds database range")
	}
	return v, u, nil
}

func seen(accountID int, folder string, uidValidity, uid uint64, uidl string) bool {
	validitySQL, uidSQL, err := externalUIDs(uidValidity, uid)
	if err != nil {
		return false
	}
	var row models.ExternalSeen
	ok, _ := db.Instance.Where("account_id=? AND folder=? AND uidvalidity=? AND uid=? AND uidl=? AND status=?", accountID, folder, validitySQL, uidSQL, uidl, "done").Get(&row)
	return ok
}

func beginSeen(accountID int, folder string, uidValidity, uid uint64, uidl, mid string) (bool, error) {
	validitySQL, uidSQL, err := externalUIDs(uidValidity, uid)
	if err != nil {
		return false, err
	}
	var row models.ExternalSeen
	found, err := db.Instance.Where("account_id=? AND folder=? AND uidvalidity=? AND uid=? AND uidl=?", accountID, folder, validitySQL, uidSQL, uidl).Get(&row)
	if err != nil {
		return false, err
	}
	if found && row.Status == "done" {
		return true, nil
	}
	if found {
		if _, err := db.Instance.Table(&models.ExternalSeen{}).ID(row.ID).Delete(&models.ExternalSeen{}); err != nil {
			return false, err
		}
	}
	_, err = db.Instance.Insert(&models.ExternalSeen{AccountID: accountID, Folder: folder, UIDValidity: validitySQL, UID: uidSQL, UIDL: uidl, MessageID: mid, Status: "pending", Created: time.Now()})
	return false, err
}

func deleteSeen(accountID int, folder string, uidValidity, uid uint64, uidl string) error {
	validitySQL, uidSQL, err := externalUIDs(uidValidity, uid)
	if err != nil {
		return err
	}
	_, err = db.Instance.Table(&models.ExternalSeen{}).Where("account_id=? AND folder=? AND uidvalidity=? AND uid=? AND uidl=?", accountID, folder, validitySQL, uidSQL, uidl).Delete(&models.ExternalSeen{})
	return err
}

func isAuthError(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "authentication") || strings.Contains(text, "invalid credentials") || strings.Contains(text, "auth")
}

func normalize(a *models.ExternalAccount) error {
	if !wiretext.Line(a.Username) || !wiretext.Line(a.Folder) || !wiretext.Line(a.Host) {
		return ErrControlCharacters
	}
	a.Host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(a.Host), "."))
	a.Protocol = strings.ToLower(strings.TrimSpace(a.Protocol))
	a.TLSMode = strings.ToLower(strings.TrimSpace(a.TLSMode))
	if a.Folder == "" {
		a.Folder = "INBOX"
	}
	if a.IntervalMinutes < 2 {
		a.IntervalMinutes = 10
	}
	if a.InitialDays < 0 {
		a.InitialDays = 30
	}
	if a.Protocol != "imap" && a.Protocol != "pop3" {
		return ErrProtocol
	}
	if a.TLSMode != "tls" && a.TLSMode != "starttls" && a.TLSMode != "none" {
		return ErrTLSMode
	}
	if a.Port == 0 {
		if a.Protocol == "imap" {
			if a.TLSMode == "tls" {
				a.Port = 993
			} else {
				a.Port = 143
			}
		} else {
			if a.TLSMode == "tls" {
				a.Port = 995
			} else {
				a.Port = 110
			}
		}
	}
	if a.Port < 1 || a.Port > 65535 {
		return ErrPort
	}
	return nil
}

func connectionControl() func(string, string, syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || isBlockedIP(ip) {
			return ErrPrivateHost
		}
		return nil
	}
}

type externalIMAP struct {
	*imapclient.Client
	abort func()
}

// go-imap pauses its decoder until the current literal is consumed. Close
// the transport first; then let Close observe a read failure on the literal.
func abortFetch(client *externalIMAP, command *imapclient.FetchCommand) {
	client.abort()
	_ = command.Close()
}

// The persisted option defaults to false and can only be enabled explicitly
// by the mailbox owner (endpoint changes require re-entering the password).
func externalTLSConfig(a models.ExternalAccount) *tls.Config {
	return &tls.Config{ServerName: a.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: a.InsecureSkipVerify} // #nosec G402 -- explicit per-account opt-in for self-signed servers; model/database default is false.
}

func dialIMAP(ctx context.Context, a models.ExternalAccount) (*externalIMAP, error) {
	addresses, err := safeAddresses(ctx, a.Host, a.Port)
	if err != nil {
		return nil, err
	}
	var last error
	for _, addr := range addresses {
		d := &net.Dialer{Timeout: 60 * time.Second, Control: connectionControl()}
		conn, dialErr := d.DialContext(ctx, "tcp", addr)
		if dialErr != nil {
			last = dialErr
			continue
		}
		if err = conn.SetDeadline(connectionDeadline(ctx)); err != nil {
			_ = conn.Close()
			last = err
			continue
		}
		tlsConfig := externalTLSConfig(a)
		opts := &imapclient.Options{TLSConfig: tlsConfig}
		var client *imapclient.Client
		switch a.TLSMode {
		case "tls":
			tlsConn := tls.Client(conn, tlsConfig)
			err = tlsConn.HandshakeContext(ctx)
			if err == nil {
				client = imapclient.New(tlsConn, opts)
			}
		case "starttls":
			stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
			client, err = imapclient.NewStartTLS(conn, opts)
			stop()
		default:
			client = imapclient.New(conn, opts)
		}
		if err == nil && client != nil {
			return &externalIMAP{Client: client, abort: func() { _ = conn.Close() }}, nil
		}
		last = err
		if client == nil {
			_ = conn.Close()
		}
	}
	return nil, last
}

func dialPOP3(ctx context.Context, a models.ExternalAccount) (net.Conn, error) {
	addresses, err := safeAddresses(ctx, a.Host, a.Port)
	if err != nil {
		return nil, err
	}
	var last error
	for _, addr := range addresses {
		d := net.Dialer{Timeout: 60 * time.Second, Control: connectionControl()}
		conn, dialErr := d.DialContext(ctx, "tcp", addr)
		if dialErr != nil {
			last = dialErr
			continue
		}
		if err = conn.SetDeadline(connectionDeadline(ctx)); err != nil {
			_ = conn.Close()
			last = err
			continue
		}
		if a.TLSMode == "tls" {
			tlsConn := tls.Client(conn, externalTLSConfig(a))
			if err = tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				last = err
				continue
			}
			conn = tlsConn
		}
		return conn, nil
	}
	return nil, last
}

func watchConnContext(ctx context.Context, conn net.Conn) func() {
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()
	return func() { close(stop) }
}

func testIMAP(ctx context.Context, a models.ExternalAccount, password string) ([]string, error) {
	c, err := dialIMAP(ctx, a)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer context.AfterFunc(ctx, c.abort)()
	if err := c.Login(a.Username, password).Wait(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAuth, err)
	}
	list, err := c.List("", "%", &imap.ListOptions{}).Collect()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		out = append(out, v.Mailbox)
	}
	return out, nil
}
func testPOP3(ctx context.Context, a models.ExternalAccount, password string) ([]string, error) {
	c, r, err := connectPOP(ctx, a)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer watchConnContext(ctx, c)()
	if err = popCommand(c, r, "USER "+a.Username); err != nil {
		return nil, err
	}
	if err = popCommand(c, r, "PASS "+password); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAuth, err)
	}
	return []string{"INBOX"}, nil
}
func popSequence(num string) bool {
	if num == "" || strings.IndexFunc(num, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return false
	}
	n, err := strconv.ParseUint(num, 10, 31)
	return err == nil && n > 0
}

func popCommand(c net.Conn, r *bufio.Reader, command string) error {
	if !wiretext.Line(command) {
		return ErrControlCharacters
	}
	if _, err := fmt.Fprintf(c, "%s\r\n", command); err != nil {
		return err
	}
	_, err := readPOP3(r)
	return err
}
func readPOP3(r *bufio.Reader) (string, error) {
	line, err := readPOPLine(r)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(line, "+OK") {
		return line, errors.New(strings.TrimSpace(line))
	}
	return line, nil
}
func readMultiPOP3(r *bufio.Reader) ([]string, error) {
	if _, err := readPOP3(r); err != nil {
		return nil, err
	}
	var out []string
	size := 0
	for {
		line, err := readPOPLine(r)
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			return out, nil
		}
		size += len(line) + 2
		if size > mailbudget.POPListing {
			return nil, mailbudget.ErrLimit
		}
		out = append(out, unDot(line))
	}
}
func popRetrieve(c net.Conn, r *bufio.Reader, num string) ([]byte, error) {
	if !popSequence(num) {
		return nil, ErrControlCharacters
	}
	if _, err := fmt.Fprintf(c, "RETR %s\r\n", num); err != nil {
		return nil, err
	}
	if _, err := readPOP3(r); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	for {
		line, err := readPOPLine(r)
		if err != nil {
			return nil, err
		}
		if strings.TrimRight(line, "\r\n") == "." {
			return b.Bytes(), nil
		}
		if b.Len()+len(line) > mailbudget.Message {
			return nil, mailbudget.ErrLimit
		}
		b.WriteString(unDot(line))
	}
}
func syncErrOrDB(original, dbErr error) error {
	if dbErr != nil {
		return dbErr
	}
	return original
}
func getenv(key string) string { return os.Getenv(key) }

func readIMAPMail(msg *imapclient.FetchMessageData) (imap.UID, []byte, error) {
	var uid imap.UID
	var raw []byte
	for item := msg.Next(); item != nil; item = msg.Next() {
		switch v := item.(type) {
		case imapclient.FetchItemDataUID:
			uid = v.UID
		case imapclient.FetchItemDataBodySection:
			if v.Literal == nil {
				continue
			}
			if raw != nil {
				return 0, nil, mailbudget.ErrLimit
			}
			var err error
			raw, err = readExternalLiteral(v.Literal)
			if err != nil {
				return 0, nil, err
			}
		}
	}
	return uid, raw, nil
}

func readPOPLine(r *bufio.Reader) (string, error) {
	var result strings.Builder
	for {
		buf, err := r.ReadSlice('\n')
		if result.Len()+len(buf) > mailbudget.POPLine {
			return "", mailbudget.ErrLimit
		}
		result.Write(buf)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return result.String(), err
		}
	}
}

func unDot(line string) string {
	if strings.HasPrefix(line, "..") {
		return line[1:]
	}
	return line
}

func connectionDeadline(ctx context.Context) time.Time {
	d := time.Now().Add(syncDeadline)
	if end, ok := ctx.Deadline(); ok && end.Before(d) {
		return end
	}
	return d
}

func connectPOP(ctx context.Context, a models.ExternalAccount) (net.Conn, *bufio.Reader, error) {
	c, err := dialPOP3(ctx, a)
	if err != nil {
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	r := bufio.NewReader(c)
	if _, err = readPOP3(r); err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	if a.TLSMode == "starttls" {
		if err = popCommand(c, r, "STLS"); err != nil {
			_ = c.Close()
			return nil, nil, err
		}
		if r.Buffered() != 0 {
			_ = c.Close()
			return nil, nil, errors.New("unexpected bytes before POP3 TLS negotiation")
		}
		tlsConn := tls.Client(c, externalTLSConfig(a))
		if err = tlsConn.HandshakeContext(ctx); err != nil {
			_ = c.Close()
			return nil, nil, err
		}
		return tlsConn, bufio.NewReader(tlsConn), nil
	}
	return c, r, nil
}

func deleteImported(ids imap.UIDSet, uidPlus bool, mark, expunge func(imap.UIDSet) error) error {
	if len(ids) == 0 {
		return nil
	}
	if err := mark(ids); err != nil {
		return err
	}
	if uidPlus {
		return expunge(ids)
	}
	return nil
}
func readExternalLiteral(r imap.LiteralReader) ([]byte, error) {
	if r.Size() > mailbudget.Message {
		return nil, mailbudget.ErrLimit
	}
	return mailbudget.Read(r, mailbudget.Message)
}
