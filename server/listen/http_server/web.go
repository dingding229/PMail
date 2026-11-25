package http_server

import (
	"compress/gzip"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/dto/response"
	"github.com/Jinnrry/pmail/i18n"
	log "github.com/sirupsen/logrus"
)

const (
	acmePrefix = "/.well-known/acme-challenge/"
	// img-src keeps the app itself from loading third-party images: mail images
	// only ever arrive through the same-origin image proxy, never from the sender.
	webCSP            = "base-uri 'self'; object-src 'none'; frame-ancestors 'self'; img-src 'self' data: blob:"
	telegramWebAppCSP = "base-uri 'self'; object-src 'none'; frame-ancestors 'self' https://web.telegram.org; img-src 'self' data: blob:"
)

// Vite uses an eight-character URL-safe fingerprint at the end of bundle names.
var fingerprintedAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{8}\.[^./]+$`)

func staticWeb() http.HandlerFunc {
	dir := config.WebRoot()
	if root, err := os.OpenRoot(dir); err != nil {
		log.Warnf("Web root %q is unavailable; API remains enabled: %v", dir, err)
	} else {
		root.Close()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if inNamespace(r.URL.Path, "/api") {
			apiNotFound(w, r)
			return
		}
		if inNamespace(r.URL.Path, "/attachments") || inNamespace(r.URL.Path, strings.TrimSuffix(acmePrefix, "/")) {
			http.Error(w, i18n.T(i18n.ForRequest(r), "not_found"), http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, i18n.T(i18n.ForRequest(r), "method_not_allowed"), http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if r.URL.Path == "/" {
			name = "index.html"
		}
		// Reject ambiguous paths before opening anything. Root also confines
		// symlinks and stays safe if files are changed during a request.
		if !fs.ValidPath(name) || name == "." || strings.Contains(name, `\`) {
			http.Error(w, i18n.T(i18n.ForRequest(r), "not_found"), http.StatusNotFound)
			return
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			webUnavailable(w, r)
			return
		}
		defer root.Close()

		file, info, err := openWebFile(root, name)
		if errors.Is(err, fs.ErrNotExist) && name != "index.html" {
			if path.Ext(name) != "" {
				w.Header().Add("Vary", "Accept")
			}
			if path.Ext(name) == "" || strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html") {
				name = "index.html"
				file, info, err = openWebFile(root, name)
			}
		}
		if err != nil {
			if name == "index.html" {
				webUnavailable(w, r)
			} else {
				http.Error(w, i18n.T(i18n.ForRequest(r), "not_found"), http.StatusNotFound)
			}
			return
		}
		defer file.Close()
		switch {
		case name == "index.html":
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasPrefix(name, "assets/") && fingerprintedAsset.MatchString(path.Base(name)):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		contentType := mime.TypeByExtension(path.Ext(name))
		mediaType, _, _ := mime.ParseMediaType(contentType)
		if mediaType == "text/html" && name == "index.html" && strings.HasPrefix(r.URL.Path, "/tg/") {
			// Telegram Web shows Mini Apps in an iframe; the page holds no
			// session state of its own (see controllers/telegram_webapp.go).
			w.Header().Del("X-Frame-Options")
			w.Header().Set("Content-Security-Policy", telegramWebAppCSP)
		}
		if mediaType == "text/html" && w.Header().Get("Content-Security-Policy") == "" {
			w.Header().Set("Content-Security-Policy", webCSP)
		}
		if compressible(mediaType) {
			w.Header().Set("Content-Type", contentType)
			w.Header().Add("Vary", "Accept-Encoding")
			if r.Header.Get("Range") == "" && acceptsGzip(r.Header.Values("Accept-Encoding")) {
				gz := &gzipStaticWriter{ResponseWriter: w, head: r.Method == http.MethodHead}
				http.ServeContent(gz, r, name, info.ModTime(), file)
				if gz.body != nil {
					_ = gz.body.Close()
				}
				return
			}
		}
		http.ServeContent(w, r, name, info.ModTime(), file)
	}
}

func openWebFile(root *os.Root, name string) (*os.File, fs.FileInfo, error) {
	info, err := root.Stat(name)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fs.ErrNotExist
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		if err == nil {
			err = fs.ErrNotExist
		}
		return nil, nil, err
	}
	return file, info, nil
}

func apiNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Accept-Language, X-PMail-Lang")
	response.Localized(i18n.ForRequest(r), response.RouteNotFound, "not_found").FPrintStatus(w, http.StatusNotFound)
}

func webUnavailable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, i18n.T(i18n.ForRequest(r), "web_unavailable"), http.StatusServiceUnavailable)
}

func compressible(media string) bool {
	return strings.HasPrefix(media, "text/") || media == "application/javascript" ||
		media == "application/json" || media == "image/svg+xml" || media == "application/wasm"
}

func acceptsGzip(headers []string) bool {
	wildcard := false
	for item := range strings.SplitSeq(strings.Join(headers, ","), ",") {
		encoding, params, _ := strings.Cut(item, ";")
		encoding = strings.ToLower(strings.TrimSpace(encoding))
		if encoding != "gzip" && encoding != "*" {
			continue
		}
		q := 1.0
		for param := range strings.SplitSeq(params, ";") {
			key, value, _ := strings.Cut(param, "=")
			if strings.EqualFold(strings.TrimSpace(key), "q") {
				q, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
			}
		}
		allowed := q > 0 && q <= 1
		if encoding == "gzip" {
			return allowed
		}
		wildcard = allowed
	}
	return wildcard
}

// Only compress a full file response. ServeContent keeps control of conditional
// requests, range requests (identity) and errors.
type gzipStaticWriter struct {
	http.ResponseWriter
	body *gzip.Writer
	head bool
}

func (w *gzipStaticWriter) WriteHeader(status int) {
	if status == http.StatusOK {
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Accept-Ranges")
		if !w.head {
			w.body = gzip.NewWriter(w.ResponseWriter)
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipStaticWriter) Write(data []byte) (int, error) {
	if w.body != nil {
		return w.body.Write(data)
	}
	return w.ResponseWriter.Write(data)
}
