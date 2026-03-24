package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kardianos/service"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/runtime"
)

const (
	ServiceName        = "ClashForClaw"
	ServiceDisplayName = "Clash for Claw"
	ServiceDescription = "Clash for Claw background service"
	legacyServiceName  = "OpenClawAdapter"
	startSettleTimeout = 5 * time.Second
	startPollInterval  = 250 * time.Millisecond
	startStablePolls   = 3
)

type Mode string

const (
	ModeNone    Mode = "none"
	ModeService Mode = "service"
	ModeTask    Mode = "task"
)

type Manager struct {
	userPaths    runtime.Paths
	servicePaths runtime.Paths
	exe          string
}

const localServiceUser = `NT AUTHORITY\LocalService`

func NewManager(baseOverride string, userPaths runtime.Paths) (*Manager, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	servicePaths, err := runtime.ResolveServicePaths(baseOverride)
	if err != nil {
		return nil, err
	}
	return &Manager{userPaths: userPaths, servicePaths: servicePaths, exe: exe}, nil
}

func (m *Manager) Install() (Mode, error) {
	mode, _, taskRegistered, err := m.currentRegistrations()
	if err != nil {
		return "", err
	}
	if mode == ModeService {
		if taskRegistered {
			_ = uninstallTask()
		}
		return mode, nil
	}

	if err := m.prepareServiceBase(); err != nil {
		return "", err
	}
	serviceErr := m.installService()
	if serviceErr == nil {
		if taskRegistered {
			_ = uninstallTask()
		}
		m.cleanupLegacyArtifacts()
		return ModeService, nil
	}
	mode, err = m.detectInstalledMode()
	if err == nil && mode == ModeService {
		if taskRegistered {
			_ = uninstallTask()
		}
		m.cleanupLegacyArtifacts()
		return mode, nil
	}
	if taskRegistered {
		m.cleanupLegacyArtifacts()
		return ModeTask, nil
	}
	taskErr := installTask(m.exe, m.userPaths.BaseDir)
	if taskErr == nil {
		m.cleanupLegacyArtifacts()
		return ModeTask, nil
	}
	mode, err = m.detectInstalledMode()
	if err == nil && mode != ModeNone {
		m.cleanupLegacyArtifacts()
		return mode, nil
	}
	return "", joinInstallErrors(serviceErr, taskErr)
}

func (m *Manager) Uninstall() error {
	mode, serviceStatus, taskRegistered, err := m.currentRegistrations()
	if err != nil {
		return err
	}

	var errs []error
	if taskRegistered {
		if err := uninstallTask(); err != nil {
			errs = append(errs, err)
		}
	}
	if mode == ModeService || serviceStatus != service.StatusUnknown {
		if err := m.uninstallService(); err != nil {
			errs = append(errs, err)
		}
	}
	m.cleanupLegacyArtifacts()
	return errors.Join(errs...)
}

func (m *Manager) Start() error {
	_, err := m.StartWithMode()
	return err
}

func (m *Manager) StartWithMode() (Mode, error) {
	mode, status, taskRegistered, err := m.currentRegistrations()
	if err != nil {
		return ModeNone, err
	}
	switch {
	case mode == ModeService:
		if status == service.StatusRunning {
			return ModeService, nil
		}
		if err := m.prepareServiceBase(); err != nil {
			if hasAccessDenied(err) {
				return ModeService, wrapCommandReason(ReasonServiceStartRequiresElevation, err)
			}
			return ModeService, err
		}
		if err := ensureServicePortFree(m.exe, m.servicePaths.ConfigPath); err != nil {
			return ModeService, err
		}
		if err := m.startService(); err != nil {
			if hasAccessDenied(err) {
				return ModeService, wrapCommandReason(ReasonServiceStartRequiresElevation, err)
			}
			return ModeService, err
		}
		if err := m.waitForServiceRunning(); err != nil {
			return ModeService, err
		}
		return ModeService, nil
	case taskRegistered:
		if err := startTask(); err != nil {
			if hasAccessDenied(err) {
				return ModeTask, wrapCommandReason(ReasonTaskStartRequiresTaskSchedulerAccess, err)
			}
			return ModeTask, err
		}
		return ModeTask, nil
	default:
		return ModeNone, errors.New(ServiceNotInstalledReason)
	}
}

