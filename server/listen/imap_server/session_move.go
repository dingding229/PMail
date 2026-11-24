package imap_server

import (
	"errors"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
)

func (s *serverSession) Move(w *imapserver.MoveWriter, set imap.NumSet, dest string) error {
	if err := s.requireSession(); err != nil {
		return err
	}
	if s.readOnly {
		return protocolNO(errors.New("mailbox is read-only"))
	}
	copied, removed, err := s.transfer(set, dest, true)
	if err != nil {
		return protocolNO(err)
	}
	if copied == nil {
		return nil
	}
	if err := w.WriteCopyData(copied); err != nil {
		return err
	}
	return s.removeAnnounced(removed, w.WriteExpunge)
}
