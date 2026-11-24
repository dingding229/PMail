package mailarchive

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto"
	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/services/mailstore"
	"github.com/Jinnrry/pmail/services/quota"
	pctx "github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/mailbudget"
	"xorm.io/xorm"
)

type Folder struct {
	Key    string
	ID     int
	Path   string
	Status int8
}

var builtins = []Folder{
	{"inbox", 0, "Inbox", consts.EmailStatusWait}, {"sent", 0, "Sent", consts.EmailStatusSent}, {"drafts", 0, "Drafts", consts.EmailStatusDrafts},
	{"junk", 0, "Junk", consts.EmailStatusJunk}, {"trash", 0, "Trash", consts.EmailStatusDel},
}

func byKey(k string) (Folder, bool) {
	for _, f := range builtins {
		if f.Key == k {
			return f, true
		}
	}
	return Folder{}, false
}

func custom(ctx context.Context, owner int) (map[int]Folder, error) {
	var rows []models.Group
	if err := db.Instance.Context(ctx).Where("user_id=?", owner).Find(&rows); err != nil {
		return nil, err
	}
	all := map[int]models.Group{}
	for _, r := range rows {
		all[r.ID] = r
	}
	out := map[int]Folder{}
	for id, r := range all {
		paths := []string{r.Name}
		seen := map[int]bool{id: true}
		for p := r.ParentId; p > 0 && len(paths) < 16; {
			x, ok := all[p]
			if !ok || seen[p] {
				break
			}
			seen[p] = true
			paths = append(paths, x.Name)
			p = x.ParentId
		}
		slices.Reverse(paths)
		out[id] = Folder{Key: fmt.Sprintf("g:%d", id), ID: id, Path: strings.Join(paths, "/"), Status: consts.EmailStatusWait}
	}
	return out, nil
}

func pickFolder(tag string, groups map[int]Folder) (Folder, error) {
	if tag == "" {
		return builtins[0], nil
	}
	if f, ok := byKey(tag); ok {
		return f, nil
	}
	if id, ok := strings.CutPrefix(tag, "group:"); ok {
		n, err := strconv.Atoi(id)
		if f, found := groups[n]; err == nil && found {
			return f, nil
		}
		return Folder{}, ErrTarget
	}
	if len(tag) > 256 {
		return Folder{}, ErrTarget
	}
	t := dto.SearchTag{Type: -1, Status: -1, GroupId: -1}
	if json.Unmarshal([]byte(tag), &t) != nil || t.Starred {
		return Folder{}, ErrTarget
	}
	if f, ok := groups[t.GroupId]; ok {
		return f, nil
	}
	switch t.GroupId {
	case models.INBOX:
		return builtins[0], nil
	case models.Sent:
		return builtins[1], nil
	case models.Drafts:
		return builtins[2], nil
	case models.Junk:
		return builtins[3], nil
	case models.Deleted:
		return builtins[4], nil
	}
	if t.GroupId > 0 {
		return Folder{}, ErrTarget
	}
	switch t.Status {
	case consts.EmailStatusDrafts:
		return builtins[2], nil
	case consts.EmailStatusJunk:
		return builtins[3], nil
	case consts.EmailStatusDel:
		return builtins[4], nil
	}
	if t.Type == consts.EmailTypeSend {
		return builtins[1], nil
	}
	if t.GroupId == 0 || t.Type == consts.EmailTypeReceive {
		return builtins[0], nil
	}
	return Folder{}, ErrTarget
}
func ExportFolders(ctx context.Context, owner int, tags []string, trash bool) ([]Folder, error) {
	if len(tags) > 256 {
		return nil, ErrTarget
	}
	groups, err := custom(ctx, owner)
	if err != nil {
		return nil, err
	}
	var out []Folder
	if len(tags) == 0 {
		out = append(out, builtins[:4]...)
		if trash {
			out = append(out, builtins[4])
		}
		var ids []int
		for id := range groups {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			out = append(out, groups[id])
		}
		return out, nil
	}
	seen := map[string]bool{}
	for _, s := range tags {
		f, err := pickFolder(s, groups)
		if err != nil {
			return nil, err
		}
		if f.Key == "trash" && !trash {
			return nil, ErrTarget
		}
		if !seen[f.Key] {
			out = append(out, f)
			seen[f.Key] = true
		}
	}
	if trash && !seen["trash"] {
		out = append(out, builtins[4])
	}
	return out, nil
}
func ImportFolder(ctx context.Context, owner int, tag string) (Folder, error) {
	g, err := custom(ctx, owner)
	if err != nil {
		return Folder{}, err
	}
	return pickFolder(tag, g)
}

