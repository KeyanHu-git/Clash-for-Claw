//go:build windows

package service

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"clash-for-claw-service/internal/config"
)

const servicePortReleaseTimeout = 3 * time.Second

func ensureServicePortFree(serviceExe string, configPath string) error {
	port, err := serviceHTTPPort(configPath)
	if err != nil {
		return err
	}
	if port <= 0 {
		return nil
	}

	deadline := time.Now().Add(servicePortReleaseTimeout)
	for time.Now().Before(deadline) {
		owners, err := listeningPortOwners([]int{port})
		if err != nil {
			return err
		}

		pid, ok := owners[port]
		if !ok || pid <= 0 {
			return nil
		}

		matches, err := processMatchesServiceExecutable(pid, serviceExe)
		if err != nil {
			if !isProcessRunning(pid) {
				time.Sleep(150 * time.Millisecond)
				continue
			}
			return fmt.Errorf("service_port_owner_path_%d: %w", pid, err)
		}
		if !matches {
			return fmt.Errorf("service_port_%d_in_use_by_%d", port, pid)
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("find_process_%d: %w", pid, err)
		}
		if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill_process_%d: %w", pid, err)
		}
		if err := waitForServiceProcessExit(pid, time.Until(deadline)); err != nil {
			return fmt.Errorf("wait_process_%d: %w", pid, err)
		}
	}

	return fmt.Errorf("service_port_release_timeout")
}

func processMatchesServiceExecutable(pid int, serviceExe string) (bool, error) {
	imagePath, err := processImagePath(pid)
	if err == nil {
		return sameExecutablePath(imagePath, serviceExe), nil
	}

	imageName, nameErr := processImageName(pid)
	if nameErr != nil {
		return false, err
	}

	return strings.EqualFold(imageName, filepath.Base(serviceExe)), nil
}

func serviceHTTPPort(configPath string) (int, error) {
	cfg, err := config.LoadOrInit(configPath)
	if err != nil {
		return 0, err
	}
	if cfg.HTTP.Port > 0 {
		return cfg.HTTP.Port, nil
	}
	return config.DefaultHTTPPort, nil
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
		if !strings.EqualFold(fields[len(fields)-2], "LISTENING") {
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

func processImageName(pid int) (string, error) {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	line := strings.TrimSpace(string(output))
	if line == "" || strings.HasPrefix(line, "INFO:") {
		return "", fmt.Errorf("tasklist_missing_%d", pid)
	}

	line = strings.Trim(line, "\r\n")
	if strings.HasPrefix(line, "\"") && strings.Contains(line, "\",\"") {
		parts := strings.Split(line, "\",\"")
		if len(parts) > 0 {
			return strings.Trim(parts[0], "\""), nil
		}
	}

	return "", fmt.Errorf("tasklist_parse_%d", pid)
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

func waitForServiceProcessExit(pid int, timeout time.Duration) error {
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

func sameExecutablePath(actual string, expected string) bool {
	return normalizePath(actual) == normalizePath(expected)
}

func normalizePath(path string) string {
	trimmed := strings.TrimPrefix(path, `\\?\`)
	return strings.ToLower(filepath.Clean(trimmed))
}
