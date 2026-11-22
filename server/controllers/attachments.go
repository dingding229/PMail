package controllers

import (
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jinnrry/pmail/dto/parsemail"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/services/attachments"
	"github.com/Jinnrry/pmail/utils/context"
)

func GetAttachments(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	serveAttachment(ctx, w, r, lookupAttachment(ctx, r, false), false)
}

func Download(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	serveAttachment(ctx, w, r, lookupAttachment(ctx, r, true), true)
}

func lookupAttachment(ctx *context.Context, r *http.Request, download bool) *parsemail.Attachment {
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
		return attachments.GetAttachments(ctx, emailID, r.PathValue("cid"))
	}
	return nil
}

func attachmentByIndex(ctx *context.Context, mail, index string) *parsemail.Attachment {
	emailID, mailErr := strconv.Atoi(mail)
	attachmentIndex, indexErr := strconv.Atoi(index)
	if mailErr != nil || indexErr != nil || emailID <= 0 || attachmentIndex < 0 {
		return nil
	}
	return attachments.GetAttachmentsByIndex(ctx, emailID, attachmentIndex)
}

func serveAttachment(ctx *context.Context, w http.ResponseWriter, r *http.Request, attachment *parsemail.Attachment, download bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !download {
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	if attachment == nil || len(attachment.Content) == 0 {
		response.Localized(ctx, response.ParamsError, "attachment_not_found").FPrint(w)
		return
	}
	contentType, inline := attachments.InlineContentType(attachment.ContentType)
	if download {
		contentType, inline = "application/octet-stream", false
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(attachment.Content)))
	if !inline {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Filename}))
	}
	if r.Method != http.MethodHead {
		_, _ = w.Write(attachment.Content)
	}
}