func (m *Manager) Stop() error {
	mode, status, taskRegistered, err := m.currentRegistrations()
	if err != nil {
		return err
	}
	switch {
	case mode == ModeService:
		if status == service.StatusStopped {
			return nil
		}
		if err := m.stopService(); err != nil {
			return err
		}
		return m.waitForServiceStopped()
	case taskRegistered:
		return stopTask()
	default:
		return nil
	}
}

func (m *Manager) Status() (Mode, service.Status, error) {
	mode, status, taskRegistered, err := m.currentRegistrations()
	if err != nil {
		return ModeNone, service.StatusUnknown, err
	}
	if mode == ModeService {
		return mode, status, nil
	}
	if taskRegistered {
		return ModeTask, service.StatusUnknown, nil
	}
	return ModeNone, service.StatusUnknown, nil
}

func (m *Manager) service() service.Service {
	cfg := buildServiceConfig(ServiceName, ServiceDisplayName, ServiceDescription, m.servicePaths.BaseDir)
	svc, _ := service.New(&noopService{}, cfg)
	return svc
}

func (m *Manager) legacyService() service.Service {
	cfg := buildServiceConfig(legacyServiceName, "OpenClaw Adapter", "OpenClaw Adapter background service", m.servicePaths.BaseDir)
	svc, _ := service.New(&noopService{}, cfg)
	return svc
}

func ServiceArgs(baseDir string) []string {
	args := []string{"--service"}
	if baseDir != "" {
		args = append(args, "--base-dir", baseDir)
	}
	return args
}

func (m *Manager) installService() error {
	return m.service().Install()
}

func (m *Manager) uninstallService() error {
	return m.service().Uninstall()
}

func (m *Manager) startService() error {
	return m.service().Start()
}

func (m *Manager) stopService() error {
	return m.service().Stop()
}

func (m *Manager) currentRegistrations() (Mode, service.Status, bool, error) {
	serviceRegistered, serviceStatus, err := queryServiceRegistration(ServiceName)
	if err != nil {
		return ModeNone, service.StatusUnknown, false, err
	}

	taskRegistered, _, err := queryTaskRegistration(ServiceName)
	if err != nil {
		return ModeNone, service.StatusUnknown, false, err
	}

	if serviceRegistered {
		return ModeService, serviceStatus, taskRegistered, nil
	}
	return ModeNone, service.StatusUnknown, taskRegistered, nil
}

func buildServiceConfig(name string, displayName string, description string, baseDir string) *service.Config {
	return &service.Config{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		UserName:    localServiceUser,
		Arguments:   ServiceArgs(baseDir),
		Option: service.KeyValue{
			service.OnFailure:              service.OnFailureRestart,
			service.OnFailureDelayDuration: "2s",
			service.OnFailureResetPeriod:   30,
			service.StartType:              service.ServiceStartAutomatic,
		},
	}
}

func joinInstallErrors(serviceErr error, taskErr error) error {
	serviceWrapped := fmt.Errorf("service_install_failed: %w", serviceErr)
	taskWrapped := fmt.Errorf("task_install_failed: %w", taskErr)
	if hasAccessDenied(serviceErr) && hasAccessDenied(taskErr) {
		return fmt.Errorf("%s: %w", ReasonInstallRequiresElevationOrTaskSchedulerAccess, errors.Join(serviceWrapped, taskWrapped))
	}
	return errors.Join(serviceWrapped, taskWrapped)
}

func hasAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "access is denied") || strings.Contains(text, "拒绝访问")
}

func (m *Manager) detectInstalledMode() (Mode, error) {
	mode, _, taskRegistered, err := m.currentRegistrations()
	if err != nil {
		return ModeNone, err
	}
	if mode != ModeNone {
		return mode, nil
	}
	if taskRegistered {
		return ModeTask, nil
	}
	return ModeNone, nil
}

