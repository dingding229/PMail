package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/listen/smtp_server"
	"github.com/Jinnrry/pmail/models"
	"github.com/Jinnrry/pmail/services/address"
	"github.com/Jinnrry/pmail/services/domain"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func AddressApprovalAPIPath(path string) bool {
	return path == "/api/admin/overview" || path == "/api/admin/address-settings" || path == "/api/admin/address-requests" || strings.HasPrefix(path, "/api/admin/address-requests/")
}

func addressApprovalError(ctx *context.Context, w http.ResponseWriter, err error) {
	for _, pair := range []struct {
		err    error
		status int
		key    string
	}{
		{address.ErrPendingLimit, 409, "address_pending_limit"},
		{address.ErrAlreadyReviewed, 409, "address_already_reviewed"},
		{address.ErrInactive, 409, "address_inactive"},
		{address.ErrRejectReason, 400, "address_reject_reason_invalid"},
		{address.ErrInvalid, 400, "invalid_address"},
		{address.ErrNotFound, 404, "address_not_found"},
		{address.ErrExists, 409, "address_exists"},
		{domain.ErrNotFound, 404, "domain_not_found"},
		{domain.ErrInvalid, 400, "invalid_domain"},
	} {
		if errors.Is(err, pair.err) {
			InboundError(ctx, w, pair.status, response.ParamsError, pair.key)
			return
		}
	}
	log.WithContext(ctx).Error("address approval operation failed")
	InboundError(ctx, w, http.StatusInternalServerError, response.ServerError, "internal_error")
}

func AdminAddressSettings(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	var required bool
	if r.Method == http.MethodPut {
		var body struct {
			Required *bool `json:"address_approval_required"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		if body.Required == nil {
			addressApprovalError(ctx, w, address.ErrInvalid)
			return
		}
		required = *body.Required
		if err := address.SetApprovalRequired(r.Context(), required); err != nil {
			addressApprovalError(ctx, w, err)
			return
		}
		log.WithContext(ctx).Infof("audit_event=address_approval_policy_changed actor_id=%d address_approval_required=%t", ctx.UserID, required)
	} else {
		var err error
		required, err = address.ApprovalRequired(r.Context())
		if err != nil {
			addressApprovalError(ctx, w, err)
			return
		}
	}
	inboundOK(w, models.AddressSettings{AddressApprovalRequired: required})
}

func AdminOverview(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	count, err := address.PendingCount(r.Context())
	if err != nil {
		addressApprovalError(ctx, w, err)
		return
	}
	inboundOK(w, map[string]int64{"pending_address_requests": count})
}

func AdminAddressRequests(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	q := r.URL.Query()
	status := q.Get("status")
	if status == "" {
		status = models.AddressPending
	}
	page, size := 1, 20
	for key, out := range map[string]*int{"page": &page, "page_size": &size} {
		if q.Has(key) {
			var err error
			*out, err = strconv.Atoi(q.Get(key))
			if err != nil {
				addressApprovalError(ctx, w, address.ErrInvalid)
				return
			}
		}
	}
	result, err := address.Requests(r.Context(), status, page, size)
	if err != nil {
		addressApprovalError(ctx, w, err)
		return
	}
	inboundOK(w, result)
}

func AdminApproveAddress(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	reviewAddress(ctx, w, r, true)
}
func AdminRejectAddress(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	reviewAddress(ctx, w, r, false)
}
func reviewAddress(ctx *context.Context, w http.ResponseWriter, r *http.Request, approve bool) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		addressApprovalError(ctx, w, address.ErrInvalid)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if !approve && r.ContentLength != 0 && !inboundBody(ctx, w, r, &body) {
		return
	}
	row, err := address.Review(r.Context(), id, ctx.UserID, approve, body.Reason)
	if err != nil {
		addressApprovalError(ctx, w, err)
		return
	}
	log.WithContext(ctx).Infof("audit_event=address_request_reviewed actor_id=%d address_id=%d applicant_id=%d status=%s", ctx.UserID, id, row.UserID, row.Status)
	if err := smtp_server.DeliverAddressReview(r.Context(), row); err != nil {
		// The review is already committed. A full mailbox, unavailable primary,
		// or a canceled request must not roll back or resubmit the decision.
		log.WithContext(ctx).Warnf("address review notice not delivered address_id=%d applicant_id=%d", id, row.UserID)
	}
	inboundOK(w, apiAddress(row))
}
