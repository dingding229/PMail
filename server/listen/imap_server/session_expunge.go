package imap_server

import (
	"errors"

	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/del_email"
	"github.com/Jinnrry/pmail/services/quota"
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
		if _, err := quota.Lock(tx, s.ctx.UserID); err != nil {
			return err
		}
		b, err := findBox(tx, s.ctx.UserID, s.currentMailbox)
		if err != nil {
			return err
		}
		state, err := syncMailbox(tx, b)
		if err != nil {
			return err
		}
		var purge []models.UserEmail
		for _, item := range s.selectSet(state.items, set) {
			if item.row.IMAPDeleted {
				purge = append(purge, item.row)
				removed = append(removed, item)
			}
		}
		if _, err := del_email.PurgeLinks(tx, s.ctx.UserID, purge); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return protocolNO(err)
	}
	if len(removed) > 0 {
		MailboxChanged(s.ctx.UserID)
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
