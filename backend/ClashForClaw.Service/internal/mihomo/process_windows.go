//go:build windows

package mihomo

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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
	owners, err := listeningPortOwners(ports)
	if err != nil {
		return err
	}
	expected := normalizePath(binPath)
	seen := make(map[int]bool)
	var errs []error
	for _, port := range ports {
		pid, ok := owners[port]
		if !ok || pid <= 0 || pid == skipPID || seen[pid] {
			continue
		}
		seen[pid] = true
		imagePath, err := processImagePath(pid)
		if err != nil {
			errs = append(errs, fmt.Errorf("process_path_%d: %w", pid, err))
			continue
		}
		if !sameExecutablePath(imagePath, expected) {
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
	if pid <= 0 || !isProcessRunning(pid) {
		return nil
	}
	imagePath, err := processImagePath(pid)
	if err != nil {
		return err
	}
	if !sameExecutablePath(imagePath, expectedPath) {
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
	owners, err := listeningPortOwners(ports)
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

func listeningPortOwners(ports []int) (map[int]int, error) {
	if len(ports) == 0 {
		return map[int]int{}, nil
	}
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseNetstatPortOwners(string(output), ports), nil
}

func parseNetstatPortOwners(raw string, ports []int) map[int]int {
	targets := make(map[int]bool, len(ports))
	for _, port := range ports {
		if port > 0 {
			targets[port] = true
		}
	}

	owners := make(map[int]int, len(targets))
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || !strings.EqualFold(fields[0], "TCP") {
			continue
		}
		state := fields[len(fields)-2]
		if !strings.EqualFold(state, "LISTENING") {
			continue
		}
		port, ok := parseWindowsPort(fields[1])
		if !ok || !targets[port] {
			continue
		}
		pid, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			continue
		}
		owners[port] = pid
	}
	return owners
}

func parseWindowsPort(address string) (int, bool) {
	index := strings.LastIndex(address, ":")
	if index < 0 || index == len(address)-1 {
		return 0, false
	}
	port, err := strconv.Atoi(address[index+1:])
	if err != nil {
		return 0, false
	}
	return port, true
}

func processImagePath(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false
	}
	return state == uint32(windows.WAIT_TIMEOUT)
}

func waitForManagedProcessExit(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}

	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if !isProcessRunning(pid) {
			return nil
		}
		return err
	}
	defer windows.CloseHandle(handle)

	waitMillis := uint32(1)
	if timeout <= 0 {
		waitMillis = 0
	} else {
		waitMillis = uint32(timeout / time.Millisecond)
		if waitMillis == 0 {
			waitMillis = 1
		}
	}

	state, err := windows.WaitForSingleObject(handle, waitMillis)
	if err != nil {
		return err
	}
	switch state {
	case uint32(windows.WAIT_OBJECT_0):
		return nil
	case uint32(windows.WAIT_TIMEOUT):
		return fmt.Errorf("process_exit_timeout")
	default:
		return fmt.Errorf("wait_failed_%d", state)
	}
}

func normalizePath(path string) string {
	trimmed := strings.TrimPrefix(path, `\\?\`)
	return strings.ToLower(filepath.Clean(trimmed))
}

func sameExecutablePath(actual string, expected string) bool {
	return normalizePath(actual) == normalizePath(expected)
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
