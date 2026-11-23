package antivirus

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
)

var ErrDisabled = errors.New("clamav_disabled")
var ErrUpdateBusy = errors.New("clamav_update_busy")
var active atomic.Pointer[Manager]

func Active() *Manager { return active.Load() }

// Start binds ClamAV to one PMail listener generation. The returned channel
// closes only after every child has exited and been reaped.
func Start(ctx context.Context, dir string, enabled bool) <-chan struct{} {
	m := newManager(dir, enabled, command)
	active.Store(m)
	done := make(chan struct{})
	go func() { defer close(done); defer active.CompareAndSwap(m, nil); m.run(ctx) }()
	return done
}

type UpdateResult struct {
	Status string    `json:"status"`
	At     time.Time `json:"at"`
}

type Status struct {
	Enabled        bool          `json:"enabled"`
	EnvControlled  bool          `json:"env_controlled"`
	State          string        `json:"state"`
	Updating       bool          `json:"updating"`
	Database       *database     `json:"database"`
	LastUpdate     *UpdateResult `json:"last_update"`
	Scanned        uint64        `json:"scanned"`
	Blocked        uint64        `json:"blocked"`
	AvailableBytes uint64        `json:"available_memory_bytes,omitempty"`
	MemoryWarning  bool          `json:"memory_warning"`
}

type counts struct {
	hour             int64
	scanned, blocked uint64
}

type Manager struct {
	dir, socket  string
	cmd          commandFunc
	wake, update chan struct{}
	slots        chan struct{}
	mu           sync.Mutex
	cancel       context.CancelFunc
	state        Status
	queued       bool
	hourly       [24]counts
	warnings     map[string]time.Time
	// Fixed production defaults, shortened only by package-local tests.
	probeInterval, retryMin, retryMax, updateInterval time.Duration
	scanTimeout                                       time.Duration
}

func newManager(dir string, enabled bool, cmd commandFunc) *Manager {
	m := &Manager{dir: dir, socket: filepath.Join(dir, "clamd.sock"), cmd: cmd,
		wake: make(chan struct{}, 1), update: make(chan struct{}, 1), slots: make(chan struct{}, 4),
		warnings: make(map[string]time.Time), probeInterval: 2 * time.Second, retryMin: 5 * time.Second,
		retryMax: 5 * time.Minute, updateInterval: 6 * time.Hour, scanTimeout: ScanTimeout}
	m.state = Status{Enabled: enabled, State: "disabled", Database: readDatabase(dir, "daily"), LastUpdate: readUpdateResult(dir)}
	if enabled {
		m.state.State = "starting"
	}
	return m
}

func (m *Manager) Snapshot() Status {
	m.mu.Lock()
	s := m.state
	s.Updating = s.Updating || m.queued
	now := time.Now().Unix() / 3600
	for _, b := range m.hourly {
		if b.hour <= now && b.hour > now-24 {
			s.Scanned += b.scanned
			s.Blocked += b.blocked
		}
	}
	m.mu.Unlock()
	s.AvailableBytes = availableMemory()
	s.MemoryWarning = s.AvailableBytes > 0 && s.AvailableBytes < 2<<30
	return s
}

func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Enabled == enabled {
		return
	}
	m.state.Enabled = enabled
	if enabled {
		m.state.State = "starting"
	} else if m.cancel != nil {
		m.state.State = "stopping"
		m.cancel()
	} else {
		m.state.State = "disabled"
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) RequestUpdate() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.state.Enabled {
		return ErrDisabled
	}
	if m.queued || m.state.Updating {
		return ErrUpdateBusy
	}
	m.queued = true
	m.update <- struct{}{}
	return nil
}

func (m *Manager) setState(ctx context.Context, state string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() == nil && m.state.Enabled {
		m.state.State = state
	}
}

