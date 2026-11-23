package imap_server

import (
	"errors"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/list"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	log "github.com/sirupsen/logrus"
)

// Search implements the IMAP SEARCH command with full criteria support
// Supports: UID, SeqNum, Date filters, Header search, Body/Text search,
// Flag filters, Size filters, and logical combinations (NOT, OR)
func (s *serverSession) Search(kind imapserver.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) {
	if err := s.requireSession(); err != nil {
		return nil, err
	}
	log.WithContext(s.ctx).Debugf("IMAP SEARCH: mailbox=%s, kind=%v, criteria=%+v", s.currentMailbox, kind, criteria)

	// Use the new comprehensive search function
	rows, err := s.searchSnapshot()
	if err != nil {
		return nil, protocolNO(err)
	}
	var retList []*response.UserEmailUIDData
	if len(s.announced) > 0 {
		count, countErr := protocolNumber(int64(len(s.announced)))
		if countErr != nil {
			return nil, protocolNO(countErr)
		}
		retList, err = list.SearchSnapshot(s.ctx, rows, criteria, uint32(s.announced[len(s.announced)-1]), count)
	} else {
		retList, err = list.SearchRows(s.ctx, rows, criteria)
	}
	if err != nil {
		log.WithContext(s.ctx).Errorf("IMAP SEARCH error: %v", err)
		return nil, protocolNO(errors.New("mailbox search failed"))
	}

	count, err := protocolNumber(int64(len(retList)))
	if err != nil {
		return nil, protocolNO(err)
	}
	ret := &imap.SearchData{Count: count}
	var sequences imap.SeqSet
	var uids imap.UIDSet
	for _, row := range retList {
		number := row.ID
		if kind == imapserver.NumKindSeq {
			number = row.SerialNumber
		}
		n, err := protocolNumber(int64(number))
		if err != nil || n == 0 {
			return nil, protocolNO(errMailboxNumber)
		}
		if kind == imapserver.NumKindSeq {
			sequences = append(sequences, imap.SeqRange{Start: n, Stop: n})
		} else {
			uids = append(uids, imap.UIDRange{Start: imap.UID(n), Stop: imap.UID(n)})
		}
		if options != nil && options.ReturnMin && (ret.Min == 0 || n < ret.Min) {
			ret.Min = n
		}
		if options != nil && options.ReturnMax {
			ret.Max = max(ret.Max, n)
		}
	}
	if kind == imapserver.NumKindSeq {
		ret.All = sequences
	} else {
		ret.All = uids
	}

	log.WithContext(s.ctx).Debugf("IMAP SEARCH result: count=%d", ret.Count)
	return ret, nil
}
