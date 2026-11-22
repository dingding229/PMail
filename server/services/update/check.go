// Package update reads release metadata and talks only to the private updater.
package update

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const releasesURL = "https://api.github.com/repos/dingding229/PMail/releases"
const maxReleaseBytes = 2 << 20

var ErrRate = errors.New("update_check_rate_limited")
var ErrDisabled = errors.New("updater_disabled")
var ErrSidecar = errors.New("updater_unavailable")
var ErrJob = errors.New("update_job_not_found")
var ErrBusy = errors.New("updater_busy")
var semver = regexp.MustCompile(`^v0\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

//go:embed changelog.md
var fallback string

type Release struct {
	Version   string `json:"version"`
	Published string `json:"published_at"`
	Notes     string `json:"notes_markdown"`
	URL       string `json:"url"`
	Current   bool   `json:"is_current"`
}
type UpdaterStatus struct {
	Enabled bool    `json:"enabled"`
	Kind    *string `json:"kind"`
}
type Info struct {
	Current   string        `json:"current"`
	Latest    *string       `json:"latest"`
	HasUpdate bool          `json:"has_update"`
	CheckedAt *time.Time    `json:"checked_at"`
	Error     *string       `json:"check_error"`
	Releases  []Release     `json:"releases"`
	Updater   UpdaterStatus `json:"updater"`
	Manual    []string      `json:"manual_commands"`
}
type GitHubRelease struct {
	Tag       string `json:"tag_name"`
	Draft     bool   `json:"draft"`
	Pre       bool   `json:"prerelease"`
	Published string `json:"published_at"`
	Body      string `json:"body"`
}
type Checker struct {
	mu         sync.Mutex
	version    string
	client     *http.Client
	releases   []Release
	etag       string
	checked    *time.Time
	problem    *string
	lastManual time.Time
	inFlight   bool
}

var checkerMu sync.RWMutex
var global = New("dev", nil)

func Current() *Checker { checkerMu.RLock(); defer checkerMu.RUnlock(); return global }
func New(version string, tr http.RoundTripper) *Checker {
	return &Checker{version: version, client: &http.Client{Timeout: 10 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func canonicalVersion(v string) string {
	if semver.MatchString(v) {
		return v
	}
	v = "v" + v
	if semver.MatchString(v) {
		return v
	}
	return ""
}
func rank(v string) int64 {
	m := semver.FindStringSubmatch(v)
	if len(m) == 0 {
		return -1
	}
	a, _ := strconv.ParseInt(m[1], 10, 64)
	b, _ := strconv.ParseInt(m[2], 10, 64)
	return a*1_000_000_000 + b
}
func note(tag string) string {
	version := strings.TrimPrefix(tag, "v")
	found := false
	var lines []string
	for line := range strings.SplitSeq(fallback, "\n") {
		if strings.HasPrefix(line, "## ") {
			if found {
				break
			}
			found = strings.HasPrefix(line, "## ["+version+"]") || line == "## "+version || strings.HasPrefix(line, "## "+version+" ")
			continue
		}
		if found {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
func releaseLink(tag string) string {
	return "https://github.com/dingding229/PMail/releases/tag/" + tag
}
func (c *Checker) Info() Info {
	c.mu.Lock()
	defer c.mu.Unlock()
	if os.Getenv("PMAIL_UPDATE_CHECK") == "0" {
		disabled := "update_check_disabled"
		return Info{Current: c.version, Error: &disabled, Releases: fallbackList(c.version, nil), Updater: sidecarStatus(), Manual: manual()}
	}
	rows := fallbackList(c.version, c.releases)
	out := Info{Current: c.version, CheckedAt: c.checked, Error: c.problem, Releases: rows, Updater: sidecarStatus(), Manual: manual()}
	if len(c.releases) > 0 {
		out.Latest = &c.releases[0].Version
		cur := rank(canonicalVersion(c.version))
		out.HasUpdate = cur >= 0 && rank(*out.Latest) > cur
	}
	return out
}
func fallbackList(current string, releases []Release) []Release {
	rows := append([]Release{}, releases...)
	tag := canonicalVersion(current)
	if tag == "" {
		return rows
	}
	for i := range rows {
		if rows[i].Version == tag {
			rows[i].Current = true
			if strings.TrimSpace(rows[i].Notes) == "" {
				rows[i].Notes = note(tag)
			}
			return rows
		}
	}
	if text := note(tag); text != "" {
		rows = append(rows, Release{Version: tag, Notes: text, URL: releaseLink(tag), Current: true})
	}
	sortRows(rows)
	return rows[:min(20, len(rows))]
}
func sortRows(r []Release) {
	slices.SortFunc(r, func(a, b Release) int {
		switch {
		case rank(a.Version) > rank(b.Version):
			return -1
		case rank(a.Version) < rank(b.Version):
			return 1
		}
		return 0
	})
}
func (c *Checker) Refresh(parent context.Context, manual bool) error {
	if os.Getenv("PMAIL_UPDATE_CHECK") == "0" {
		return nil
	}
	c.mu.Lock()
	now := time.Now()
	if manual && (c.inFlight || now.Sub(c.lastManual) < time.Minute) {
		c.mu.Unlock()
		return ErrRate
	}
	if c.inFlight {
		c.mu.Unlock()
		return nil
	}
	if manual {
		c.lastManual = now
	}
	c.inFlight = true
	etag := c.etag
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	rows, tag, unchanged, problem := c.fetch(ctx, etag)
	end := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inFlight = false
	if parent.Err() != nil {
		return nil
	}
	c.checked = &end
	if problem != "" {
		c.problem = &problem
		return nil
	}
	c.problem = nil
	if !unchanged {
		c.etag = tag
		c.releases = rows
	}
	return nil
}
func (c *Checker) fetch(ctx context.Context, etag string) ([]Release, string, bool, string) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return nil, "", false, "update_check_unavailable"
	}
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("User-Agent", "PMail-Pro/update")
	if etag != "" {
		r.Header.Set("If-None-Match", etag)
	}
	res, err := c.client.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", false, "update_check_timeout"
		}
		return nil, "", false, "update_check_unavailable"
	}
	defer res.Body.Close()
	if res.StatusCode == 304 && etag != "" {
		return nil, etag, true, ""
	}
	if res.StatusCode != 200 {
		return nil, "", false, "update_check_unavailable"
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxReleaseBytes+1))
	if err != nil || len(data) > maxReleaseBytes {
		return nil, "", false, "update_check_invalid_response"
	}
	var input []GitHubRelease
	if json.Unmarshal(data, &input) != nil || input == nil {
		return nil, "", false, "update_check_invalid_response"
	}
	output := []Release{}
	seen := map[string]bool{}
	for _, g := range input {
		if g.Draft || g.Pre || rank(g.Tag) < 0 || seen[g.Tag] {
			continue
		}
		seen[g.Tag] = true
		published := ""
		if date, err := time.Parse(time.RFC3339, g.Published); err == nil {
			published = date.UTC().Format(time.RFC3339)
		}
		notes := g.Body
		if len(notes) > 128<<10 {
			notes = strings.ToValidUTF8(notes[:128<<10], "")
		}
		output = append(output, Release{Version: g.Tag, Published: published, Notes: notes, URL: releaseLink(g.Tag)})
	}
	sortRows(output)
	return output[:min(20, len(output))], res.Header.Get("ETag"), false, ""
}
func manual() []string { return []string{"docker compose pull", "docker compose up -d"} }
func Start(ctx context.Context, version string) <-chan struct{} {
	c := New(version, nil)
	checkerMu.Lock()
	global = c
	checkerMu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for ctx.Err() == nil {
			_ = c.Refresh(ctx, false)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return done
}