func (m *Manager) run(ctx context.Context) {
	for ctx.Err() == nil {
		m.mu.Lock()
		if !m.state.Enabled {
			m.mu.Unlock()
			select {
			case <-ctx.Done():
			case <-m.wake:
			}
			continue
		}
		child, cancel := context.WithCancel(ctx)
		m.cancel = cancel
		m.mu.Unlock()
		m.runEnabled(child)
		cancel()
		m.mu.Lock()
		m.cancel = nil
		m.queued, m.state.Updating = false, false
		select {
		case <-m.update:
		default:
		}
		m.state.State = "disabled"
		if m.state.Enabled && ctx.Err() == nil {
			m.state.State = "starting"
		}
		m.mu.Unlock()
	}
}

func delay(ctx context.Context, d time.Duration, wake <-chan struct{}) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	case <-wake:
		return true
	}
}

func (m *Manager) runEnabled(ctx context.Context) {
	backoff := m.retryMin
	for ctx.Err() == nil {
		if err := m.prepare(); err != nil {
			m.setState(ctx, "error")
			m.mu.Lock()
			if m.queued {
				m.queued = false
				m.state.LastUpdate = &UpdateResult{Status: "failed", At: time.Now().UTC()}
				select {
				case <-m.update:
				default:
				}
			}
			m.mu.Unlock()
			m.warn("configuration_error", err)
		} else if readDatabase(m.dir, "main") != nil && readDatabase(m.dir, "daily") != nil {
			break
		} else {
			m.setState(ctx, "downloading")
			if m.refresh(ctx, false) == nil && readDatabase(m.dir, "main") != nil && readDatabase(m.dir, "daily") != nil {
				break
			}
			m.setState(ctx, "error")
		}
		// Peek via the queued flag on the next refresh, not a second process.
		if !delay(ctx, backoff, m.update) {
			return
		}
		backoff = min(backoff*2, m.retryMax)
	}
	if ctx.Err() != nil {
		return
	}
	m.mu.Lock()
	m.state.Database = readDatabase(m.dir, "daily")
	m.mu.Unlock()
	updaterDone := make(chan struct{})
	go func() { defer close(updaterDone); m.updateLoop(ctx) }()
	m.runClamd(ctx)
	<-updaterDone
}

func (m *Manager) updateLoop(ctx context.Context) {
	timer := time.NewTimer(m.updateInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-m.update:
		}
		if ctx.Err() != nil {
			return
		}
		_ = m.refresh(ctx, true)
		timer.Reset(m.updateInterval)
	}
}

func (m *Manager) refresh(ctx context.Context, reload bool) error {
	m.mu.Lock()
	// Claim both timer and manual requests as one operation. Drain the token
	// under the same lock used by RequestUpdate to avoid leaving a duplicate.
	m.queued, m.state.Updating = false, true
	select {
	case <-m.update:
	default:
	}
	before := m.state.Database
	m.mu.Unlock()
	job, cancel := context.WithTimeout(ctx, 20*time.Minute)
	err := m.cmd(job, "/usr/bin/freshclam", "--config-file="+filepath.Join(m.dir, "freshclam.conf"), "--stdout").Run()
	cancel()
	after := readDatabase(m.dir, "daily")
	var last *UpdateResult
	if ctx.Err() == nil {
		result := "success"
		if err != nil {
			result = "failed"
		}
		last = &UpdateResult{Status: result, At: time.Now().UTC()}
		if writeErr := writeUpdateResult(m.dir, last); writeErr != nil {
			m.warn("update_metadata_not_saved", writeErr)
		}
	}
	m.mu.Lock()
	m.state.Database = after
	if last != nil {
		m.state.LastUpdate = last
	}
	m.mu.Unlock()
	if ctx.Err() == nil && err != nil {
		m.warn("update_failed", err)
	}
	if ctx.Err() == nil && err == nil && reload && (before == nil || after == nil || *before != *after) {
		probe, stop := context.WithTimeout(ctx, 2*time.Second)
		reply, reloadErr := roundTrip(probe, m.socket, "RELOAD", nil)
		stop()
		if reloadErr != nil || reply != "RELOADING" {
			m.warn("reload_pending", reloadErr)
		} // clamd SelfCheck also picks up disk changes
	}
	m.mu.Lock()
	m.state.Updating = false
	m.mu.Unlock()
	return err
}

