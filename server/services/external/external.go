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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/listen/smtp_server"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/quota"
	pcontext "github.com/Jinnrry/pmail/utils/context"
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

var accountLocks sync.Map
var workerSemaphore = make(chan struct{}, 3)

const syncDeadline = 5 * time.Minute

type syncCounterKey struct{}

func accountMutex(id int) *sync.Mutex {
	lock, _ := accountLocks.LoadOrStore(id, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

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
	return launchSync(context.WithoutCancel(ctx), id)
}

func TestConnection(input models.ExternalAccount, password string) ([]string, error) {
	if !wiretext.Line(password) {
		return nil, ErrControlCharacters
	}
	if err := normalize(&input); err != nil {
		return nil, err
	}
	if err := validateHost(input.Host); err != nil {
		return nil, err
	}
	if input.Protocol == "imap" {
		return testIMAP(input, password)
	}
	if input.Protocol == "pop3" {
		return testPOP3(input, password)
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

func Start(ctx context.Context) {
	workers := 3
	if value, err := strconv.Atoi(strings.TrimSpace(getenv("PMAIL_FETCH_WORKERS"))); err == nil && value > 0 && value <= 32 {
		workers = value
	}
	workerSemaphore = make(chan struct{}, workers)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				scan(ctx)
			}
		}
	}()
}

func scan(ctx context.Context) {
	var accounts []models.ExternalAccount
	if err := db.Instance.Where("enabled=?", true).Find(&accounts); err != nil {
		log.Errorf("external account scan failed: %v", err)
		return
	}
	now := time.Now()
	for _, account := range accounts {
		if account.LastSyncAt != nil && now.Before(account.LastSyncAt.Add(time.Duration(max(account.IntervalMinutes, 2))*time.Minute)) {
			continue
		}
		if err := launchSync(ctx, account.ID); err != nil && !errors.Is(err, ErrSyncInProgress) {
			log.WithField("account_id", account.ID).Warnf("external sync launch failed: %v", err)
		}
	}
}

var ErrSyncInProgress = errors.New("external account is already syncing")

func launchSync(ctx context.Context, id int) error {
	select {
	case workerSemaphore <- struct{}{}:
	default:
		return ErrSyncInProgress
	}
	mu := accountMutex(id)
	if !mu.TryLock() {
		<-workerSemaphore
		return ErrSyncInProgress
	}
	go func() {
		defer func() {
			mu.Unlock()
			<-workerSemaphore
		}()
		if err := syncAccountLocked(ctx, id); err != nil {
			log.WithField("account_id", id).Warnf("external sync failed: %v", err)
		}
	}()
	return nil
}

func SyncAccount(ctx context.Context, id int) error {
	mu := accountMutex(id)
	if !mu.TryLock() {
		return ErrSyncInProgress
	}
	defer mu.Unlock()
	return syncAccountLocked(ctx, id)
}

