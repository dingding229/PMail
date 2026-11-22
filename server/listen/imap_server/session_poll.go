package imap_server

import (
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/models"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/spf13/cast"
)

func (s *serverSession) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error {
	if err := s.requireSession(); err != nil {
		return err
	}

	var ue []models.UserEmail
	db.Instance.Table("user_email").Where("user_id=? and "+db.Instance.Quote("create")+" >=?", s.ctx.UserID, s.connectTime).Find(&ue)

	if len(ue) > 0 {
		w.WriteNumMessages(cast.ToUint32(len(ue)))
	}

	return nil
}
