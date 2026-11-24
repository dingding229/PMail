package imap_server

import (
	"errors"
	"time"

	"github.com/Jinnrry/pmail/consts"
	"github.com/Jinnrry/pmail/services/del_email"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/mailaddress"
	"github.com/Jinnrry/pmail/services/quota"
	"github.com/emersion/go-imap/v2"
	"xorm.io/xorm"
)

func (s *serverSession) transfer(set imap.NumSet, dest string, move bool) (*imap.CopyData, []mailboxEntry, error) {
	var removed []mailboxEntry
	var copied *imap.CopyData
	err := mailboxTx(s.ctx, func(tx *xorm.Session) error {
		removed = nil
		copied = nil
		src, err := findBox(tx, s.ctx.UserID, s.currentMailbox)
		if err != nil {
			return err
		}
		target, err := findBox(tx, s.ctx.UserID, dest)
		if err != nil {
			return err
		}
		if move && src.key == target.key {
			return errors.New("move destination must be a different mailbox")
		}
		from, err := syncMailbox(tx, src)
		if err != nil {
			return err
		}
		selected := s.selectSet(from.items, set)
		if len(selected) == 0 {
			return nil
		}
		var added int64
		if !move {
			if _, err := quota.Lock(tx, s.ctx.UserID); err != nil {
				return err
			}
			ids := make([]int, 0, len(selected))
			for _, e := range selected {
				ids = append(ids, e.row.EmailID)
			}
			sizes, err := quota.MessageSizes(tx, ids)
			if err != nil {
				return err
			}
			for _, id := range ids {
				added += sizes[id]
			}
		}
		now := time.Now()
		ar := make([]int, 0, len(selected))
		for _, entry := range selected {
			original := entry.row
			next := models.UserEmail{UserID: s.ctx.UserID, EmailID: original.EmailID, IsRead: original.IsRead, IsStarred: original.IsStarred, InboundSpam: original.InboundSpam, IMAPDeleted: original.IMAPDeleted, GroupId: target.group, Status: target.status, CatchAllAddresses: original.CatchAllAddresses, AddressAttributed: true}
			if target.status == consts.EmailStatusDel {
				del_email.TrashCopy(&next, original, now)
			}
			if move {
				next.IMAPDeleted = false
			}
			if _, err := tx.Insert(&next); err != nil {
				return err
			}
			if err := mailaddress.Copy(tx, original, next); err != nil {
				return err
			}
			ar = append(ar, next.ID)
			if move {
				n, err := src.scope(tx).ID(original.ID).Delete(&models.UserEmail{})
				if err != nil {
					return err
				}
				if n != 1 {
					return errors.New("source membership changed")
				}
				if err := mailaddress.DeleteLinks(tx, []int{original.ID}); err != nil {
					return err
				}
				if err := del_email.ForgetUIDs(tx, []int{original.ID}); err != nil {
					return err
				}
				removed = append(removed, entry)
			}
		}
		if move {
			if _, err := syncMailbox(tx, src); err != nil {
				return err
			}
		}
		if err := quota.Adjust(tx, s.ctx.UserID, added); err != nil {
			return err
		}
		to, err := syncMailbox(tx, target)
		if err != nil {
			return err
		}
		ids := map[int]imap.UID{}
		for _, entry := range to.items {
			ids[entry.row.ID] = entry.uid
		}
		copied = &imap.CopyData{UIDValidity: to.validity}
		for i, entry := range selected {
			uid := ids[ar[i]]
			if uid == 0 {
				return errors.New("destination UID was not assigned")
			}
			copied.SourceUIDs.AddNum(entry.uid)
			copied.DestUIDs.AddNum(uid)
		}
		return nil
	})
	if err == nil && copied != nil {
		MailboxChanged(s.ctx.UserID)
	}
	return copied, removed, err
}

func (s *serverSession) Copy(set imap.NumSet, dest string) (*imap.CopyData, error) {
	if err := s.requireSession(); err != nil {
		return nil, err
	}
	copied, _, err := s.transfer(set, dest, false)
	if err != nil {
		return nil, protocolNO(err)
	}
	return copied, nil
}