func syncAccountLocked(ctx context.Context, id int) (resultErr error) {
	started := time.Now()
	var account models.ExternalAccount
	ok, err := db.Instance.ID(id).Get(&account)
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
	if _, err = db.Instance.ID(id).Cols("last_sync_at", "last_error", "updated").Update(&models.ExternalAccount{LastSyncAt: &attempted, LastError: "", Updated: attempted}); err != nil {
		return err
	}
	syncCtx, cancel := context.WithTimeout(ctx, syncDeadline)
	defer cancel()
	syncCtx = context.WithValue(syncCtx, syncCounterKey{}, &counter)
	password, legacy, err := decryptAccount(account)
	if err != nil {
		resultErr = recordError(account, err, false)
		return resultErr
	}
	if !wiretext.Line(password) || !wiretext.Line(account.Username) || !wiretext.Line(account.Folder) {
		return recordError(account, ErrControlCharacters, false)
	}
	if legacy {
		replacement, err := encryptAccount(account, password)
		if err != nil {
			return recordError(account, err, false)
		}
		n, err := db.Instance.ID(account.ID).Where("password_enc=?", account.PasswordEnc).NoAutoTime().Cols("password_enc").Update(&models.ExternalAccount{PasswordEnc: replacement})
		if err != nil {
			return recordError(account, err, false)
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
		resultErr = recordError(account, syncErr, isAuthError(syncErr))
		return resultErr
	}
	success := time.Now()
	_, resultErr = db.Instance.ID(account.ID).Cols("last_sync_at", "last_success_at", "last_error", "auth_failures", "updated").Update(&models.ExternalAccount{LastSyncAt: &attempted, LastSuccessAt: &success, LastError: "", AuthFailures: 0, Updated: success})
	return resultErr
}

func recordError(account models.ExternalAccount, syncErr error, authFailure bool) error {
	failures := account.AuthFailures
	if authFailure {
		failures++
	}
	enabled := account.Enabled
	if failures >= 3 || errors.Is(syncErr, quota.ErrExceeded) {
		enabled = false
	}
	_, err := db.Instance.ID(account.ID).Cols("last_error", "auth_failures", "enabled", "updated").Update(&models.ExternalAccount{LastError: syncErr.Error(), AuthFailures: failures, Enabled: enabled, Updated: time.Now()})
	return syncErrOrDB(syncErr, err)
}

func syncIMAP(ctx context.Context, account models.ExternalAccount, password string) error {
	client, err := dialIMAP(account)
	if err != nil {
		return err
	}
	stopWatch := watchClientContext(ctx, client)
	defer stopWatch()
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
		selected, err := client.Select(folder, &imap.SelectOptions{ReadOnly: account.DeleteRemote == false}).Wait()
		if err != nil {
			return err
		}
		uidValidity := strconv.FormatUint(uint64(selected.UIDValidity), 10)
		if account.LastUIDValidity != "" && account.LastUIDValidity != uidValidity {
			account.LastUID = 0
		}
		start := account.LastUID + 1
		if start < 1 {
			start = 1
		}
		stop := int64(selected.UIDNext) - 1
		if stop < start {
			continue
		}
		set := imap.UIDSet{imap.UIDRange{Start: imap.UID(start), Stop: imap.UID(stop)}}
		if account.LastUID == 0 && account.InitialDays > 0 {
			search, searchErr := client.UIDSearch(&imap.SearchCriteria{Since: time.Now().Add(-time.Duration(account.InitialDays) * 24 * time.Hour)}, nil).Wait()
			if searchErr != nil {
				return searchErr
			}
			if uids := search.AllUIDs(); len(uids) > 0 {
				if len(uids) > 200 {
					uids = uids[:200]
				}
				set = imap.UIDSetNum(uids...)
			} else {
				continue
			}
		} else if stop-start > 199 {
			set = imap.UIDSet{imap.UIDRange{Start: imap.UID(start), Stop: imap.UID(start + 199)}}
		}
		section := &imap.FetchItemBodySection{}
		cmd := client.Fetch(set, &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}})
		for {
			msg := cmd.Next()
			if msg == nil {
				break
			}
			buffer, err := msg.Collect()
			if err != nil {
				_ = cmd.Close()
				return err
			}
			for _, body := range buffer.BodySection {
				if err := importMessage(ctx, account, folder, uint64(selected.UIDValidity), uint64(buffer.UID), "", body.Bytes); err != nil {
					_ = cmd.Close()
					return err
				}
			}
		}
		if err := cmd.Close(); err != nil {
			return err
		}
		if account.DeleteRemote {
			store := client.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil)
			if err := store.Close(); err != nil {
				return err
			}
			if _, err := client.UIDExpunge(set).Collect(); err != nil {
				if _, fallbackErr := client.Expunge().Collect(); fallbackErr != nil {
					return fmt.Errorf("uid expunge: %v; expunge fallback: %w", err, fallbackErr)
				}
			}
		}
		if _, err := db.Instance.ID(account.ID).Cols("last_uidvalidity", "last_uid", "updated").Update(&models.ExternalAccount{LastUIDValidity: uidValidity, LastUID: stop, Updated: time.Now()}); err != nil {
			return err
		}
	}
	return nil
}

func syncPOP3(ctx context.Context, account models.ExternalAccount, password string) error {
	conn, err := dialPOP3(account)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	stopWatch := watchConnContext(ctx, conn)
	defer stopWatch()
	defer func() {
		_ = popCommand(conn, reader, "QUIT")
		_ = conn.Close()
	}()
	if _, err := readPOP3(reader); err != nil {
		return err
	}
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
	return nil
}

func importMessage(ctx context.Context, account models.ExternalAccount, folder string, uidValidity, uid uint64, uidl string, raw []byte) error {
	mid := messageID(raw)
	if (uid != 0 || uidValidity != 0) && seen(account.ID, folder, uidValidity, uid, uidl) {
		return nil
	}
	if uid == 0 && uidValidity == 0 && mid != "" && seenMessage(account.ID, mid) {
		return nil
	}
	already, err := beginSeen(account.ID, folder, uidValidity, uid, uidl, mid)
	if err != nil || already {
		return err
	}
	primary, err := address.Primary(account.UserID)
	if err != nil {
		_ = deleteSeen(account.ID, folder, uidValidity, uid, uidl)
		return err
	}
	pctx := &pcontext.Context{UserID: account.UserID}
	if _, err = smtp_server.ImportExternal(pctx, raw, primary.Address, account.TargetGroupID); err != nil {
		_ = deleteSeen(account.ID, folder, uidValidity, uid, uidl)
		return err
	}
	if err = finishSeen(account.ID, folder, uidValidity, uid, uidl, mid); err != nil {
		log.Errorf("external_seen completion failed account=%d folder=%s uid=%d: %v", account.ID, folder, uid, err)
		return err
	}
	if value := ctx.Value(syncCounterKey{}); value != nil {
		atomic.AddInt64(value.(*int64), 1)
	}
	return nil
}

func messageID(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(msg.Header.Get("Message-Id"))
}

func seen(accountID int, folder string, uidValidity, uid uint64, uidl string) bool {
	var row models.ExternalSeen
	ok, _ := db.Instance.Where("account_id=? AND folder=? AND uidvalidity=? AND uid=? AND uidl=? AND status=?", accountID, folder, int64(uidValidity), int64(uid), uidl, "done").Get(&row)
	return ok
}

