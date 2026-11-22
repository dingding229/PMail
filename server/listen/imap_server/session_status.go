package imap_server

import "github.com/emersion/go-imap/v2"

func (s *serverSession) Status(mailbox string, options *imap.StatusOptions) (*imap.StatusData, error) {
	if err := s.requireSession(); err != nil {
		return nil, err
	}
	state, err := mailboxSnapshot(s.ctx, mailbox)
	if err != nil {
		return nil, protocolNO(err)
	}
	count := uint32(len(state.items))
	var unseen uint32
	for _, item := range state.items {
		if item.row.IsRead == 0 {
			unseen++
		}
	}
	data := &imap.StatusData{Mailbox: mailbox}
	if options != nil {
		if options.NumMessages {
			data.NumMessages = &count
		}
		if options.NumUnseen {
			data.NumUnseen = &unseen
		}
		if options.UIDNext {
			data.UIDNext = state.next
		}
		if options.UIDValidity {
			data.UIDValidity = state.validity
		}
	}
	return data, nil
}
