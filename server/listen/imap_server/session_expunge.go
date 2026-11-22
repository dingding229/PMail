package imap_server

import (
	"errors"

	"github.com/Jinnrry/pmail/models"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"xorm.io/xorm"
)

func (s *serverSession) expunge(set imap.NumSet, emit func(uint32) error) error {
	if s.readOnly {
		return protocolNO(errors.New("mailbox is read-only"))
	}
	var removed []mailboxEntry
	err := mailboxTx(s.ctx, func(tx *xorm.Session) error {
		removed = nil
		b, err := findBox(tx, s.ctx.UserID, s.currentMailbox)
		if err != nil {
			return err
		}
		state, err := syncMailbox(tx, b)
		if err != nil {
			return err
		}
		for _, item := range s.selectSet(state.items, set) {
			if !item.row.IMAPDeleted {
				continue
			}
			n, err := b.scope(tx).ID(item.row.ID).Where("imap_deleted=?", true).Delete(&models.UserEmail{})
			if err != nil {
				return err
			}
			if n > 0 {
				removed = append(removed, item)
			}
		}
		if len(removed) > 0 {
			_, err = syncMailbox(tx, b)
		}
		return err
	})
	if err != nil {
		return protocolNO(err)
	}
	if len(removed) > 0 {
		notifyUser(s.ctx.UserID)
	}
	return s.removeAnnounced(removed, emit)
}

func (s *serverSession) Expunge(w *imapserver.ExpungeWriter, uids *imap.UIDSet) error {
	if err := s.requireSession(); err != nil {
		return err
	}
	var set imap.NumSet
	if uids != nil {
		set = *uids
	}
	return s.expunge(set, w.WriteExpunge)
}
