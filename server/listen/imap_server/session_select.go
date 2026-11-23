package imap_server

import "github.com/emersion/go-imap/v2"

func (s *serverSession) Select(mailbox string, options *imap.SelectOptions) (*imap.SelectData, error) {
	if err := s.requireSession(); err != nil {
		return nil, err
	}
	state, err := mailboxSnapshot(s.ctx, mailbox)
	if err != nil {
		return nil, protocolNO(err)
	}
	s.currentMailbox = mailbox
	s.readOnly = options != nil && options.ReadOnly
	s.announced = uids(state.items)
	flags := []imap.Flag{imap.FlagSeen, imap.FlagFlagged, imap.FlagDeleted}
	return &imap.SelectData{Flags: flags, PermanentFlags: flags, NumMessages: state.count, UIDNext: state.next, UIDValidity: state.validity}, nil
}
