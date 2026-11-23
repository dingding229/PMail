package imap_server

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/utils/context"
	"github.com/Jinnrry/pmail/utils/intbounds"
	"github.com/emersion/go-imap/v2"
	"xorm.io/xorm"
)

type mailboxRef struct {
	owner, group int
	status       int8
	key, name    string
}
type mailboxEntry struct {
	row models.UserEmail
	uid imap.UID
	seq uint32
}
type mailboxState struct {
	validity uint32
	count    uint32
	next     imap.UID
	items    []mailboxEntry
}

var errMailboxNumber = errors.New("mailbox number exceeds IMAP range")

func protocolNumber(value int64) (uint32, error) {
	n, ok := intbounds.Uint32(value)
	if !ok {
		return 0, errMailboxNumber
	}
	return n, nil
}

var errMailboxMissing = errors.New("mailbox does not exist")

func protocolNO(err error) error {
	return &imap.Error{Type: imap.StatusResponseTypeNo, Text: err.Error()}
}

func findBox(tx *xorm.Session, owner int, name string) (mailboxRef, error) {
	if strings.EqualFold(name, "INBOX") {
		name = "INBOX"
	}
	sys := map[string]int8{"INBOX": 0, "Sent Messages": consts.EmailStatusSent, "Drafts": consts.EmailStatusDrafts, "Deleted Messages": consts.EmailStatusDel, "Junk": consts.EmailStatusJunk}
	if state, ok := sys[name]; ok {
		return mailboxRef{owner: owner, status: state, key: fmt.Sprintf("s:%d", state), name: name}, nil
	}
	if name == "" || len(name) > 600 {
		return mailboxRef{}, errMailboxMissing
	}
	parts := strings.Split(name, "/")
	if len(parts) > 16 {
		return mailboxRef{}, errMailboxMissing
	}
	var parent int
	for _, part := range parts {
		if part == "" {
			return mailboxRef{}, errMailboxMissing
		}
		var row models.Group
		ok, err := tx.Where("user_id=? AND parent_id=? AND name=?", owner, parent, part).Get(&row)
		if err != nil {
			return mailboxRef{}, err
		}
		if !ok {
			return mailboxRef{}, errMailboxMissing
		}
		parent = row.ID
	}
	return mailboxRef{owner: owner, group: parent, key: fmt.Sprintf("g:%d", parent), name: name}, nil
}

func (b mailboxRef) scope(tx *xorm.Session) *xorm.Session {
	tx = tx.Where("user_id=?", b.owner)
	if b.group > 0 {
		return tx.Where("group_id=? AND status<>?", b.group, consts.EmailStatusDel)
	}
	if b.status == consts.EmailStatusDel {
		return tx.Where("status=?", b.status)
	}
	if b.status == 0 {
		return tx.Where("status=? AND (group_id=0 OR group_id=?)", b.status, models.INBOX)
	}
	return tx.Where("group_id=0 AND status=?", b.status)
}

func mailboxTx(ctx *context.Context, fn func(*xorm.Session) error) error {
	return db.Transaction(db.Instance, func(tx *xorm.Session) error {
		if ctx.Context != nil {
			tx.Context(ctx.Context)
		}
		q := tx.ID(ctx.UserID).Cols("id")
		if db.Instance.DriverName() != "sqlite" {
			q.ForUpdate()
		}
		found, err := q.Get(&models.User{})
		if err != nil {
			return err
		}
		if !found {
			return errMailboxMissing
		}
		return fn(tx)
	})
}

