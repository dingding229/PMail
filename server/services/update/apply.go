package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/internal/releasebundle"
	"github.com/Jinnrry/pmail/signal"
	log "github.com/sirupsen/logrus"
)

var ErrStale = errors.New("update_target_stale")
var ErrUnavailable = errors.New("updater_unavailable")
var errDigest = errors.New("release archive checksum mismatch")

const releaseDownloadBase = "https://github.com/dingding229/PMail/releases/download/"

type installer struct {
	store     store
	key       ed25519.PublicKey
	arch      string
	supported bool
	restart   func()
}

func runtimeInstaller() installer {
	_, docker := os.Stat("/.dockerenv")
	return installer{
		store: storeAt(config.ROOT_PATH), key: releasebundle.PublicKey(), arch: runtime.GOARCH,
		supported: runtime.GOOS == "linux" && filepath.Clean(config.ROOT_PATH) == "/work" && docker == nil,
		restart:   func() { signal.UpdateExitChan <- struct{}{} },
	}
}

func (in installer) enabled(version string) bool {
	return !config.Flag("PMAIL_DISABLE_STAGED_UPDATE") && in.supported && releasebundle.Asset(in.arch) != "" && len(in.key) == ed25519.PublicKeySize && rank(canonicalVersion(version)) >= 0
}

func (c *Checker) updater(rows []Release) UpdaterStatus {
	status := UpdaterStatus{}
	switch {
	case config.Flag("PMAIL_DISABLE_STAGED_UPDATE") || os.Getenv("PMAIL_UPDATE_CHECK") == "0":
		status.Reason = "update_install_disabled"
	case len(c.installer.key) != ed25519.PublicKeySize:
		status.Reason = "update_key_unconfigured"
	case !c.installer.enabled(c.version):
		status.Reason = "update_install_unsupported"
	case len(rows) > 0 && rank(rows[0].Version) > rank(canonicalVersion(c.version)) && c.bundleURLs(rows[0]) == nil:
		status.Reason = "update_assets_unavailable"
	default:
		kind := "self-update"
		status.Enabled, status.Kind = true, &kind
	}
	return status
}

func (c *Checker) bundleURLs(release Release) map[string]string {
	result := make(map[string]string)
	endpoint, err := url.Parse(c.endpoint)
	if err != nil || endpoint.Scheme != "https" {
		return nil
	}
	for _, name := range []string{releasebundle.Checksums, releasebundle.Signature, releasebundle.Asset(c.installer.arch)} {
		raw := release.assets[name]
		if raw == "" || name == "" {
			return nil
		}
		if c.endpoint == defaultReleasesURL {
			expected := releaseDownloadBase + release.Version + "/" + name
			if raw != expected {
				return nil
			}
		} else {
			u, err := url.Parse(raw)
			if err != nil || !sameDownloadOrigin(u, endpoint) {
				return nil
			}
		}
		result[name] = raw
	}
	return result
}

func sameDownloadOrigin(u, origin *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Opaque == "" && u.Fragment == "" && strings.EqualFold(u.Host, origin.Host)
}

func (c *Checker) downloadClient() *http.Client {
	return &http.Client{Transport: c.client.Transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return http.ErrUseLastResponse
		}
		if c.endpoint != defaultReleasesURL {
			if sameDownloadOrigin(req.URL, via[0].URL) {
				return nil
			}
		} else if req.URL.Scheme == "https" && req.URL.User == nil && req.URL.Opaque == "" && req.URL.Fragment == "" {
			switch req.URL.Host {
			case "release-assets.githubusercontent.com", "objects.githubusercontent.com":
				return nil
			}
		}
		return http.ErrUseLastResponse
	}}
}

func (c *Checker) Apply(ctx context.Context, tag string) (JobRef, error) {
	if err := ctx.Err(); err != nil {
		return JobRef{}, err
	}
	c.mu.Lock()
	if os.Getenv("PMAIL_UPDATE_CHECK") == "0" || !c.installer.enabled(c.version) {
		c.mu.Unlock()
		return JobRef{}, ErrDisabled
	}
	if c.problem != nil || len(c.releases) == 0 || c.releases[0].Version != tag || rank(tag) <= rank(canonicalVersion(c.version)) {
		c.mu.Unlock()
		return JobRef{}, ErrStale
	}
	urls := c.bundleURLs(c.releases[0])
	c.mu.Unlock()
	if urls == nil {
		return JobRef{}, ErrUnavailable
	}

	jobMu.Lock()
	defer jobMu.Unlock()
	s := c.installer.store
	if err := s.init(); err != nil {
		return JobRef{}, err
	}
	last, err := s.readJob()
	if err != nil {
		return JobRef{}, err
	}
	if last.active() {
		return JobRef{}, ErrBusy
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return JobRef{}, err
	}
	job := Job{JobRef: JobRef{hex.EncodeToString(id[:])}, Version: tag, State: jobPending}
	if err := s.saveJob(job); err != nil {
		return JobRef{}, err
	}
	go c.install(context.WithoutCancel(ctx), job, urls)
	return job.JobRef, nil
}