func (m *Manager) waitForServiceRunning() error {
	deadline := time.Now().Add(startSettleTimeout)
	runningPolls := 0

	for time.Now().Before(deadline) {
		mode, status, _, err := m.currentRegistrations()
		if err != nil {
			return err
		}
		if mode == ModeService {
			if status == service.StatusRunning {
				runningPolls++
				if runningPolls >= startStablePolls {
					return nil
				}
			} else if runningPolls > 0 {
				return fmt.Errorf("%s: service failed to remain running after start", ReasonStartFailed)
			}
		}
		time.Sleep(startPollInterval)
	}

	return fmt.Errorf("%s: service failed to remain running after start", ReasonStartFailed)
}

func (m *Manager) waitForServiceStopped() error {
	deadline := time.Now().Add(startSettleTimeout)

	for time.Now().Before(deadline) {
		mode, status, _, err := m.currentRegistrations()
		if err != nil {
			return err
		}
		if mode != ModeService || status == service.StatusStopped {
			return nil
		}
		time.Sleep(startPollInterval)
	}

	return fmt.Errorf("%s: service failed to stop cleanly", ReasonStopFailed)
}

func (m *Manager) cleanupLegacyArtifacts() {
	_ = uninstallTaskNamed(legacyServiceName)
	_ = m.legacyService().Stop()
	_ = m.legacyService().Uninstall()
	_ = os.RemoveAll(legacyUserBaseDir())
	_ = os.RemoveAll(legacyServiceBaseDir())
}

func legacyUserBaseDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return filepath.Join(base, "OpenClawAdapter")
}

func legacyServiceBaseDir() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return ""
	}
	return filepath.Join(filepath.Clean(programData), "OpenClawAdapter")
}

func (m *Manager) prepareServiceBase() error {
	if err := ensureServiceAccess(m.servicePaths); err != nil {
		return err
	}
	return syncServiceConfig(m.userPaths, m.servicePaths)
}

func syncServiceConfig(userPaths runtime.Paths, servicePaths runtime.Paths) error {
	userInfo, err := os.Stat(userPaths.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	serviceInfo, err := os.Stat(servicePaths.ConfigPath)
	if err == nil && !userInfo.ModTime().After(serviceInfo.ModTime()) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	userCfg, err := config.LoadOrInit(userPaths.ConfigPath)
	if err != nil {
		return err
	}
	serviceCfg := *userCfg
	if userCfg.Proxy.Subscriptions != nil {
		serviceCfg.Proxy.Subscriptions = append([]config.Subscription(nil), userCfg.Proxy.Subscriptions...)
	}
	if err := copySubscriptionFiles(&serviceCfg, servicePaths); err != nil {
		return err
	}
	return config.Save(servicePaths.ConfigPath, &serviceCfg)
}

func copySubscriptionFiles(cfg *config.Config, servicePaths runtime.Paths) error {
	if cfg == nil || len(cfg.Proxy.Subscriptions) == 0 {
		return nil
	}
	destDir := filepath.Join(servicePaths.RuntimeDir, "subscriptions")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for i := range cfg.Proxy.Subscriptions {
		sub := &cfg.Proxy.Subscriptions[i]
		if strings.TrimSpace(sub.FilePath) == "" {
			continue
		}
		if _, err := os.Stat(sub.FilePath); err != nil {
			continue
		}
		name := filepath.Base(sub.FilePath)
		if sub.ID != "" {
			name = sub.ID + "-" + name
		}
		destPath := filepath.Join(destDir, name)
		if err := copyFile(sub.FilePath, destPath); err != nil {
			return fmt.Errorf("copy subscription %s: %w", sub.ID, err)
		}
		sub.FilePath = destPath
	}
	return nil
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

type noopService struct{}

func (n *noopService) Start(service.Service) error { return nil }
func (n *noopService) Stop(service.Service) error  { return nil }