// Sync membership on access, not by mutating shared email IDs. A surviving
// link keeps its UID; arriving/moved links receive a larger UID and removing
// the highest link cannot rewind the persisted watermark.
func syncMailbox(tx *xorm.Session, b mailboxRef) (mailboxState, error) {
	var state models.IMAPMailbox
	ok, err := tx.Where("user_id=? AND box_key=?", b.owner, b.key).Get(&state)
	if err != nil {
		return mailboxState{}, err
	}
	if !ok {
		state = models.IMAPMailbox{UserID: b.owner, BoxKey: b.key, NextUID: 1}
		if _, err = tx.Insert(&state); err != nil {
			return mailboxState{}, err
		}
	}
	if state.ID <= 0 || state.ID >= 1<<31 {
		return mailboxState{}, errors.New("mailbox UIDVALIDITY exhausted")
	}
	var links []models.UserEmail
	if err = b.scope(tx).Asc("id").Find(&links); err != nil {
		return mailboxState{}, err
	}
	count, err := protocolNumber(int64(len(links)))
	if err != nil {
		return mailboxState{}, err
	}
	var known []models.IMAPMailboxUID
	if err = tx.Where("box_id=?", state.ID).Find(&known); err != nil {
		return mailboxState{}, err
	}
	ids := make(map[int]int64, len(known))
	for _, m := range known {
		ids[m.LinkID] = m.UID
	}
	initial := state.NextUID
	uidValidity, err := protocolNumber(int64(state.ID))
	if err != nil {
		return mailboxState{}, err
	}
	result := mailboxState{count: count, validity: uidValidity | 0x80000000, items: make([]mailboxEntry, 0, len(links))}
	for _, link := range links {
		uid, ok := ids[link.ID]
		if !ok {
			if state.NextUID < 1 || state.NextUID >= math.MaxUint32 {
				return mailboxState{}, errors.New("mailbox UID exhausted")
			}
			uid = state.NextUID
			state.NextUID++
			if _, err = tx.Insert(&models.IMAPMailboxUID{BoxID: state.ID, LinkID: link.ID, UID: uid}); err != nil {
				return mailboxState{}, err
			}
		}
		delete(ids, link.ID)
		wireUID, err := protocolNumber(uid)
		if err != nil || wireUID == 0 || uid >= state.NextUID {
			return mailboxState{}, errors.New("invalid persisted IMAP UID")
		}
		result.items = append(result.items, mailboxEntry{row: link, uid: imap.UID(wireUID)})
	}
	stale := make([]int, 0, len(ids))
	for id := range ids {
		stale = append(stale, id)
	}
	for start := 0; start < len(stale); start += 500 {
		if _, err = tx.Where("box_id=?", state.ID).In("link_id", stale[start:min(start+500, len(stale))]).Delete(&models.IMAPMailboxUID{}); err != nil {
			return mailboxState{}, err
		}
	}
	if initial != state.NextUID {
		if _, err = tx.ID(state.ID).Cols("next_uid").Update(&state); err != nil {
			return mailboxState{}, err
		}
	}
	slices.SortFunc(result.items, func(a, b mailboxEntry) int {
		if a.uid < b.uid {
			return -1
		}
		if a.uid > b.uid {
			return 1
		}
		return 0
	})
	for i, seq := 0, uint32(1); i < len(result.items); i++ {
		result.items[i].seq = seq
		if seq < count {
			seq++
		}
	}
	next, err := protocolNumber(state.NextUID)
	if err != nil || next == 0 {
		return mailboxState{}, errors.New("invalid persisted IMAP UIDNEXT")
	}
	result.next = imap.UID(next)
	return result, nil
}

func mailboxSnapshot(ctx *context.Context, name string) (result mailboxState, err error) {
	err = mailboxTx(ctx, func(tx *xorm.Session) error {
		b, err := findBox(tx, ctx.UserID, name)
		if err != nil {
			return err
		}
		result, err = syncMailbox(tx, b)
		return err
	})
	return
}

func present(items []mailboxEntry) []*response.UserEmailUIDData {
	result := make([]*response.UserEmailUIDData, 0, len(items))
	for _, entry := range items {
		row := entry.row
		row.ID = int(entry.uid)
		result = append(result, &response.UserEmailUIDData{UserEmail: row, SerialNumber: int(entry.seq), LinkID: entry.row.ID})
	}
	return result
}