func seenMessage(accountID int, messageID string) bool {
	var row models.ExternalSeen
	ok, _ := db.Instance.Where("account_id=? AND message_id=? AND status=?", accountID, messageID, "done").Get(&row)
	return ok
}

func beginSeen(accountID int, folder string, uidValidity, uid uint64, uidl, mid string) (bool, error) {
	var row models.ExternalSeen
	found, err := db.Instance.Where("account_id=? AND folder=? AND uidvalidity=? AND uid=? AND uidl=?", accountID, folder, int64(uidValidity), int64(uid), uidl).Get(&row)
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
	_, err = db.Instance.Insert(&models.ExternalSeen{AccountID: accountID, Folder: folder, UIDValidity: int64(uidValidity), UID: int64(uid), UIDL: uidl, MessageID: mid, Status: "pending", Created: time.Now()})
	return false, err
}

func finishSeen(accountID int, folder string, uidValidity, uid uint64, uidl, mid string) error {
	_, err := db.Instance.Table(&models.ExternalSeen{}).Where("account_id=? AND folder=? AND uidvalidity=? AND uid=? AND uidl=?", accountID, folder, int64(uidValidity), int64(uid), uidl).Cols("status", "message_id").Update(map[string]any{"status": "done", "message_id": mid})
	return err
}

func deleteSeen(accountID int, folder string, uidValidity, uid uint64, uidl string) error {
	_, err := db.Instance.Table(&models.ExternalSeen{}).Where("account_id=? AND folder=? AND uidvalidity=? AND uid=? AND uidl=?", accountID, folder, int64(uidValidity), int64(uid), uidl).Delete(&models.ExternalSeen{})
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

func dialIMAP(a models.ExternalAccount) (*imapclient.Client, error) {
	addresses, err := safeAddresses(a.Host, a.Port)
	if err != nil {
		return nil, err
	}
	var last error
	for _, addr := range addresses {
		d := &net.Dialer{Timeout: 60 * time.Second, Control: connectionControl()}
		conn, dialErr := d.Dial("tcp", addr)
		if dialErr != nil {
			last = dialErr
			continue
		}
		if err = conn.SetDeadline(time.Now().Add(syncDeadline)); err != nil {
			_ = conn.Close()
			last = err
			continue
		}
		tlsConfig := &tls.Config{ServerName: a.Host, InsecureSkipVerify: a.InsecureSkipVerify}
		opts := &imapclient.Options{TLSConfig: tlsConfig}
		var client *imapclient.Client
		switch a.TLSMode {
		case "tls":
			tlsConn := tls.Client(conn, tlsConfig)
			err = tlsConn.Handshake()
			if err == nil {
				client = imapclient.New(tlsConn, opts)
			}
		case "starttls":
			client, err = imapclient.NewStartTLS(conn, opts)
		default:
			client = imapclient.New(conn, opts)
		}
		if err == nil && client != nil {
			return client, nil
		}
		last = err
		if client == nil {
			_ = conn.Close()
		}
	}
	return nil, last
}

func dialPOP3(a models.ExternalAccount) (net.Conn, error) {
	addresses, err := safeAddresses(a.Host, a.Port)
	if err != nil {
		return nil, err
	}
	var last error
	for _, addr := range addresses {
		d := net.Dialer{Timeout: 60 * time.Second, Control: connectionControl()}
		conn, dialErr := d.Dial("tcp", addr)
		if dialErr != nil {
			last = dialErr
			continue
		}
		if err = conn.SetDeadline(time.Now().Add(syncDeadline)); err != nil {
			_ = conn.Close()
			last = err
			continue
		}
		if a.TLSMode == "tls" {
			tlsConn := tls.Client(conn, &tls.Config{ServerName: a.Host, InsecureSkipVerify: a.InsecureSkipVerify})
			if err = tlsConn.Handshake(); err != nil {
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

func watchClientContext(ctx context.Context, client *imapclient.Client) func() {
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
		case <-stop:
		}
	}()
	return func() { close(stop) }
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

func testIMAP(a models.ExternalAccount, password string) ([]string, error) {
	c, err := dialIMAP(a)
	if err != nil {
		return nil, err
	}
	defer c.Close()
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
func testPOP3(a models.ExternalAccount, password string) ([]string, error) {
	c, err := dialPOP3(a)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	r := bufio.NewReader(c)
	if _, err = readPOP3(r); err != nil {
		return nil, err
	}
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
	line, err := r.ReadString('\n')
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
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "." {
			return out, nil
		}
		out = append(out, strings.TrimPrefix(line, ".."))
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
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if strings.TrimRight(line, "\r\n") == "." {
			return b.Bytes(), nil
		}
		b.WriteString(strings.TrimPrefix(line, ".."))
	}
}
func syncErrOrDB(original, dbErr error) error {
	if dbErr != nil {
		return dbErr
	}
	return original
}
func ptrTime(t time.Time) *time.Time { return &t }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func getenv(key string) string { return os.Getenv(key) }
