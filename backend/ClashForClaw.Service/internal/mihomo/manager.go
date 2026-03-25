package mihomo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"clash-for-claw-service/internal/config"
	adapterRuntime "clash-for-claw-service/internal/runtime"
	"clash-for-claw-service/internal/winproc"
)

const (
	minMihomoSize  = 1024 * 1024
	managedPIDFile = "mihomo.pid"
)

type Manager struct {
	mu             sync.Mutex
	paths          adapterRuntime.Paths
	cmd            *exec.Cmd
	exitCh         chan error
	jobHandle      uintptr
	pid            int
	binPath        string
	mixedPort      int
	httpPort       int
	socksPort      int
	controllerPort int
}

type RuntimeState struct {
	Active bool
	PID    int
	Error  string
}

func NewManager(paths adapterRuntime.Paths) *Manager {
	return &Manager{paths: paths}
}

func (m *Manager) Apply(cfg *config.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg.Proxy.SubscriptionURL == "" {
		return errors.New("subscription_url_required")
	}

	binPath, err := m.ensureBinary()
	if err != nil {
		return err
	}
	if err := m.writeConfig(cfg); err != nil {
		return err
	}

	m.mixedPort = cfg.Proxy.Mihomo.MixedPort
	m.httpPort = cfg.Proxy.Mihomo.HTTPPort
	m.socksPort = cfg.Proxy.Mihomo.SocksPort
	m.controllerPort = cfg.Proxy.Mihomo.ControllerPort

	if m.cmd != nil || m.pid != 0 {
		_ = m.stopLocked()
	}
	return m.startLocked(binPath)
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *Manager) RuntimeState() RuntimeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runtimeStateLocked()
}

func (m *Manager) stopLocked() error {
	var errs []error
	if m.cmd != nil && m.cmd.Process != nil && m.pid > 0 {
		if err := m.terminateTrackedLocked(3 * time.Second); err != nil {
			errs = append(errs, err)
		}
	}
	if err := cleanupManagedPIDFile(m.pidFilePath(), m.expectedBinPathLocked()); err != nil {
		errs = append(errs, err)
	}
	if err := cleanupManagedPortOwners(m.expectedBinPathLocked(), m.expectedPortsLocked(), 0); err != nil {
		errs = append(errs, err)
	}
	if err := m.closeJobLocked(); err != nil {
		errs = append(errs, err)
	}
	if err := m.removePIDFile(); err != nil {
		errs = append(errs, err)
	}
	m.cmd = nil
	m.exitCh = nil
	m.jobHandle = 0
	m.pid = 0
	m.binPath = ""
	return errors.Join(errs...)
}

func (m *Manager) startLocked(binPath string) error {
	if err := cleanupManagedPIDFile(m.pidFilePath(), binPath); err != nil {
		return err
	}
	if err := cleanupManagedPortOwners(binPath, m.expectedPortsLocked(), 0); err != nil {
		return err
	}

	configPath := filepath.Join(m.paths.MihomoDir, "config.yaml")
	cmd := exec.Command(binPath, "-f", configPath, "-d", m.paths.MihomoDir)
	cmd.Stdout = openLogFile(filepath.Join(m.paths.LogsDir, "mihomo.out.log"))
	cmd.Stderr = openLogFile(filepath.Join(m.paths.LogsDir, "mihomo.err.log"))
	if err := cmd.Start(); err != nil {
		return err
	}
	jobHandle, err := attachKillOnCloseJob(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return err
	}

	exitCh := make(chan error, 1)
	go func() {
		exitCh <- cmd.Wait()
		close(exitCh)
	}()

	m.cmd = cmd
	m.exitCh = exitCh
	m.jobHandle = jobHandle
	m.pid = cmd.Process.Pid
	m.binPath = binPath
	if err := m.writePIDFile(m.pid); err != nil {
		_ = m.terminateTrackedLocked(2 * time.Second)
		_ = m.closeJobLocked()
		return err
	}
	_ = startPIDFileCleanerProcess(m.pid, m.pidFilePath())

	if err := m.waitReadyLocked(8 * time.Second); err != nil {
		_ = m.terminateTrackedLocked(2 * time.Second)
		_ = m.closeJobLocked()
		_ = cleanupManagedPIDFile(m.pidFilePath(), binPath)
		_ = cleanupManagedPortOwners(binPath, m.expectedPortsLocked(), 0)
		_ = m.removePIDFile()
		return err
	}
	return nil
}

