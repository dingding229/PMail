// Package imageproxy fetches remote images in mail on the reader's behalf, so
// the sender sees the server's address instead of the reader's IP, browser and
// cookies.
package imageproxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/utils/safenet"
)

const (
	MaxBytes     = 8 << 20
	maxRedirects = 3
	// A generic browser string: the sender learns nothing about the reader's
	// browser, and sites that refuse unknown agents still serve the image.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"
)

var (
	ErrURL      = errors.New("unsupported image URL")
	ErrUpstream = errors.New("image fetch failed")
	ErrType     = errors.New("not an image")
	ErrTooLarge = errors.New("image too large")
)

var allowedTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"image/avif": true, "image/bmp": true, "image/x-icon": true, "image/vnd.microsoft.icon": true,
	"image/svg+xml": true,
}

type Fetcher struct {
	Client *http.Client
}

var sem = make(chan struct{}, 16)

func New() *Fetcher {
	d := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext:            safenet.PublicDial(net.DefaultResolver, d.DialContext),
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  8 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		MaxIdleConns:           32,
		IdleConnTimeout:        30 * time.Second,
	}
	return &Fetcher{Client: &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return ErrUpstream
			}
			if _, err := Normalize(req.URL.String()); err != nil {
				return err
			}
			req.Header.Del("Referer")
			return nil
		},
		Jar: nil,
	}}
}

// Normalize accepts absolute http(s) URLs and protocol-relative ones (which a
// browser would load over https).
func Normalize(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	if len(raw) > 4096 {
		return nil, ErrURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return nil, ErrURL
	}
	u.Fragment = ""
	return u, nil
}

type Image struct {
	ContentType string
	Body        []byte
}

func (f *Fetcher) Fetch(ctx context.Context, raw string) (*Image, error) {
	u, err := Normalize(raw)
	if err != nil {
		return nil, err
	}
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrURL
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/svg+xml,image/*;q=0.8")
	res, err := f.Client.Do(req)
	if err != nil {
		return nil, errors.Join(ErrUpstream, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ErrUpstream
	}
	if res.ContentLength > MaxBytes {
		return nil, ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxBytes+1))
	if err != nil {
		return nil, errors.Join(ErrUpstream, err)
	}
	if len(body) > MaxBytes {
		return nil, ErrTooLarge
	}
	ct := imageType(res.Header.Get("Content-Type"), body)
	if ct == "" {
		return nil, ErrType
	}
	return &Image{ContentType: ct, Body: body}, nil
}

// imageType trusts a declared image type only when the bytes agree; servers
// that send application/octet-stream are sniffed instead.
func imageType(declared string, body []byte) string {
	media, _, _ := mime.ParseMediaType(declared)
	media = strings.ToLower(media)
	sniffed := http.DetectContentType(body)
	if media == "image/svg+xml" && looksLikeSVG(body) {
		return media
	}
	if allowedTypes[sniffed] && sniffed != "image/svg+xml" {
		return sniffed
	}
	if (media == "image/avif" || media == "image/x-icon" || media == "image/vnd.microsoft.icon") && allowedTypes[media] && !strings.HasPrefix(sniffed, "text/") {
		return media
	}
	if looksLikeSVG(body) && (media == "" || media == "application/octet-stream" || strings.HasPrefix(media, "text/")) {
		return "image/svg+xml"
	}
	return ""
}

func looksLikeSVG(body []byte) bool {
	head := bytes.ToLower(body[:min(len(body), 1024)])
	return bytes.Contains(head, []byte("<svg"))
}
