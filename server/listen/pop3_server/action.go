package pop3_server

import (
	stdcontext "context"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/auth"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/id"
	"xorm.io/xorm"
)

var (
	errNoSuchMessage  = errors.New("no such message")
	errMaildropLocked = errors.New("maildrop already locked")
	maildropLocks     sync.Map // one POP3 transaction per user across both listeners
	crlf              = strings.NewReplacer("\r\n", "\r\n", "\n", "\r\n", "\r", "\r\n")
)

type messageRef struct {
	id     int
	size   int64 // octets of the exact (unescaped) RETR rendition, -1 until measured
	marked bool
}

type maildrop struct {
	ctx      *context.Context
	messages []messageRef
}

// Restrict snapshots and loads to messages visible in this user's POP3 maildrop.
// An EXISTS guard also de-duplicates copies in user_email.
// POP3 exposes the Inbox, not IMAP's separate user folders. Apply the same
// filter to STAT/LIST and to the links DELE can move to Trash.
const inboxLink = `user_id=? AND status=? AND (group_id=0 OR group_id=?)`
const visibleMessage = `email.type=? AND EXISTS (
	SELECT 1 FROM user_email WHERE email_id=email.id AND ` + inboxLink + `
)`

func openMaildrop(user models.User) (*maildrop, error) {
	if _, held := maildropLocks.LoadOrStore(user.ID, true); held {
		return nil, errMaildropLocked
	}
	ctx := &context.Context{Context: stdcontext.Background(), UserID: user.ID, UserName: user.Name, UserAccount: user.Account, AuthVersion: user.AuthVersion}
	ctx.SetValue(context.LogID, id.GenLogID())
	var rows []struct {
		ID int `xorm:"id"`
	}
	if err := db.Instance.Table("email").Cols("id").Where(visibleMessage, consts.EmailTypeReceive, user.ID, consts.EmailStatusWait, models.INBOX).Desc("id").Find(&rows); err != nil {
		maildropLocks.Delete(user.ID)
		return nil, err
	}
	drop := &maildrop{ctx: ctx, messages: make([]messageRef, len(rows))}
	for i, row := range rows {
		drop.messages[i] = messageRef{id: row.ID, size: -1}
	}
	return drop, nil
}

func (d *maildrop) close()          { maildropLocks.Delete(d.ctx.UserID) }
func (d *maildrop) validate() error { return auth.ValidateSession(d.ctx.UserID, d.ctx.AuthVersion) }

func (d *maildrop) ref(number int) (*messageRef, error) {
	if number < 1 || number > len(d.messages) || d.messages[number-1].marked {
		return nil, errNoSuchMessage
	}
	return &d.messages[number-1], nil
}

func (d *maildrop) render(ref *messageRef) (string, error) {
	var mail models.Email
	// Even an administrator can only read mail belonging to this POP3 mailbox.
	// Recheck the association: a concurrent Web/IMAP delete or move can hide it.
	found, err := db.Instance.ID(ref.id).Where(visibleMessage, consts.EmailTypeReceive, d.ctx.UserID, consts.EmailStatusWait, models.INBOX).Get(&mail)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errNoSuchMessage
	}
	body := crlf.Replace(string(parsemail.NewEmailFromModel(mail).BuildBytes(d.ctx, false)))
	if !strings.HasSuffix(body, "\r\n") {
		body += "\r\n"
	}
	ref.size = int64(len(body))
	return body, nil
}

func (d *maildrop) size(ref *messageRef) (int64, error) {
	if ref.size < 0 {
		if _, err := d.render(ref); err != nil {
			return 0, err
		}
	}
	return ref.size, nil
}

func (d *maildrop) stat() (int, int64, error) {
	var count int
	var total int64
	for i := range d.messages {
		ref := &d.messages[i]
		if ref.marked {
			continue
		}
		n, err := d.size(ref)
		if errors.Is(err, errNoSuchMessage) {
			continue
		}
		if err != nil {
			return 0, 0, err
		}
		count++
		total += n
	}
	return count, total, nil
}

func (d *maildrop) list(number int, uid bool) ([]string, error) {
	line := func(number int, ref *messageRef) (string, error) {
		if uid {
			return strconv.Itoa(number) + " " + strconv.Itoa(ref.id), nil
		}
		size, err := d.size(ref)
		if err != nil {
			return "", err
		}
		return strconv.Itoa(number) + " " + strconv.FormatInt(size, 10), nil
	}
	if number != 0 {
		ref, err := d.ref(number)
		if err != nil {
			return nil, err
		}
		entry, err := line(number, ref)
		if err != nil {
			return nil, err
		}
		return []string{entry}, nil
	}
	out := make([]string, 0, len(d.messages))
	for i := range d.messages {
		ref := &d.messages[i]
		if ref.marked {
			continue
		}
		entry, err := line(i+1, ref)
		if errors.Is(err, errNoSuchMessage) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

func (d *maildrop) retr(number int) (string, error) {
	ref, err := d.ref(number)
	if err != nil {
		return "", err
	}
	return d.render(ref)
}

func topLines(message string, n int) string {
	pos := strings.Index(message, "\r\n\r\n")
	if pos < 0 {
		return message
	}
	end := pos + 4
	for range n {
		at := strings.Index(message[end:], "\r\n")
		if at < 0 {
			return message
		}
		end += at + 2
		if end == len(message) {
			break
		}
	}
	return message[:end]
}

func (d *maildrop) mark(number int) error {
	ref, err := d.ref(number)
	if err == nil {
		ref.marked = true
	}
	return err
}

func (d *maildrop) reset() {
	for i := range d.messages {
		d.messages[i].marked = false
	}
}

// QUIT removes marked messages from this user's POP3 maildrop. Consistent with
// Web deletion, keep them in Trash; never purge already-trashed or foreign rows.
func (d *maildrop) commit() error {
	var ids []int
	for _, ref := range d.messages {
		if ref.marked {
			ids = append(ids, ref.id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		for start := 0; start < len(ids); start += 500 {
			if _, err := tx.Table(&models.UserEmail{}).Where(inboxLink, d.ctx.UserID, consts.EmailStatusWait, models.INBOX).
				In("email_id", ids[start:min(start+500, len(ids))]).Update(map[string]any{"status": consts.EmailStatusDel, "group_id": 0}); err != nil {
				return err
			}
		}
		return nil
	})
}
