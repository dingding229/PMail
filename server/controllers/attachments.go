package controllers

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/attachments"
	"github.com/Jinnrry/pmail/services/inbound"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func GetAttachments(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	serveAttachment(ctx, w, r, lookupAttachment(ctx, r, false), false)
}

func Download(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	serveAttachment(ctx, w, r, lookupAttachment(ctx, r, true), true)
}

func lookupAttachment(ctx *context.Context, r *http.Request, download bool) *attachments.Part {
	if id := r.PathValue("id"); id != "" {
		mail, index, ok := strings.Cut(id, ".")
		if ok {
			return attachmentByIndex(ctx, mail, index)
		}
		return nil
	}
	mail := r.PathValue("emailId")
	if download {
		return attachmentByIndex(ctx, mail, r.PathValue("index"))
	}
	if emailID, err := strconv.Atoi(mail); err == nil && emailID > 0 {
		return attachments.Find(ctx, emailID, -1, r.PathValue("cid"))
	}
	return nil
}

func attachmentByIndex(ctx *context.Context, mail, index string) *attachments.Part {
	emailID, mailErr := strconv.Atoi(mail)
	attachmentIndex, indexErr := strconv.Atoi(index)
	if mailErr != nil || indexErr != nil || emailID <= 0 || attachmentIndex < 0 {
		return nil
	}
	return attachments.Find(ctx, emailID, attachmentIndex, "")
}

func serveAttachment(ctx *context.Context, w http.ResponseWriter, r *http.Request, attachment *attachments.Part, download bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !download {
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	if attachment == nil || attachment.Size == 0 {
		response.Localized(ctx, response.ParamsError, "attachment_not_found").FPrint(w)
		return
	}
	if attachment.HighRisk {
		confirmed := r.URL.Query().Get("confirm_risk") == "1"
		auditRisk(ctx, r, attachment, "download", confirmed)
		if !confirmed {
			InboundError(ctx, w, 409, response.ParamsError, "risk_confirmation_required")
			return
		}
		download = true // Acknowledgement never enables inline rendering.
	}
	contentType, inline := attachments.InlineContentType(attachment.ContentType)
	if download {
		contentType, inline = "application/octet-stream", false
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(attachment.Size))
	if !inline {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Filename}))
	}
	if r.Method != http.MethodHead {
		if _, err := io.Copy(w, attachment.Open(r.Context())); err != nil {
			attachments.LogStreamError(ctx, err)
		}
	}
}

func PreviewAttachment(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	att := lookupAttachment(ctx, r, true)
	if att == nil {
		apiError(ctx, w, http.StatusNotFound, response.ParamsError, "attachment_not_found")
		return
	}
	if att.HighRisk {
		auditRisk(ctx, r, att, "preview", false)
		InboundError(ctx, w, 403, response.NoAccessPrivileges, "risk_preview_forbidden")
		return
	}
	preview, ok := att.Preview(r.Context())
	if !ok {
		apiError(ctx, w, http.StatusUnsupportedMediaType, response.ParamsError, "attachment_preview_unsupported")
		return
	}
	success(w, preview)
}

func auditRisk(ctx *context.Context, r *http.Request, part *attachments.Part, operation string, confirmed bool) {
	s, err := inbound.GetSettings(r.Context())
	if err == nil && s.RiskAudit {
		log.WithContext(ctx).Warnf("audit_event=high_risk_attachment actor_id=%d email_id=%d attachment_index=%d operation=%q confirmed=%t", ctx.UserID, part.EmailID, part.Index, operation, confirmed)
	}
}
