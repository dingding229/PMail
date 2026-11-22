package imap_server

import (
	"errors"

	"github.com/Jinnrry/pmail/models"
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
		ar := make([]int, 0, len(selected))
		for _, entry := range selected {
			original := entry.row
			next := models.UserEmail{UserID: s.ctx.UserID, EmailID: original.EmailID, IsRead: original.IsRead, IsStarred: original.IsStarred, IMAPDeleted: original.IMAPDeleted, GroupId: target.group, Status: target.status}
			if move {
				next.IMAPDeleted = false
			}
			if _, err := tx.Insert(&next); err != nil {
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
				removed = append(removed, entry)
			}
		}
		if move {
			if _, err := syncMailbox(tx, src); err != nil {
				return err
			}
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
	notifyUser(s.ctx.UserID)
	return copied, nil
}
