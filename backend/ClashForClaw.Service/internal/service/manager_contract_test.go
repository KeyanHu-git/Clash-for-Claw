package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kardianos/service"

	"clash-for-claw-service/internal/runtime"
)

var fakeWindowsToolsMu sync.Mutex

type fakeToolResponse struct {
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
}

type fakeToolConfig struct {
	Responses []fakeToolResponse `json:"responses"`
	Default   fakeToolResponse   `json:"default"`
}

const fakeWindowsToolSource = `package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type response struct {
	ExitCode int    ` + "`json:\"exitCode\"`" + `
	Output   string ` + "`json:\"output\"`" + `
}

type config struct {
	Responses []response ` + "`json:\"responses\"`" + `
	Default   response   ` + "`json:\"default\"`" + `
}

func main() {
	prefix := "FAKE_SC"
	base := strings.ToLower(filepath.Base(os.Args[0]))
	if strings.HasPrefix(base, "schtasks") {
		prefix = "FAKE_SCHTASKS"
	}

	cfgPath := os.Getenv(prefix + "_CONFIG")
	statePath := os.Getenv(prefix + "_STATE")
	cfg := config{}
	if cfgPath != "" {
		if data, err := os.ReadFile(cfgPath); err == nil {
			_ = json.Unmarshal(data, &cfg)
		}
	}

	index := nextIndex(statePath)
	resp := cfg.Default
	if index < len(cfg.Responses) {
		resp = cfg.Responses[index]
	} else if len(cfg.Responses) > 0 && resp == (response{}) {
		resp = cfg.Responses[len(cfg.Responses)-1]
	}

	if resp.Output != "" {
		fmt.Print(resp.Output)
	}
	os.Exit(resp.ExitCode)
}

func nextIndex(path string) int {
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		_ = os.WriteFile(path, []byte("1"), 0644)
		return 0
	}
	index, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		index = 0
	}
	_ = os.WriteFile(path, []byte(strconv.Itoa(index+1)), 0644)
	return index
}
`

func TestDetectInstalledModePrefersServiceWhenServiceAndTaskExist(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskRegistered(),
		},
	)

	mode, err := newTestManager().detectInstalledMode()
	if err != nil {
		t.Fatalf("detectInstalledMode returned error: %v", err)
	}
	if mode != ModeService {
		t.Fatalf("detectInstalledMode = %q, want %q", mode, ModeService)
	}
}

func TestDetectInstalledModeReturnsTaskWhenOnlyTaskExists(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Default: fakeTaskRegistered(),
		},
	)

	mode, err := newTestManager().detectInstalledMode()
	if err != nil {
		t.Fatalf("detectInstalledMode returned error: %v", err)
	}
	if mode != ModeTask {
		t.Fatalf("detectInstalledMode = %q, want %q", mode, ModeTask)
	}
}

func TestInstallPromotesExistingTaskToServiceWhenServiceAppearsAfterInstallAttempt(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceMissing(),
				fakeServiceRegistered(service.StatusStopped),
			},
			Default: fakeToolResponse{},
		},
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeTaskRegistered(),
				{},
			},
			Default: fakeTaskMissing(),
		},
	)

	mode, err := newTestManager().Install()
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if mode != ModeService {
		t.Fatalf("Install = %q, want %q", mode, ModeService)
	}
}

func TestInstallKeepsExistingTaskModeWhenServiceInstallFails(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceMissing(),
			},
			Default: fakeToolResponse{
				ExitCode: 5,
				Output:   "Access is denied.",
			},
		},
		fakeToolConfig{
			Default: fakeTaskRegistered(),
		},
	)

	mode, err := newTestManager().Install()
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if mode != ModeTask {
		t.Fatalf("Install = %q, want %q", mode, ModeTask)
	}
}

func TestInstallReturnsExistingServiceModeWithoutTouchingInstallPath(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	mode, err := newTestManager().Install()
	if err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if mode != ModeService {
		t.Fatalf("Install = %q, want %q", mode, ModeService)
	}
}

func TestStartWithModeIsIdempotentWhenServiceAlreadyRunning(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceRegistered(service.StatusRunning),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	mode, err := newTestManager().StartWithMode()
	if err != nil {
		t.Fatalf("StartWithMode returned error: %v", err)
	}
	if mode != ModeService {
		t.Fatalf("StartWithMode = %q, want %q", mode, ModeService)
	}
}

