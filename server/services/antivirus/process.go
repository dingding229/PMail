package antivirus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Jinnrry/pmail/utils/mailbudget"
)

type commandFunc func(context.Context, string, ...string) *exec.Cmd

func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	// Fixed packaged executables; the only argument is a generated config path.
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- only fixed ClamAV binaries and PMail-generated options are passed here.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 4 * time.Second // escalate and reap even if graceful shutdown is stuck
	return cmd
}

func (m *Manager) prepare() error {
	if !filepath.IsAbs(m.dir) || strings.ContainsAny(m.dir, "\r\n\x00") {
		return errors.New("invalid ClamAV directory")
	}
	if err := os.Mkdir(m.dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(m.dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("ClamAV path must be a real directory")
	}
	if err := os.Chmod(m.dir, 0700); err != nil { // #nosec G302 -- this is a real directory; owner traverse permission is required.
		return err
	}
	clamd := fmt.Sprintf(`Foreground yes
LocalSocket %s
LocalSocketMode 600
FixStaleSocket yes
DatabaseDirectory %s
StreamMaxLength %d
MaxFileSize %d
MaxScanSize 128M
AlertExceedsMax yes
MaxThreads 4
MaxQueue 12
ReadTimeout 30
CommandReadTimeout 5
SelfCheck 600
`, m.socket, m.dir, mailbudget.Message, mailbudget.Message)
	fresh := fmt.Sprintf(`DatabaseDirectory %s
DatabaseOwner root
DatabaseMirror database.clamav.net
ConnectTimeout 20
ReceiveTimeout 90
MaxAttempts 2
`, m.dir)
	if err := writeConfig(m.dir, "clamd.conf", clamd); err != nil {
		return err
	}
	return writeConfig(m.dir, "freshclam.conf", fresh)
}

func writeConfig(dir, name, content string) error {
	file, err := os.CreateTemp(dir, ".conf-*") // mode 0600
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := io.WriteString(file, content)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dir, name))
}

type database struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Use the CVD/CLD header's version and the file's installation time. If
// both variants exist, use the latest. The daemon verifies database content.
func readDatabase(dir, name string) *database {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil
	}
	defer root.Close()
	var latest *database
	for _, suffix := range []string{".cvd", ".cld"} {
		file, err := root.Open(name + suffix)
		if err != nil {
			continue
		}
		info, statErr := file.Stat()
		var header [512]byte
		_, readErr := io.ReadFull(file, header[:])
		_ = file.Close()
		if statErr != nil || readErr != nil || !info.Mode().IsRegular() {
			continue
		}
		fields := strings.SplitN(string(header[:]), ":", 4)
		if len(fields) < 4 || fields[0] != "ClamAV-VDB" {
			continue
		}
		version, err := strconv.Atoi(fields[2])
		if err != nil || version <= 0 {
			continue
		}
		if latest == nil || version > latest.Version || (version == latest.Version && info.ModTime().After(latest.UpdatedAt)) {
			latest = &database{version, info.ModTime().UTC()}
		}
	}
	return latest
}

const lastUpdateFile = "freshclam-result.json"

func readUpdateResult(dir string) *UpdateResult {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil
	}
	defer root.Close()
	file, err := root.Open(lastUpdateFile)
	if err != nil {
		return nil
	}
	defer file.Close()
	var result UpdateResult
	if json.NewDecoder(io.LimitReader(file, 512)).Decode(&result) != nil || result.At.IsZero() || (result.Status != "success" && result.Status != "failed") {
		return nil
	}
	return &result
}

func writeUpdateResult(dir string, result *UpdateResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return writeConfig(dir, lastUpdateFile, string(data))
}

func availableMemory() uint64 {
	// Linux's MemAvailable is a better estimate than MemFree. Also honor a
	// stricter cgroup budget if the container was given one.
	var available uint64
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "MemAvailable:" {
				if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil && kb <= (1<<63)/1024 {
					available = kb * 1024
				}
				break
			}
		}
	}
	for _, files := range [][2]string{{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.current"}, {"/sys/fs/cgroup/memory/memory.limit_in_bytes", "/sys/fs/cgroup/memory/memory.usage_in_bytes"}} {
		maxText, err1 := os.ReadFile(files[0])
		usageText, err2 := os.ReadFile(files[1])
		limit, err3 := strconv.ParseUint(strings.TrimSpace(string(maxText)), 10, 64)
		usage, err4 := strconv.ParseUint(strings.TrimSpace(string(usageText)), 10, 64)
		if err1 == nil && err2 == nil && err3 == nil && err4 == nil && limit < (1<<60) {
			free := uint64(1) // zero is reserved for unknown
			if limit > usage {
				free = limit - usage
			}
			if available == 0 || free < available {
				available = free
			}
		}
	}
	return available
}
