package imap_server

import (
	"slices"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/del_email"
	"github.com/Jinnrry/pmail/services/detail"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
)

func mailFlags(read, starred bool, status int8) []imap.Flag {
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
	if status == consts.EmailStatusDel {
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

func mailboxLimits(rows []*response.UserEmailUIDData) (maxSeq, maxUID uint32) {
	for _, row := range rows {
		maxUID = max(maxUID, uint32(row.ID))
		maxSeq = max(maxSeq, uint32(row.SerialNumber))
	}
	return
}

func (s *serverSession) Store(w *imapserver.FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, _ *imap.StoreOptions) error {
	if err := s.requireSession(); err != nil {
		return err
	}
	rows := list.GetUEListByUID(s.ctx, s.currentMailbox, 0, 0, nil)
	maxSeq, maxUID := mailboxLimits(rows)
	read := flagChange(flags.Op, slices.Contains(flags.Flags, imap.FlagSeen))
	starred := flagChange(flags.Op, slices.Contains(flags.Flags, imap.FlagFlagged))
	deleteFlag := slices.Contains(flags.Flags, imap.FlagDeleted) && flags.Op != imap.StoreFlagsDel
	var ids, deleteUIDs []int
	for _, row := range rows {
		if storeContains(numSet, uint32(row.SerialNumber), uint32(row.ID), maxSeq, maxUID) {
			ids = append(ids, row.EmailID)
			if deleteFlag {
				deleteUIDs = append(deleteUIDs, row.ID)
			}
		}
	}
	if err := detail.SetFlags(s.ctx, ids, read, starred); err != nil {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "could not update message flags"}
	}
	if deleteFlag && len(deleteUIDs) > 0 {
		if err := del_email.DelByUID(s.ctx, deleteUIDs); err != nil {
			return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "could not delete messages"}
		}
	}
	if !flags.Silent {
		_, usingUID := numSet.(imap.UIDSet)
		for _, row := range rows {
			if !storeContains(numSet, uint32(row.SerialNumber), uint32(row.ID), maxSeq, maxUID) {
				continue
			}
			seen, star, status := row.IsRead == 1, row.IsStarred, row.Status
			if read != nil {
				seen = *read
			}
			if starred != nil {
				star = *starred
			}
			if deleteFlag {
				status = consts.EmailStatusDel
			}
			writer := w.CreateMessage(uint32(row.SerialNumber))
			if usingUID {
				writer.WriteUID(imap.UID(row.ID))
			}
			writer.WriteFlags(mailFlags(seen, star, status))
			writer.Close()
		}
	}
	return nil
}
