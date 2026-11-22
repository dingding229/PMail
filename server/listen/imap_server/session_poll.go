package imap_server

import (
	"slices"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
)

func (s *serverSession) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error {
	if err := s.requireSession(); err != nil {
		return err
	}
	if s.currentMailbox == "" {
		return nil
	}
	state, err := mailboxSnapshot(s.ctx, s.currentMailbox)
	if err != nil {
		return protocolNO(err)
	}
	current := uids(state.items)
	if slices.Equal(s.announced, current) {
		return nil
	}
	live := make(map[imap.UID]bool, len(current))
	for _, uid := range current {
		live[uid] = true
	}
	var removals []mailboxEntry
	for i, uid := range s.announced {
		if !live[uid] {
			removals = append(removals, mailboxEntry{seq: uint32(i + 1), uid: uid})
		}
	}
	if len(removals) > 0 && !allowExpunge {
		return nil
	}
	if err := s.removeAnnounced(removals, w.WriteExpunge); err != nil {
		return err
	}
	if len(current) != len(s.announced) {
		if err := w.WriteNumMessages(uint32(len(current))); err != nil {
			return err
		}
	}
	s.announced = current
	return nil
}