func TestStopIsIdempotentWhenServiceAlreadyStopped(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	if err := newTestManager().Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestWaitForRegistrationsClearedAllowsServiceDeletionToSettle(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceRegistered(service.StatusStopped),
				fakeServiceRegistered(service.StatusStopped),
			},
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	if err := newTestManager().waitForRegistrationsCleared(); err != nil {
		t.Fatalf("waitForRegistrationsCleared returned error: %v", err)
	}
}

func TestWaitForServiceRunningSucceedsWhenServiceStaysRunning(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceRegistered(service.StatusRunning),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	if err := newTestManager().waitForServiceRunning(); err != nil {
		t.Fatalf("waitForServiceRunning returned error: %v", err)
	}
}

func TestWaitForServiceRunningSucceedsAfterThreeConsecutiveRunningPolls(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceRegistered(service.StatusRunning),
				fakeServiceRegistered(service.StatusRunning),
				fakeServiceRegistered(service.StatusRunning),
			},
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	if err := newTestManager().waitForServiceRunning(); err != nil {
		t.Fatalf("waitForServiceRunning returned error: %v", err)
	}
}

func TestWaitForServiceRunningSucceedsWhenServiceAppearsAfterInitialMissingPolls(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceMissing(),
				fakeServiceMissing(),
				fakeServiceRegistered(service.StatusRunning),
				fakeServiceRegistered(service.StatusRunning),
				fakeServiceRegistered(service.StatusRunning),
			},
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	if err := newTestManager().waitForServiceRunning(); err != nil {
		t.Fatalf("waitForServiceRunning returned error: %v", err)
	}
}

func TestWaitForServiceRunningFailsWhenServiceDoesNotRemainRunning(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceRegistered(service.StatusRunning),
			},
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	err := newTestManager().waitForServiceRunning()
	if err == nil {
		t.Fatal("waitForServiceRunning returned nil, want error")
	}
	if !strings.Contains(err.Error(), ReasonStartFailed) {
		t.Fatalf("waitForServiceRunning error = %q, want token %q", err.Error(), ReasonStartFailed)
	}
}

func TestWaitForServiceRunningFailsWhenOnlyTwoRunningPollsAreObserved(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceRegistered(service.StatusRunning),
				fakeServiceRegistered(service.StatusRunning),
			},
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	err := newTestManager().waitForServiceRunning()
	if err == nil {
		t.Fatal("waitForServiceRunning returned nil, want error")
	}
	if !strings.Contains(err.Error(), ReasonStartFailed) {
		t.Fatalf("waitForServiceRunning error = %q, want token %q", err.Error(), ReasonStartFailed)
	}
}

func TestWaitForServiceRunningPropagatesQueryErrors(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeToolResponse{
				ExitCode: 2,
				Output:   "query exploded",
			},
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	err := newTestManager().waitForServiceRunning()
	if err == nil {
		t.Fatal("waitForServiceRunning returned nil, want error")
	}
	if !strings.Contains(err.Error(), "query exploded") {
		t.Fatalf("waitForServiceRunning error = %q, want propagated query detail", err.Error())
	}
}

func TestStartWithModeReturnsTaskWhenTaskRegistered(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeTaskRegistered(),
				{},
			},
			Default: fakeTaskMissing(),
		},
	)

	mode, err := newTestManager().StartWithMode()
	if err != nil {
		t.Fatalf("StartWithMode returned error: %v", err)
	}
	if mode != ModeTask {
		t.Fatalf("StartWithMode = %q, want %q", mode, ModeTask)
	}
}

func TestStartWithModeWrapsTaskAccessDenied(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeTaskRegistered(),
				{
					ExitCode: 5,
					Output:   "ERROR: Access is denied.",
				},
			},
			Default: fakeTaskMissing(),
		},
	)

	mode, err := newTestManager().StartWithMode()
	if err == nil {
		t.Fatal("StartWithMode returned nil, want error")
	}
	if mode != ModeTask {
		t.Fatalf("StartWithMode mode = %q, want %q", mode, ModeTask)
	}
	if !strings.Contains(err.Error(), ReasonTaskStartRequiresTaskSchedulerAccess) {
		t.Fatalf("StartWithMode error = %q, want token %q", err.Error(), ReasonTaskStartRequiresTaskSchedulerAccess)
	}
}