func safeLeaf(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_ .", r) {
			return r
		}
		return '_'
	}, s)
	r := []rune(strings.Trim(s, " ."))
	if len(r) == 0 {
		return "Folder"
	}
	return string(r[:min(120, len(r))])
}
func filename(f Folder) string {
	if f.ID == 0 {
		return f.Path + ".mbox"
	}
	return fmt.Sprintf("Folders/%s--%d.mbox", safeLeaf(strings.ReplaceAll(f.Path, "/", "__")), f.ID)
}
func scope(f Folder) (string, []any) {
	if f.ID > 0 {
		return "ue.group_id=? AND ue.status<>?", []any{f.ID, consts.EmailStatusDel}
	}
	switch f.Key {
	case "inbox":
		return "ue.status=0 AND e.type=0 AND ue.group_id IN (0,?)", []any{models.INBOX}
	case "sent":
		return "(ue.group_id IN (0,?,?) AND ue.status IN (0,1,2,6) AND (e.type=1 OR ue.status=1))", []any{models.INBOX, models.Sent}
	case "trash":
		return "ue.status=3", nil
	case "drafts":
		return "(ue.status=4 OR ue.group_id=?) AND ue.status<>3", []any{models.Drafts}
	case "junk":
		return "(ue.status=5 OR ue.group_id=?) AND ue.status<>3", []any{models.Junk}
	}
	return "1=0", nil
}

