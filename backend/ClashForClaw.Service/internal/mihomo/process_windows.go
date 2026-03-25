//go:build windows

package mihomo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"clash-for-claw-service/internal/winproc"
)

const managedProcessExitTimeout = 2 * time.Second

func attachKillOnCloseJob(pid int) (uintptr, error) {
	if pid <= 0 {
		return 0, nil
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}

	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	defer windows.CloseHandle(process)

	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}

	return uintptr(job), nil
}

func startPIDFileCleanerProcess(watchPID int, pidFilePath string) error {
	if watchPID <= 0 || strings.TrimSpace(pidFilePath) == "" {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(
		exe,
		"--pid-file-cleaner",
		"--watch-pid", strconv.Itoa(watchPID),
		"--pid-file", pidFilePath,
	)
	cmd.Stdout = ioDiscard{}
	cmd.Stderr = ioDiscard{}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func closeJobHandle(jobHandle uintptr) error {
	if jobHandle == 0 {
		return nil
	}
	return windows.CloseHandle(windows.Handle(jobHandle))
}

func cleanupManagedPortOwners(binPath string, ports []int, skipPID int) error {
	if len(ports) == 0 {
		return nil
	}
	owners, err := winproc.ListeningPortOwners(ports)
	if err != nil {
		return err
	}
	seen := make(map[int]bool)
	var errs []error
	for _, port := range ports {
		pid, ok := owners[port]
		if !ok || pid <= 0 || pid == skipPID || seen[pid] {
			continue
		}
		seen[pid] = true
		imagePath, err := winproc.ProcessImagePath(pid)
		if err != nil {
			errs = append(errs, fmt.Errorf("process_path_%d: %w", pid, err))
			continue
		}
		if !winproc.SameExecutablePath(imagePath, binPath) {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			errs = append(errs, fmt.Errorf("find_process_%d: %w", pid, err))
			continue
		}
		if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			errs = append(errs, fmt.Errorf("kill_process_%d: %w", pid, err))
			continue
		}
		if err := waitForManagedProcessExit(pid, managedProcessExitTimeout); err != nil {
			errs = append(errs, fmt.Errorf("wait_process_%d: %w", pid, err))
		}
	}
	return errors.Join(errs...)
}

func cleanupManagedPIDFile(pidFilePath string, expectedPath string) error {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		_ = os.Remove(pidFilePath)
		return nil
	}
	if err := killManagedProcess(pid, expectedPath); err != nil {
		return err
	}
	_ = os.Remove(pidFilePath)
	return nil
}

func killManagedProcess(pid int, expectedPath string) error {
	if pid <= 0 || !winproc.IsProcessRunning(pid) {
		return nil
	}
	imagePath, err := winproc.ProcessImagePath(pid)
	if err != nil {
		return err
	}
	if !winproc.SameExecutablePath(imagePath, expectedPath) {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return waitForManagedProcessExit(pid, managedProcessExitTimeout)
}

func portsOwnedByPID(pid int, ports []int) (bool, string) {
	owners, err := winproc.ListeningPortOwners(ports)
	if err != nil {
		return false, "mihomo_port_check_failed"
	}
	for _, port := range ports {
		owner, ok := owners[port]
		if !ok {
			return false, fmt.Sprintf("mihomo_port_%d_missing", port)
		}
		if owner != pid {
			return false, fmt.Sprintf("mihomo_port_%d_owned_by_%d", port, owner)
		}
	}
	return true, ""
}

func waitForManagedProcessExit(pid int, timeout time.Duration) error {
	return winproc.WaitForExit(pid, timeout)
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