func TestStatusReturnsTaskWhenOnlyTaskIsRegistered(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Default: fakeTaskRegistered(),
		},
	)

	mode, status, err := newTestManager().Status()
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if mode != ModeTask {
		t.Fatalf("Status mode = %q, want %q", mode, ModeTask)
	}
	if status != service.StatusUnknown {
		t.Fatalf("Status status = %v, want %v", status, service.StatusUnknown)
	}
}

func TestStatusReturnsNoneWhenNothingIsRegistered(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	mode, status, err := newTestManager().Status()
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if mode != ModeNone {
		t.Fatalf("Status mode = %q, want %q", mode, ModeNone)
	}
	if status != service.StatusUnknown {
		t.Fatalf("Status status = %v, want %v", status, service.StatusUnknown)
	}
}

func TestStartWithModeReturnsNotInstalledReasonWhenNothingIsRegistered(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	mode, err := newTestManager().StartWithMode()
	if err == nil {
		t.Fatal("StartWithMode returned nil, want error")
	}
	if mode != ModeNone {
		t.Fatalf("StartWithMode mode = %q, want %q", mode, ModeNone)
	}
	if !strings.Contains(err.Error(), ServiceNotInstalledReason) {
		t.Fatalf("StartWithMode error = %q, want token %q", err.Error(), ServiceNotInstalledReason)
	}
}

func TestStopReturnsNilWhenNothingIsRegistered(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	if err := newTestManager().Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestStopTreatsServiceAlreadyStoppedAfterStopSignalAsSuccess(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceRegistered(service.StatusRunning),
				fakeServiceRegistered(service.StatusStopped),
			},
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	manager := newTestManager()
	manager.stopFn = func() error {
		return errors.New("service not active")
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestStopWaitsThroughStopPendingAfterStopSignalError(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceRegistered(service.StatusRunning),
				fakeServiceStopPending(),
				fakeServiceRegistered(service.StatusStopped),
			},
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	manager := newTestManager()
	manager.stopFn = func() error {
		return errors.New("service not active")
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestUninstallWaitsForServiceRegistrationToDisappear(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceRegistered(service.StatusRunning),
				fakeServiceRegistered(service.StatusRunning),
				fakeServiceRegistered(service.StatusStopped),
				fakeServiceRegistered(service.StatusStopped),
				fakeServiceMissing(),
			},
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	stopCalled := false
	uninstallCalled := false
	manager := newTestManager()
	manager.stopFn = func() error {
		stopCalled = true
		return nil
	}
	manager.uninstallFn = func() error {
		uninstallCalled = true
		return nil
	}

	if err := manager.Uninstall(); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	if !stopCalled {
		t.Fatal("stop function was not called")
	}
	if !uninstallCalled {
		t.Fatal("uninstall function was not called")
	}
}

func TestUninstallSuppressesTransientDeleteErrorWhenRegistrationClears(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeServiceRegistered(service.StatusStopped),
				fakeServiceRegistered(service.StatusStopped),
				fakeServiceRegistered(service.StatusStopped),
			},
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	manager := newTestManager()
	manager.uninstallFn = func() error {
		return errors.New("DeleteService pending")
	}

	if err := manager.Uninstall(); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
}

func TestUninstallRemovesTaskRegistrationIdempotently(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceMissing(),
		},
		fakeToolConfig{
			Responses: []fakeToolResponse{
				fakeTaskRegistered(),
				{},
				fakeTaskMissing(),
			},
			Default: fakeTaskMissing(),
		},
	)

	if err := newTestManager().Uninstall(); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
}