func (m *Manager) runClamd(ctx context.Context) {
	backoff := m.retryMin
	for ctx.Err() == nil {
		m.setState(ctx, "starting")
		started := time.Now()
		child, stop := context.WithCancel(ctx)
		cmd := m.cmd(child, "/usr/sbin/clamd", "--config-file="+filepath.Join(m.dir, "clamd.conf"))
		err := cmd.Start()
		if err == nil {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			err = m.monitor(ctx, started, done, stop)
		}
		stop()
		if ctx.Err() != nil {
			return
		}
		m.setState(ctx, "error")
		m.warn("daemon_unavailable", err)
		if time.Since(started) > 2*time.Minute {
			backoff = m.retryMin
		}
		if !delay(ctx, backoff, nil) {
			return
		}
		backoff = min(backoff*2, m.retryMax)
	}
}

func (m *Manager) monitor(ctx context.Context, started time.Time, done <-chan error, stop context.CancelFunc) error {
	ticker := time.NewTicker(m.probeInterval)
	defer ticker.Stop()
	ready := false
	missed := 0
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			stop()
			return <-done
		case <-ticker.C:
			probe, cancel := context.WithTimeout(ctx, time.Second)
			reply, err := roundTrip(probe, m.socket, "PING", nil)
			cancel()
			if err == nil && reply == "PONG" {
				ready, missed = true, 0
				m.setState(ctx, "running")
			} else if !ready && time.Since(started) > 2*time.Minute {
				stop()
				<-done
				return errors.New("clamd startup timed out")
			} else if ready {
				missed++
				if missed == 3 {
					m.setState(ctx, "error")
					m.warn("daemon_probe_failed", err)
				}
			}
		}
	}
}

func (m *Manager) warn(reason string, err error) {
	now := time.Now()
	m.mu.Lock()
	if now.Before(m.warnings[reason]) {
		m.mu.Unlock()
		return
	}
	m.warnings[reason] = now.Add(time.Minute)
	m.mu.Unlock()
	log.Warnf("ClamAV unavailable or update pending: reason=%s error=%v", reason, err)
}

func (m *Manager) Scan(ctx context.Context, raw []byte) *Result {
	m.mu.Lock()
	enabled, state := m.state.Enabled, m.state.State
	m.mu.Unlock()
	if !enabled {
		return nil
	}
	if state != "running" {
		return m.skipped("scanner_"+state, nil)
	}
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	default:
		return m.skipped("scanner_busy", nil)
	}
	ctx, cancel := context.WithTimeout(ctx, m.scanTimeout)
	defer cancel()
	virus, err := scanRaw(ctx, m.socket, raw)
	if err != nil {
		var ne net.Error
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
			return m.skipped("scan_timeout", err)
		case errors.Is(err, errLimits):
			return m.skipped("scan_limit", err)
		default:
			return m.skipped("scanner_unavailable", err)
		}
	}
	m.mu.Lock()
	hour := time.Now().Unix() / 3600
	b := &m.hourly[hour%int64(len(m.hourly))]
	if b.hour != hour {
		*b = counts{hour: hour}
	}
	b.scanned++
	if virus != "" {
		b.blocked++
	}
	m.mu.Unlock()
	if virus != "" {
		return &Result{Status: "infected", Virus: virus}
	}
	return &Result{Status: "clean"}
}

func (m *Manager) skipped(reason string, err error) *Result {
	m.warn(reason, err)
	return &Result{Status: "not_scanned", Reason: reason}
}
