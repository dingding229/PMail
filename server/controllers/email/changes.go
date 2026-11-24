package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	"github.com/Jinnrry/pmail/services/mailevents"
	pctx "github.com/Jinnrry/pmail/utils/context"
)

const changesPath = "/api/email/changes"

func IsChangesPath(path string) bool { return path == changesPath }

type changesCursor struct {
	epoch   string
	version uint64
	maxID   int64
}

func parseChangesCursor(raw string) (changesCursor, bool) {
	if raw == "" {
		return changesCursor{}, true
	}
	if len(raw) > 106 {
		return changesCursor{}, false
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || len(parts[0]) == 0 || len(parts[0]) > 64 {
		return changesCursor{}, false
	}
	for _, r := range parts[0] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return changesCursor{}, false
		}
	}
	v, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || strconv.FormatUint(v, 10) != parts[1] {
		return changesCursor{}, false
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || id < 0 || strconv.FormatInt(id, 10) != parts[2] {
		return changesCursor{}, false
	}
	return changesCursor{epoch: parts[0], version: v, maxID: id}, true
}
func (c changesCursor) String() string { return fmt.Sprintf("%s.%d.%d", c.epoch, c.version, c.maxID) }

type changesResponse struct {
	Cursor   string            `json:"cursor"`
	Changed  bool              `json:"changed"`
	NewCount int64             `json:"new_count"`
	NewItems []mailevents.Item `json:"new_items"`
}

func changesError(ctx *pctx.Context, w http.ResponseWriter, status, code int, key string) {
	response.NewErrorResponse(code, i18n.T(ctx, key), map[string]string{"code": key}).FPrintStatus(w, status)
}

func Changes(ctx *pctx.Context, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if ctx.UserID <= 0 {
		changesError(ctx, w, http.StatusUnauthorized, response.NeedLogin, "inbound_login_required")
		return
	}
	q := r.URL.Query()
	cursor, ok := parseChangesCursor(q.Get("cursor"))
	if !ok {
		changesError(ctx, w, http.StatusBadRequest, response.ParamsError, "live_updates_invalid_cursor")
		return
	}
	seconds := 25
	if q.Has("wait") {
		var err error
		seconds, err = strconv.Atoi(q.Get("wait"))
		if err != nil || seconds < 0 || seconds > 25 {
			changesError(ctx, w, http.StatusBadRequest, response.ParamsError, "live_updates_invalid_wait")
			return
		}
	}
	// Keep the server's 90s default for every other handler. Real HTTP writers
	// implement this through the session wrapper; test writers may not.
	writeDeadline := time.Now().Add(time.Duration(seconds+10) * time.Second)
	_ = http.NewResponseController(w).SetWriteDeadline(writeDeadline)
	base := r.Context()
	if base.Err() != nil {
		return
	}
	b := mailevents.Default
	first := cursor.epoch != b.Epoch()
	version := b.Version(ctx.UserID)
	changed := first || version > cursor.version
	itemResponse := changesResponse{NewItems: []mailevents.Item{}}
	if !changed && seconds > 0 {
		wait, cancel := context.WithTimeout(base, time.Duration(seconds)*time.Second)
		result, err := b.Wait(wait, ctx.UserID, cursor.version)
		cancel()
		if base.Err() != nil {
			return
		}
		if errors.Is(err, mailevents.ErrBusy) {
			w.Header().Set("Retry-After", "1")
			changesError(ctx, w, http.StatusServiceUnavailable, response.ServerError, "live_updates_busy")
			return
		}
		if result.Evicted {
			// Don't advance this tab's cursor or consume its notifications.
			itemResponse.Cursor = cursor.String()
			_ = json.NewEncoder(w).Encode(itemResponse)
			return
		}
		version, changed = result.Version, result.Changed
	}
	// Sample the version before querying the DB. A concurrent commit that is
	// not visible in this snapshot will still raise the next long poll's version.
	queryDeadline := time.Now().Add(9 * time.Second)
	if latestSafe := writeDeadline.Add(-time.Second); latestSafe.Before(queryDeadline) {
		queryDeadline = latestSafe
	}
	query, cancel := context.WithDeadline(base, queryDeadline)
	defer cancel()
	if !first && seconds > 0 {
		// A request can outlive a password/2FA reset or an administrator
		// disabling the account. Don't return new subjects to a stale identity.
		ok, err := mailevents.CredentialsCurrent(query, ctx.UserID, ctx.AuthVersion, ctx.FactorVersion)
		if base.Err() != nil {
			return
		}
		if err != nil {
			changesError(ctx, w, http.StatusInternalServerError, response.ServerError, "internal_error")
			return
		}
		if !ok {
			changesError(ctx, w, http.StatusUnauthorized, response.NeedLogin, "inbound_login_required")
			return
		}
	}
	latest, err := mailevents.VisibleMax(query, ctx.UserID)
	if err == nil && !first {
		itemResponse.NewCount, itemResponse.NewItems, err = mailevents.NewMail(query, ctx.UserID, cursor.maxID, latest)
	}
	if base.Err() != nil {
		return
	}
	if err != nil {
		changesError(ctx, w, http.StatusInternalServerError, response.ServerError, "internal_error")
		return
	}
	itemResponse.Cursor = changesCursor{b.Epoch(), version, latest}.String()
	itemResponse.Changed = changed || latest != cursor.maxID || itemResponse.NewCount != 0
	_ = json.NewEncoder(w).Encode(itemResponse)
}
