package controllers

import (
	"errors"
	"net/http"
	"sync"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/antivirus"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

var clamavWrite sync.Mutex // keep persisted updates and the live supervisor in order

func ClamAVSettings(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	m := antivirus.Active()
	if r.Method == http.MethodPut {
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		if body.Enabled == nil {
			inboundErr(ctx, w, inbound.ErrInvalid)
			return
		}
		clamavWrite.Lock()
		defer clamavWrite.Unlock()
		if err := inbound.SetClamAV(r.Context(), *body.Enabled); err != nil {
			if errors.Is(err, inbound.ErrClamAVEnv) {
				InboundError(ctx, w, 409, response.ParamsError, err.Error())
			} else {
				inboundErr(ctx, w, err)
			}
			return
		}
		if m != nil {
			m.SetEnabled(*body.Enabled)
		}
		log.WithContext(ctx).Infof("audit_event=clamav_setting_changed actor_id=%d enabled=%t", ctx.UserID, *body.Enabled)
	}
	if m != nil {
		s := m.Snapshot()
		_, s.EnvControlled = inbound.ClamAVOverride()
		inboundOK(w, s)
		return
	}
	enabled, controlled, err := inbound.ClamAVPolicy(r.Context())
	if err != nil {
		inboundErr(ctx, w, err)
		return
	}
	state := "disabled"
	if enabled {
		state = "error"
	}
	inboundOK(w, antivirus.Status{Enabled: enabled, EnvControlled: controlled, State: state})
}

func ClamAVUpdate(ctx *context.Context, w http.ResponseWriter, _ *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	m := antivirus.Active()
	if m == nil {
		InboundError(ctx, w, 409, response.ParamsError, antivirus.ErrDisabled.Error())
		return
	}
	if err := m.RequestUpdate(); err != nil {
		InboundError(ctx, w, 409, response.ParamsError, err.Error())
		return
	}
	log.WithContext(ctx).Infof("audit_event=clamav_update_requested actor_id=%d", ctx.UserID)
	inboundOK(w, map[string]bool{"queued": true})
}
