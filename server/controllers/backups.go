package controllers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/db"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/maintenance"
	"github.com/Jinnrry/pmail/services/sitebackup"
	"github.com/Jinnrry/pmail/utils/clientip"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

const BackupStatusPath = "/api/admin/backups/restore-status"

func BackupAPIPath(path string) bool {
	return path == "/api/admin/backups" || strings.HasPrefix(path, "/api/admin/backups/") || strings.HasPrefix(path, "/api/email/export") || strings.HasPrefix(path, "/api/email/import") || path == "/api/admin/mail-archive"
}
func backupStore(version string) sitebackup.Store {
	s := sitebackup.Store{Engine: db.Instance, Root: config.ROOT_PATH, Version: version}
	if config.Instance != nil {
		s.SecretKey = config.Instance.SecretKey
	}
	return s
}
func auditBackup(ctx *context.Context, action string) {
	log.Infof("audit_event=backup_%s actor_id=%d", action, ctx.UserID)
}
func backupError(ctx *context.Context, w http.ResponseWriter, err error) {
	status, number := http.StatusBadRequest, response.ParamsError
	switch {
	case errors.Is(err, sitebackup.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, sitebackup.ErrBusy):
		status = http.StatusConflict
	case errors.Is(err, sitebackup.ErrTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, maintenance.ErrBusy):
		status = http.StatusServiceUnavailable
	case sitebackup.ErrorCode(err) == "backup_failed":
		status, number = http.StatusInternalServerError, response.ServerError
	}
	InboundError(ctx, w, status, number, sitebackup.ErrorCode(err))
}
func BackupIndex(version string) HandlerFunc {
	return func(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
		if !inboundAccess(ctx, w, true) {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodGet {
			auditBackup(ctx, "list")
			dir, err := sitebackup.DataDir(config.ROOT_PATH, "backups")
			if err != nil {
				backupError(ctx, w, err)
				return
			}
			list, err := sitebackup.List(dir)
			if err != nil {
				backupError(ctx, w, err)
				return
			}
			inboundOK(w, map[string]any{"items": list, "active_job": sitebackup.Jobs.Current()})
			return
		}
		var body struct {
			Passphrase string `json:"passphrase"`
			TLS        *bool  `json:"include_tls"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		auditBackup(ctx, "export_requested")
		j, err := sitebackup.Jobs.StartExport(backupStore(version), body.Passphrase, body.TLS == nil || *body.TLS)
		if err != nil {
			backupError(ctx, w, err)
			return
		}
		response.NewSuccessResponse(map[string]string{"job_id": j.ID}).FPrintStatus(w, http.StatusAccepted)
	}
}
func BackupRestore(version string) HandlerFunc {
	return func(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
		if !inboundAccess(ctx, w, true) {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		var body struct {
			Passphrase string `json:"passphrase"`
			Confirm    string `json:"confirm"`
		}
		if !inboundBody(ctx, w, r, &body) {
			return
		}
		auditBackup(ctx, "restore_requested")
		if body.Confirm != "RESTORE" {
			backupError(ctx, w, sitebackup.ErrConfirm)
			return
		}
		ready := make(chan struct{})
		defer close(ready)
		j, err := sitebackup.Jobs.StartRestore(backupStore(version), r.PathValue("name"), body.Passphrase, ready)
		if err != nil {
			backupError(ctx, w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: restoreCookie, Value: j.ID, Path: BackupStatusPath, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: clientip.HTTPS(r) || strings.HasPrefix(r.Header.Get("Origin"), "https://"), MaxAge: 86400})
		response.NewSuccessResponse(map[string]any{"started": true, "job_id": j.ID}).FPrintStatus(w, http.StatusAccepted)
	}
}

const restoreCookie = "pmail_restore_status"

func RestoreCapability(r *http.Request) string {
	if id := r.Header.Get("X-PMail-Restore-ID"); id != "" {
		return id
	}
	if c, err := r.Cookie(restoreCookie); err == nil {
		return c.Value
	}
	return ""
}
func restoreView(ctx *context.Context, j *sitebackup.Job) map[string]any {
	if j == nil || j.Kind != "restore" {
		return map[string]any{"stage": "idle"}
	}
	view := map[string]any{"stage": j.Phase, "tables_done": j.Tables, "tables_total": j.TotalTables, "rows_done": j.Rows}
	if j.Error != "" {
		view["stage"] = "failed"
		view["error_code"] = j.Error
		view["error"] = i18n.T(ctx, j.Error)
	}
	return view
}

func BackupRestoreStatus(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	auditBackup(ctx, "restore_status")
	w.Header().Set("Cache-Control", "no-store")
	j := sitebackup.Jobs.Current()
	if j != nil && j.Kind != "restore" {
		j = nil
	}
	inboundOK(w, restoreView(ctx, j))
}

// During maintenance DB sessions cannot be queried. Only a random capability
// from the accepted restore response can access its (non-sensitive) progress.
func BackupRestoreProgress(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	j := sitebackup.Jobs.RestoreStatus(RestoreCapability(r))
	if j == nil {
		http.SetCookie(w, &http.Cookie{Name: restoreCookie, Path: BackupStatusPath, MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		backupError(ctx, w, sitebackup.ErrNotFound)
		return
	}
	j.ID = ""
	j.Name = ""
	log.Info("audit_event=backup_restore_status_capability")
	inboundOK(w, restoreView(ctx, j))
}
func BackupDownload(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	auditBackup(ctx, "download")
	dir, err := sitebackup.DataDir(config.ROOT_PATH, "backups")
	if err != nil {
		backupError(ctx, w, err)
		return
	}
	name := r.PathValue("name")
	f, err := sitebackup.OpenNamed(dir, name)
	if err != nil {
		backupError(ctx, w, err)
		return
	}
	defer f.Close()
	i, err := f.Stat()
	if err != nil {
		backupError(ctx, w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(6 * time.Hour))
	http.ServeContent(w, r, name, i.ModTime(), f)
}
func BackupDelete(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	auditBackup(ctx, "delete")
	dir, err := sitebackup.DataDir(config.ROOT_PATH, "backups")
	if err == nil {
		err = sitebackup.Jobs.Delete(dir, r.PathValue("name"))
	}
	if err != nil {
		backupError(ctx, w, err)
		return
	}
	inboundOK(w, nil)
}
func BackupUpload(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	if !inboundAccess(ctx, w, true) {
		return
	}
	auditBackup(ctx, "upload")
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(6 * time.Hour))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(6 * time.Hour))
	r.Body = http.MaxBytesReader(w, r.Body, sitebackup.UploadLimit+(1<<20))
	parts, err := r.MultipartReader()
	if err != nil {
		backupError(ctx, w, sitebackup.ErrInvalid)
		return
	}
	part, err := parts.NextPart()
	if err != nil || part.FormName() != "file" || part.FileName() == "" {
		backupError(ctx, w, sitebackup.ErrInvalid)
		return
	}
	dir, err := sitebackup.DataDir(config.ROOT_PATH, "backups")
	if err != nil {
		backupError(ctx, w, err)
		return
	}
	name, err := sitebackup.Jobs.Receive(dir, part.FileName(), part)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			err = sitebackup.ErrTooLarge
		}
		backupError(ctx, w, err)
		return
	}
	items, err := sitebackup.List(dir)
	if err != nil {
		backupError(ctx, w, err)
		return
	}
	for _, item := range items {
		if item.Name == name {
			response.NewSuccessResponse(item).FPrintStatus(w, http.StatusCreated)
			return
		}
	}
	backupError(ctx, w, sitebackup.ErrNotFound)
}