func withFakeWindowsTools(t *testing.T, scCfg fakeToolConfig, taskCfg fakeToolConfig) {
	t.Helper()

	fakeWindowsToolsMu.Lock()
	t.Cleanup(fakeWindowsToolsMu.Unlock)

	dir := t.TempDir()
	buildFakeWindowsTool(t, dir, "sc.exe")
	buildFakeWindowsTool(t, dir, "schtasks.exe")

	writeFakeToolConfig(t, filepath.Join(dir, "sc-config.json"), scCfg)
	writeFakeToolConfig(t, filepath.Join(dir, "schtasks-config.json"), taskCfg)

	t.Setenv("FAKE_SC_CONFIG", filepath.Join(dir, "sc-config.json"))
	t.Setenv("FAKE_SC_STATE", filepath.Join(dir, "sc-state.txt"))
	t.Setenv("FAKE_SCHTASKS_CONFIG", filepath.Join(dir, "schtasks-config.json"))
	t.Setenv("FAKE_SCHTASKS_STATE", filepath.Join(dir, "schtasks-state.txt"))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q) failed: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})
}

func buildFakeWindowsTool(t *testing.T, dir string, exeName string) {
	t.Helper()

	sourcePath := filepath.Join(dir, strings.TrimSuffix(exeName, ".exe")+".go")
	if err := os.WriteFile(sourcePath, []byte(fakeWindowsToolSource), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", sourcePath, err)
	}

	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, exeName), sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("building fake tool %q failed: %v\n%s", exeName, err, string(output))
	}
}

func writeFakeToolConfig(t *testing.T, path string, cfg fakeToolConfig) {
	t.Helper()

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", path, err)
	}
}

func newTestManager() *Manager {
	root, err := os.MkdirTemp("", "clashforclaw-service-manager-test-*")
	if err != nil {
		panic(err)
	}

	userBase := filepath.Join(root, "user")
	serviceBase := filepath.Join(root, "service")
	return &Manager{
		userPaths: runtime.Paths{
			BaseDir:    userBase,
			ConfigPath: filepath.Join(userBase, "config.json"),
			RuntimeDir: filepath.Join(userBase, "runtime"),
			BinDir:     filepath.Join(userBase, "bin"),
			LogsDir:    filepath.Join(userBase, "logs"),
			MihomoDir:  filepath.Join(userBase, "mihomo"),
		},
		servicePaths: runtime.Paths{
			BaseDir:    serviceBase,
			ConfigPath: filepath.Join(serviceBase, "config.json"),
			RuntimeDir: filepath.Join(serviceBase, "runtime"),
			BinDir:     filepath.Join(serviceBase, "bin"),
			LogsDir:    filepath.Join(serviceBase, "logs"),
			MihomoDir:  filepath.Join(serviceBase, "mihomo"),
		},
		exe: `C:\Program Files\ClashForClaw\ClashForClaw.Service.exe`,
	}
}

func fakeServiceRegistered(status service.Status) fakeToolResponse {
	state := 1
	label := "STOPPED"
	if status == service.StatusRunning {
		state = 4
		label = "RUNNING"
	}
	return fakeToolResponse{
		Output: fmt.Sprintf("SERVICE_NAME: %s\r\n        STATE              : %d  %s\r\n", ServiceName, state, label),
	}
}

func fakeServiceMissing() fakeToolResponse {
	return fakeToolResponse{
		ExitCode: 1060,
		Output:   "[SC] EnumQueryServicesStatus:OpenService FAILED 1060:",
	}
}

func fakeServiceStopPending() fakeToolResponse {
	return fakeToolResponse{
		Output: fmt.Sprintf("SERVICE_NAME: %s\r\n        STATE              : 3  STOP_PENDING\r\n", ServiceName),
	}
}

func fakeTaskRegistered() fakeToolResponse {
	return fakeToolResponse{
		Output: fmt.Sprintf("TaskName: \\%s\r\n", ServiceName),
	}
}

func fakeTaskMissing() fakeToolResponse {
	return fakeToolResponse{
		ExitCode: 1,
		Output:   "ERROR: The system cannot find the file specified.",
	}
}

func TestJoinInstallErrorsUsesElevationReasonWhenBothSidesAreAccessDenied(t *testing.T) {
	err := joinInstallErrors(
		errors.New("Access is denied."),
		errors.New("ERROR: 拒绝访问。"),
	)

	if err == nil {
		t.Fatal("joinInstallErrors returned nil, want error")
	}
	if !strings.Contains(err.Error(), ReasonInstallRequiresElevationOrTaskSchedulerAccess) {
		t.Fatalf("joinInstallErrors error = %q, want token %q", err.Error(), ReasonInstallRequiresElevationOrTaskSchedulerAccess)
	}
}