func (c *Checker) install(parent context.Context, job Job, urls map[string]string) {
	// The administrator's request can disconnect; the job owns its deadline.
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	if err := c.stage(ctx, job, urls); err != nil {
		// Raw download errors can include expiring redirect query credentials.
		message := "update_stage_failed"
		switch {
		case errors.Is(err, ErrUnavailable), ctx.Err() != nil:
			message = "update_download_failed"
		case errors.Is(err, releasebundle.ErrSignature):
			message = "update_signature_invalid"
		case errors.Is(err, errDigest):
			message = "update_checksum_invalid"
		case errors.Is(err, errArchive):
			message = "update_bundle_invalid"
		}
		log.Warnf("self-update of %s failed: %s", job.Version, message)
		if err := c.installer.store.phase(job, jobFailed, message); err != nil {
			log.Error("could not record failed self-update job")
		}
		return
	}
	log.Infof("verified PMail %s staged; exiting for container restart", job.Version)
	c.installer.restart()
}

func (s store) phase(job Job, state, message string) error {
	jobMu.Lock()
	defer jobMu.Unlock()
	return s.saveJob(changedJob(job, state, message))
}

func changedJob(job Job, state, message string) Job {
	job.State, job.Message = state, nil
	if message != "" {
		job.Message = &message
	}
	return job
}

func (c *Checker) stage(ctx context.Context, job Job, urls map[string]string) error {
	s := c.installer.store
	if err := s.phase(job, jobDownloading, ""); err != nil {
		return err
	}
	client := c.downloadClient()
	manifest, err := downloadSmall(ctx, client, urls[releasebundle.Checksums], releasebundle.MaxManifest)
	if err != nil {
		return err
	}
	signature, err := downloadSmall(ctx, client, urls[releasebundle.Signature], 256)
	if err != nil {
		return err
	}
	expected, err := releasebundle.Verify(c.installer.key, job.Version, c.installer.arch, manifest, signature)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(s.staged(), ".incoming-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	root, err := os.OpenRoot(tmp)
	if err != nil {
		return err
	}
	defer root.Close()
	archive, err := root.OpenFile(archiveName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer archive.Close()
	if err := withDownload(ctx, client, urls[releasebundle.Asset(c.installer.arch)], func(data io.Reader) error {
		n, err := io.Copy(archive, io.LimitReader(data, releasebundle.MaxDownload+1))
		if err != nil || n == 0 || n > releasebundle.MaxDownload {
			return ErrUnavailable
		}
		return archive.Sync()
	}); err != nil {
		return err
	}
	if err := s.phase(job, jobVerifying, ""); err != nil {
		return err
	}
	if err := verifyArchive(archive, expected); err != nil {
		return err
	}
	if err := s.phase(job, jobStaging, ""); err != nil {
		return err
	}
	if err := unpackBundle(ctx, root, archive, maxExtracted); err != nil {
		return errors.Join(errArchive, err)
	}
	for name, data := range map[string][]byte{releasebundle.Checksums: manifest, releasebundle.Signature: signature} {
		if err := atomicState(filepath.Join(tmp, name), data); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	jobMu.Lock()
	defer jobMu.Unlock()
	if err := s.verifyDirs(); err != nil {
		return err
	}
	previous, err := s.current()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if previous == job.Version {
		return ErrBusy
	}
	if err := os.RemoveAll(s.version(job.Version)); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.version(job.Version)); err != nil {
		return err
	}
	if err := syncDirectory(s.staged()); err != nil {
		return err
	}
	if err := s.saveJob(changedJob(job, jobRestarting, "")); err != nil {
		return err
	}
	if err := atomicState(s.pointer(), []byte(job.Version+"\n")); err != nil {
		// If rename committed but syncing its directory failed, restore the
		// previous pointer instead of silently activating a failed job later.
		if got, _ := s.current(); got == job.Version {
			if previous != "" {
				return errors.Join(err, atomicState(s.pointer(), []byte(previous+"\n")))
			}
			return errors.Join(err, s.clearPointer())
		}
		return err
	}
	return nil
}

func verifyArchive(archive *os.File, expected [sha256.Size]byte) error {
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(archive, releasebundle.MaxDownload+1))
	if err != nil || n == 0 || n > releasebundle.MaxDownload || [sha256.Size]byte(hash.Sum(nil)) != expected {
		return errDigest
	}
	_, err = archive.Seek(0, io.SeekStart)
	return err
}

func withDownload(ctx context.Context, client *http.Client, raw string, use func(io.Reader) error) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("User-Agent", "PMail-Pro/update")
	res, err := client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	return use(res.Body)
}

func downloadSmall(ctx context.Context, client *http.Client, raw string, limit int64) ([]byte, error) {
	var data []byte
	err := withDownload(ctx, client, raw, func(r io.Reader) error {
		var err error
		data, err = io.ReadAll(io.LimitReader(r, limit+1))
		if err != nil || len(data) == 0 || int64(len(data)) > limit {
			return ErrUnavailable
		}
		return nil
	})
	return data, err
}

func (c *Checker) Status(id string) (Job, error) {
	if !jobID.MatchString(id) {
		return Job{}, ErrJob
	}
	job, err := c.installer.store.readJob()
	if err != nil {
		return Job{}, err
	}
	if job.JobID != id {
		return Job{}, ErrJob
	}
	return job, nil
}