// A sequence number can only be interpreted relative to the last announced
// sequence list. A concurrent move must never retarget a client's sequence.
func (s *serverSession) visible(items []mailboxEntry) []mailboxEntry {
	if s.announced == nil {
		return items
	}
	byUID := make(map[imap.UID]mailboxEntry, len(items))
	for _, item := range items {
		byUID[item.uid] = item
	}
	out := make([]mailboxEntry, 0, len(items))
	for i, uid := range s.announced {
		if item, ok := byUID[uid]; ok {
			seq, err := protocolNumber(int64(i) + 1)
			if err != nil {
				return nil
			}
			item.seq = seq
			out = append(out, item)
		}
	}
	return out
}

func (s *serverSession) selectSet(items []mailboxEntry, set imap.NumSet) []mailboxEntry {
	seqMax, err := protocolNumber(int64(len(s.announced)))
	if err != nil {
		return nil
	}
	uidMax := uint32(0)
	if len(s.announced) > 0 {
		uidMax = uint32(s.announced[len(s.announced)-1])
	}
	if s.announced == nil && len(items) > 0 {
		seqMax, err = protocolNumber(int64(len(items)))
		if err != nil {
			return nil
		}
		uidMax = uint32(items[len(items)-1].uid)
	}
	selected := []mailboxEntry{}
	for _, item := range s.visible(items) {
		if set == nil || storeContains(set, item.seq, uint32(item.uid), seqMax, uidMax) {
			selected = append(selected, item)
		}
	}
	return selected
}

func uids(items []mailboxEntry) []imap.UID {
	out := make([]imap.UID, len(items))
	for i, item := range items {
		out[i] = item.uid
	}
	return out
}

func (s *serverSession) removeAnnounced(items []mailboxEntry, emit func(uint32) error) error {
	// Descending sequence numbers are still valid as each smaller number follows.
	for i := len(items) - 1; i >= 0; i-- {
		if err := emit(items[i].seq); err != nil {
			return err
		}
		if p := int(items[i].seq) - 1; p >= 0 && p < len(s.announced) {
			s.announced = slices.Delete(s.announced, p, p+1)
		}
	}
	return nil
}

// Search does not resynchronize an already announced mailbox. Read existing
// UID mappings and live flags in one projected query, without creating UIDs
// or holding an exclusive user lock (Poll announces newly arrived messages).
func (s *serverSession) searchSnapshot() ([]*response.UserEmailUIDData, error) {
	if s.announced == nil {
		state, err := mailboxSnapshot(s.ctx, s.currentMailbox)
		if err != nil {
			return nil, err
		}
		return present(state.items), nil
	}
	tx := db.Instance.NewSession()
	defer tx.Close()
	if s.ctx.Context != nil {
		tx.Context(s.ctx.Context)
	}
	b, err := findBox(tx, s.ctx.UserID, s.currentMailbox)
	if err != nil {
		return nil, err
	}
	var box models.IMAPMailbox
	ok, err := tx.Cols("id").Where("user_id=? AND box_key=?", b.owner, b.key).Get(&box)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errMailboxMissing
	}
	var links []response.UserEmailUIDData
	q := tx.Table("user_email").Join("INNER", "imap_mailbox_uid", "imap_mailbox_uid.link_id=user_email.id").Where("imap_mailbox_uid.box_id=?", box.ID)
	if err = b.scope(q).Select("imap_mailbox_uid.uid AS id,email_id,is_read,is_starred,status,imap_deleted").Asc("imap_mailbox_uid.uid").Find(&links); err != nil {
		return nil, err
	}
	rows := make([]*response.UserEmailUIDData, 0, min(len(links), len(s.announced)))
	// Both sets are UID ordered. Match them linearly and preserve the session's
	// sequence numbers when a concurrent expunge has not been announced yet.
	for i, j := 0, 0; i < len(links) && j < len(s.announced); {
		u, err := protocolNumber(int64(links[i].ID))
		if err != nil || u == 0 {
			return nil, errors.New("invalid persisted IMAP UID")
		}
		uid := imap.UID(u)
		switch {
		case uid < s.announced[j]:
			i++
		case uid > s.announced[j]:
			j++
		default:
			links[i].SerialNumber = j + 1
			rows = append(rows, &links[i])
			i++
			j++
		}
	}
	return rows, nil
}