func countExport(ctx context.Context, owner int, folders []Folder) (int64, error) {
	var total int64
	for _, f := range folders {
		cond, args := scope(f)
		var n int64
		if _, err := db.Instance.Context(ctx).SQL("SELECT COUNT(*) FROM user_email ue JOIN email e ON e.id=ue.email_id WHERE ue.user_id=? AND ("+cond+")", append([]any{owner}, args...)...).Get(&n); err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

func Export(ctx context.Context, owner int, folders []Folder, out io.Writer, done func()) error {
	z := zip.NewWriter(out)
	for _, f := range folders {
		dst, err := z.Create(filename(f))
		if err != nil {
			return err
		}
		cond, args := scope(f)
		cursor := 0
		for ctx.Err() == nil {
			var links []models.UserEmail
			q := "SELECT ue.id,ue.email_id,ue.is_read,ue.is_starred FROM user_email ue JOIN email e ON e.id=ue.email_id WHERE ue.user_id=? AND ue.id>? AND (" + cond + ") ORDER BY ue.id LIMIT 64"
			if err := db.Instance.Context(ctx).SQL(q, append([]any{owner, cursor}, args...)...).Find(&links); err != nil {
				return err
			}
			if len(links) == 0 {
				break
			}
			for _, l := range links {
				cursor = l.ID
				var e models.Email
				found, err := db.Instance.Context(ctx).ID(l.EmailID).Where("EXISTS (SELECT 1 FROM user_email WHERE id=? AND user_id=? AND email_id=?)", l.ID, owner, l.EmailID).Get(&e)
				if err != nil {
					return err
				}
				if !found {
					continue
				}
				wire, err := mailstore.POPMessage(&pctx.Context{Context: ctx, UserID: owner}, e)
				if err != nil {
					return err
				}
				if err := writeMbox(dst, wire, l.IsRead != 0, l.IsStarred, e.SendDate); err != nil {
					return err
				}
				if done != nil {
					done()
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return z.Close()
}

// Import directly persists mail and its projections. It does not call the
// delivery services, thus not a path to any inbound or outbound side effect.
func Import(ctx context.Context, owner int, f Folder, path, ext string, budget int64, count func(string)) error {
	one := func(raw []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := importMessage(ctx, owner, f, raw)
		if errors.Is(err, ErrQuota) || errors.Is(err, ErrTarget) {
			if count != nil {
				count("failed")
			}
			return err
		}
		if err != nil {
			result = "failed"
		}
		if count != nil {
			count(result)
		}
		return nil
	}
	visit := func(ext string, in io.Reader) error {
		switch ext {
		case ".mbox":
			return readMbox(ctx, in, one)
		case ".eml":
			raw, err := mailbudget.Read(in, MessageLimit)
			if errors.Is(err, mailbudget.ErrLimit) {
				if count != nil {
					count("failed")
				}
				return nil
			}
			if err != nil {
				return err
			}
			return one(raw)
		default:
			return ErrInvalid
		}
	}
	if ext == ".zip" {
		f, err := os.Open(path) // #nosec G304 -- path is the server-created temporary upload file.
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if err = boundZipDirectory(f, info.Size()); err != nil {
			return err
		}
		z, err := zip.NewReader(f, info.Size())
		if err != nil {
			return ErrInvalid
		}
		if err = validZip(ctx, z.File, budget); err != nil {
			return err
		}
		return visitInput(ctx, z.File, visit)
	}
	r, err := os.Open(path) // #nosec G304 -- path is the server-created temporary upload file.
	if err != nil {
		return err
	}
	defer r.Close()
	return visit(ext, r)
}

func importMessage(ctx context.Context, owner int, f Folder, b []byte) (string, error) {
	if len(b) == 0 || len(b) > MessageLimit {
		return "", ErrInvalid
	}
	header, err := mail.ReadMessage(bytes.NewReader(b))
	if err != nil {
		return "", ErrInvalid
	}
	p, err := parsemail.ParseLimited(nil, bytes.NewReader(b), len(b))
	if err != nil || p == nil || p.From == nil || p.From.EmailAddress == "" {
		return "", ErrInvalid
	}
	mid := p.MsgID
	if len(mid) > 255 {
		return "", ErrInvalid
	}
	created := time.Now()
	date, err := mail.ParseDate(header.Header.Get("Date"))
	if err != nil {
		date = created
	}
	serial := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	trim := func(v string, n int) string { r := []rune(v); return string(r[:min(n, len(r))]) }
	vals := models.Email{MsgID: mid, Subject: trim(p.Subject, 1000), FromName: trim(p.From.Name, 50), FromAddress: trim(p.From.EmailAddress, 100),
		To: serial(p.To), Cc: serial(p.Cc), Bcc: serial(p.Bcc), ReplyTo: serial(p.ReplyTo), Sender: serial(p.Sender), Attachments: serial(p.Attachments),
		Text: sql.NullString{String: string(p.Text), Valid: len(p.Text) > 0}, Html: sql.NullString{String: string(p.HTML), Valid: len(p.HTML) > 0},
		SendDate: date, CreateTime: created, Size: len(b), AuthUnchecked: true}
	if f.Key == "drafts" {
		// The edit/send path validates the backing draft as well as its owner link.
		vals.Status, vals.SendUserID = consts.EmailStatusDrafts, owner
	}

	read := int8(0)
	if strings.Contains(strings.ToUpper(header.Header.Get("Status")), "R") {
		read = 1
	}
	link := models.UserEmail{UserID: owner, GroupId: f.ID, Status: f.Status, IsRead: read, IsStarred: strings.Contains(strings.ToUpper(header.Header.Get("X-Status")), "F"), AddressAttributed: true}
	if f.Key == "trash" {
		ms := created.UnixMilli()
		link.DeletedAt = &ms
	}
	state := "success"
	err = db.Transaction(db.Instance, func(tx *xorm.Session) error {
		// A retry must use fresh IDs and state after a rolled-back attempt.
		e, l := vals, link
		state = "success"
		tx.Context(ctx)
		u, err := quota.Lock(tx, owner)
		if err != nil || u.Disabled != 0 {
			if err != nil {
				return err
			}
			return ErrNotFound
		}
		if f.ID > 0 {
			q := tx.ID(f.ID).Where("user_id=?", owner).Cols("id")
			if tx.Engine().DriverName() != "sqlite" {
				q.ForUpdate()
			}
			ok, e := q.Get(&models.Group{})
			if e != nil {
				return e
			}
			if !ok {
				return ErrTarget
			}
		}
		if mid != "" {
			var id int
			ok, e := tx.SQL("SELECT e.id FROM email e JOIN user_email ue ON e.id=ue.email_id WHERE ue.user_id=? AND e.msg_id=? LIMIT 1", owner, mid).Get(&id)
			if e != nil {
				return e
			}
			if ok {
				state = "skipped"
				return nil
			}
		}
		if err := quota.LockAndCheck(tx, owner, int64(len(b))); err != nil {
			if errors.Is(err, quota.ErrExceeded) {
				return ErrQuota
			}
			return err
		}
		var targets []string
		for _, x := range append(slices.Clone(p.To), p.Cc...) {
			if x != nil {
				targets = append(targets, x.EmailAddress)
			}
		}
		if f.Key == "sent" {
			if local, host, e2 := address.Split(p.From.EmailAddress); e2 == nil {
				var id int
				own, e2 := tx.SQL("SELECT m.id FROM mailbox_address m JOIN domain d ON d.id=m.domain_id WHERE m.user_id=? AND m.local_part=? AND d.name=? AND m.status=? LIMIT 1", owner, local, host, models.AddressActive).Get(&id)
				if e2 != nil {
					return e2
				}
				if own {
					e.Type = consts.EmailTypeSend
					e.Status = consts.EmailStatusSent
					e.SendUserID = owner
					targets = []string{p.From.EmailAddress}
				}
			}
		}
		mailstore.Prepare(&e)
		mailstore.SetPOPSize(&pctx.Context{Context: ctx, UserID: owner}, &e)
		if _, err := tx.Insert(&e); err != nil {
			return err
		}
		if err := mailstore.WriteParts(tx, &e, false); err != nil {
			return err
		}
		l.EmailID = e.Id
		if _, err := tx.Insert(&l); err != nil {
			return err
		}
		if err := mailaddress.Add(tx, []models.UserEmail{l}, targets); err != nil {
			return err
		}
		return quota.Adjust(tx, owner, int64(len(b)))
	})
	return state, err
}

func extension(name string) string {
	switch ext := strings.ToLower(filepath.Ext(name)); ext {
	case ".mbox", ".eml", ".zip":
		return ext
	}
	return ""
}
