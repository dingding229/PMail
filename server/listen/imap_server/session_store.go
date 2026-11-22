package imap_server

import (
	"slices"

	"errors"
	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/models"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"xorm.io/xorm"
)

func mailFlags(read, starred bool, status int8, deleted ...bool) []imap.Flag {
	var flags []imap.Flag
	if read {
		flags = append(flags, imap.FlagSeen)
	}
	if starred {
		flags = append(flags, imap.FlagFlagged)
	}
	if status == consts.EmailStatusDrafts {
		flags = append(flags, imap.FlagDraft)
	}
	if len(deleted) > 0 && deleted[0] {
		flags = append(flags, imap.FlagDeleted)
	}
	return flags
}

func flagChange(op imap.StoreFlagsOp, included bool) *bool {
	if op == imap.StoreFlagsSet || included {
		v := included && op != imap.StoreFlagsDel
		return &v
	}
	return nil
}

func storeContains(set imap.NumSet, seq, uid, maxSeq, maxUID uint32) bool {
	within := func(n, lo, hi, largest uint32) bool {
		if lo == 0 {
			lo = largest
		}
		if hi == 0 {
			hi = largest
		}
		if lo > hi {
			lo, hi = hi, lo
		}
		return n >= lo && n <= hi
	}
	switch set := set.(type) {
	case imap.SeqSet:
		for _, x := range set {
			if within(seq, x.Start, x.Stop, maxSeq) {
				return true
			}
		}
	case imap.UIDSet:
		for _, x := range set {
			if within(uid, uint32(x.Start), uint32(x.Stop), maxUID) {
				return true
			}
		}
	}
	return false
}

func (s *serverSession) Store(w *imapserver.FetchWriter, set imap.NumSet, flags *imap.StoreFlags, _ *imap.StoreOptions) error {
	if err := s.requireSession(); err != nil {
		return err
	}
	if s.readOnly {
		return protocolNO(errors.New("mailbox is read-only"))
	}
	read := flagChange(flags.Op, slices.Contains(flags.Flags, imap.FlagSeen))
	starred := flagChange(flags.Op, slices.Contains(flags.Flags, imap.FlagFlagged))
	deleted := flagChange(flags.Op, slices.Contains(flags.Flags, imap.FlagDeleted))
	var changed []mailboxEntry
	err := mailboxTx(s.ctx, func(tx *xorm.Session) error {
		b, err := findBox(tx, s.ctx.UserID, s.currentMailbox)
		if err != nil {
			return err
		}
		state, err := syncMailbox(tx, b)
		if err != nil {
			return err
		}
		changed = s.selectSet(state.items, set)
		values := map[string]any{}
		if read != nil {
			v := 0
			if *read {
				v = 1
			}
			values["is_read"] = v
		}
		if starred != nil {
			values["is_starred"] = *starred
		}
		if deleted != nil {
			values["imap_deleted"] = *deleted
		}
		if len(values) == 0 {
			return nil
		}
		for _, entry := range changed {
			if _, err := b.scope(tx).ID(entry.row.ID).Table(&models.UserEmail{}).Update(values); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return protocolNO(err)
	}
	if flags.Silent || w == nil {
		return nil
	}
	_, usingUID := set.(imap.UIDSet)
	for _, entry := range changed {
		seen, star, del := entry.row.IsRead == 1, entry.row.IsStarred, entry.row.IMAPDeleted
		if read != nil {
			seen = *read
		}
		if starred != nil {
			star = *starred
		}
		if deleted != nil {
			del = *deleted
		}
		writer := w.CreateMessage(entry.seq)
		if usingUID {
			writer.WriteUID(entry.uid)
		}
		writer.WriteFlags(mailFlags(seen, star, entry.row.Status, del))
		if err := writer.Close(); err != nil {
			return err
		}
	}
	return nil
}