func (m *Manager) waitReadyLocked(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastErr := "mihomo_not_ready"
	for time.Now().Before(deadline) {
		state := m.runtimeStateLocked()
		if state.Active {
			return nil
		}
		if state.Error != "" {
			lastErr = state.Error
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New(lastErr)
}

func (m *Manager) closeJobLocked() error {
	if m.jobHandle == 0 {
		return nil
	}
	err := closeJobHandle(m.jobHandle)
	m.jobHandle = 0
	return err
}

func (m *Manager) terminateTrackedLocked(timeout time.Duration) error {
	if m.cmd == nil || m.cmd.Process == nil || m.pid <= 0 {
		return nil
	}
	proc := m.cmd.Process
	exitCh := m.exitCh
	_ = proc.Signal(os.Interrupt)
	if waitForExit(exitCh, timeout) {
		return nil
	}
	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	if waitForExit(exitCh, 2*time.Second) {
		return nil
	}
	return errors.New("mihomo_stop_timeout")
}

func waitForExit(exitCh <-chan error, timeout time.Duration) bool {
	if exitCh == nil {
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-exitCh:
		return true
	case <-timer.C:
		return false
	}
}

func (m *Manager) runtimeStateLocked() RuntimeState {
	if m.pid <= 0 {
		return RuntimeState{Active: false, Error: "mihomo_not_started"}
	}
	if !winproc.IsProcessRunning(m.pid) {
		return RuntimeState{Active: false, PID: m.pid, Error: "mihomo_process_exited"}
	}
	ports := m.expectedPortsLocked()
	if len(ports) == 0 {
		return RuntimeState{Active: false, PID: m.pid, Error: "mihomo_ports_not_configured"}
	}
	ok, errText := portsOwnedByPID(m.pid, ports)
	if !ok {
		return RuntimeState{Active: false, PID: m.pid, Error: errText}
	}
	return RuntimeState{Active: true, PID: m.pid}
}

func (m *Manager) expectedPortsLocked() []int {
	ports := []int{m.mixedPort, m.httpPort, m.socksPort, m.controllerPort}
	seen := make(map[int]bool, len(ports))
	out := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 || seen[port] {
			continue
		}
		seen[port] = true
		out = append(out, port)
	}
	return out
}

func (m *Manager) expectedBinPathLocked() string {
	if m.binPath != "" {
		return m.binPath
	}
	return filepath.Join(m.paths.BinDir, mihomoExecutableName)
}

func (m *Manager) pidFilePath() string {
	return filepath.Join(m.paths.RuntimeDir, managedPIDFile)
}

func (m *Manager) writePIDFile(pid int) error {
	return os.WriteFile(m.pidFilePath(), []byte(fmt.Sprintf("%d", pid)), 0o600)
}

func (m *Manager) removePIDFile() error {
	if err := os.Remove(m.pidFilePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m *Manager) ensureBinary() (string, error) {
	resolution, err := resolveBinaryPath(m.paths, currentExecutablePath(), os.Getenv)
	if err == nil {
		if !resolution.CopyToManaged {
			return resolution.Path, nil
		}
		return ensureManagedCopy(resolution.Path, filepath.Join(m.paths.BinDir, mihomoExecutableName))
	}

	if !errors.Is(err, errMihomoBinaryNotFound) {
		return "", err
	}

	return ensureDownloadedBinary(m.paths, os.Getenv)
}

func (m *Manager) writeConfig(cfg *config.Config) error {
	data, err := buildConfig(cfg, m.paths)
	if err != nil {
		return err
	}
	path := filepath.Join(m.paths.MihomoDir, "config.yaml")
	return os.WriteFile(path, data, 0o600)
}

func openLogFile(path string) io.Writer {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return io.Discard
	}
	return f
}
