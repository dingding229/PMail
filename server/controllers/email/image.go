package email

import (
	"errors"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jinnrry/pmail/services/detail"
	"github.com/Jinnrry/pmail/services/imageproxy"
	"github.com/Jinnrry/pmail/utils/context"
	log "github.com/sirupsen/logrus"
)

var imageFetcher = imageproxy.New()

// RemoteImage loads an external image of a mail through the server, so the
// sender cannot see the reader's IP address, browser or cookies. Only URLs
// that occur in a mail the user can read are fetched; the endpoint is not a
// general-purpose proxy.
func RemoteImage(ctx *context.Context, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fail := func(status int) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(status)
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	target := r.URL.Query().Get("u")
	if err != nil || id <= 0 {
		fail(http.StatusBadRequest)
		return
	}
	if !imageproxy.ValidToken(ctx.UserID, id, r.URL.Query().Get("t")) {
		fail(http.StatusForbidden)
		return
	}
	if _, err := imageproxy.Normalize(target); err != nil {
		fail(http.StatusBadRequest)
		return
	}
	mail, err := detail.WebDetail(ctx, id, false)
	if err != nil {
		fail(http.StatusNotFound)
		return
	}
	if !URLInHTML(mail.Html.String, target) {
		fail(http.StatusForbidden)
		return
	}
	img, err := imageFetcher.Fetch(r.Context(), target)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, imageproxy.ErrType) || errors.Is(err, imageproxy.ErrTooLarge) {
			status = http.StatusUnsupportedMediaType
		}
		log.WithContext(ctx).Debugf("Remote image for mail %d: %v", id, err)
		fail(status)
		return
	}
	w.Header().Set("Content-Type", img.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(img.Body)))
	// The browser keeps the image, so opening the mail again does not tell
	// the sender a second time.
	w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	if r.Method != http.MethodHead {
		// #nosec G705 -- verified image bytes, served with nosniff and a sandbox CSP
		_, _ = w.Write(img.Body)
	}
}

// URLInHTML reports whether target is referenced by the mail body, either as
// written or with HTML character references decoded.
func URLInHTML(body, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" || body == "" {
		return false
	}
	if strings.Contains(body, target) || strings.Contains(html.UnescapeString(body), target) {
		return true
	}
	// Protocol-relative URLs are requested with https: added by the client.
	if rest, ok := strings.CutPrefix(target, "https:"); ok && strings.HasPrefix(rest, "//") {
		return URLInHTML(body, rest)
	}
	return false
}
