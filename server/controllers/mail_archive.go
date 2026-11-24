package controllers

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/listen/imap_server"
	"github.com/Jinnrry/pmail/services/mailarchive"
	"github.com/Jinnrry/pmail/services/maintenance"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

func mailArchiveError(ctx *context.Context, w http.ResponseWriter, err error) {
	status, number := http.StatusBadRequest, response.ParamsError
	switch {
	case errors.Is(err, mailarchive.ErrBusy) || errors.Is(err, mailarchive.ErrQuota):
		status = http.StatusConflict
	case errors.Is(err, mailarchive.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, mailarchive.ErrTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, maintenance.ErrBusy):
		status = http.StatusServiceUnavailable
	case mailarchive.ErrorCode(err) == "mail_archive_failed":
		status, number = http.StatusInternalServerError, response.ServerError
	}
	InboundError(ctx, w, status, number, mailarchive.ErrorCode(err))
}
func archiveView(ctx *context.Context, j mailarchive.Job) map[string]any {
	v := map[string]any{"id": j.ID, "state": j.State, "expires_at": j.Expires}
	if j.Kind == "export" {
		v["exported_count"] = j.Succeeded
		v["total_count"] = max(j.Expected, j.Succeeded)
		if j.Size > 0 {
			v["size"] = j.Size
		}
	} else {
		v["total"] = j.Total
		v["imported"] = j.Succeeded
		v["skipped"] = j.Skipped
		v["failed"] = j.Failed
	}
	if j.Error != "" {
		v["error_code"] = j.Error
		v["error"] = i18n.T(ctx, j.Error)
	}
	return v
}
func archiveBusy(ctx *context.Context, w http.ResponseWriter, err error, kind string) {
	if errors.Is(err, mailarchive.ErrBusy) {
		InboundError(ctx, w, http.StatusConflict, response.ParamsError, kind+"_busy")
		return
	}
	mailArchiveError(ctx, w, err)
}
func MailExport(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	var body struct {
		Folders []string `json:"folders"`
		Format  string   `json:"format"`
		Trash   bool     `json:"include_trash"`
	}
	if !inboundBody(ctx, w, r, &body) {
		return
	}
	if body.Format != "mbox" {
		mailArchiveError(ctx, w, mailarchive.ErrInvalid)
		return
	}
	folders, err := mailarchive.ExportFolders(r.Context(), ctx.UserID, body.Folders, body.Trash)
	if err != nil {
		mailArchiveError(ctx, w, err)
		return
	}
	j, err := mailarchive.Jobs.StartExport(config.ROOT_PATH, ctx.UserID, folders)
	if err != nil {
		archiveBusy(ctx, w, err, "export")
		return
	}
	response.NewSuccessResponse(archiveView(ctx, j)).FPrintStatus(w, http.StatusAccepted)
}
func MailArchiveStatus(kind string) HandlerFunc {
	return func(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
		if !inboundAccess(ctx, w, false) {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		j, err := mailarchive.Jobs.Status(config.ROOT_PATH, ctx.UserID, r.PathValue("id"), kind)
		if err != nil {
			mailArchiveError(ctx, w, err)
			return
		}
		inboundOK(w, archiveView(ctx, j))
	}
}
func MailExportDownload(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	f, err := mailarchive.Jobs.OpenExport(config.ROOT_PATH, ctx.UserID, r.PathValue("id"))
	if err != nil {
		mailArchiveError(ctx, w, err)
		return
	}
	defer f.Close()
	i, err := f.Stat()
	if err != nil {
		mailArchiveError(ctx, w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="pmail-mailbox.zip"`)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(6 * time.Hour))
	http.ServeContent(w, r, "pmail-mailbox.zip", i.ModTime(), f)
}
func MailImport(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, false) {
		return
	}
	limit, err := mailarchive.UploadLimit(r.Context())
	if err != nil {
		mailArchiveError(ctx, w, err)
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(30 * time.Minute))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, limit+(64<<10))
	p, err := r.MultipartReader()
	if err != nil {
		mailArchiveError(ctx, w, mailarchive.ErrInvalid)
		return
	}
	u, err := mailarchive.Jobs.PrepareImport(config.ROOT_PATH, ctx.UserID)
	if err != nil {
		archiveBusy(ctx, w, err, "import")
		return
	}
	defer u.Abort()
	var target string
	fileSeen, targetSeen := false, false
	for {
		part, e := p.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			err = mailarchive.ErrInvalid
			var x *http.MaxBytesError
			if errors.As(e, &x) {
				err = mailarchive.ErrTooLarge
			}
			break
		}
		switch {
		case part.FormName() == "file" && !fileSeen && part.FileName() != "":
			fileSeen = true
			err = u.Copy(part.FileName(), part, limit)
		case part.FormName() == "target" && !targetSeen && part.FileName() == "":
			targetSeen = true
			b, e := io.ReadAll(io.LimitReader(part, 257))
			if e != nil || len(b) > 256 {
				err = mailarchive.ErrTarget
			} else {
				target = string(b)
			}
		default:
			err = mailarchive.ErrInvalid
		}
		if err != nil {
			var x *http.MaxBytesError
			if errors.As(err, &x) {
				err = mailarchive.ErrTooLarge
			}
			break
		}
	}
	if err == nil && !fileSeen {
		err = mailarchive.ErrInvalid
	}
	if err != nil {
		mailArchiveError(ctx, w, err)
		return
	}
	f, err := mailarchive.ImportFolder(r.Context(), ctx.UserID, target)
	if err != nil {
		mailArchiveError(ctx, w, err)
		return
	}
	j, err := u.Submit(f, limit, imap_server.MailboxChanged)
	if err != nil {
		mailArchiveError(ctx, w, err)
		return
	}
	response.NewSuccessResponse(archiveView(ctx, j)).FPrintStatus(w, http.StatusAccepted)
}
func MailArchiveSettings(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			Max int64 `json:"max_upload_bytes"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		if err := mailarchive.SetUploadLimit(r.Context(), body.Max); err != nil {
			mailArchiveError(ctx, w, err)
			return
		}
		log.Infof("audit_event=mail_import_limit_changed actor_id=%d", ctx.UserID)
	}
	n, err := mailarchive.UploadLimit(r.Context())
	if err != nil {
		mailArchiveError(ctx, w, err)
		return
	}
	inboundOK(w, map[string]int64{"max_upload_bytes": n})
}
