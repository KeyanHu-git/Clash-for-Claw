//go:build windows

package winproc

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func ListeningPortOwners(ports []int) (map[int]int, error) {
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

func ProcessImagePath(pid int) (string, error) {
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

func IsProcessRunning(pid int) bool {
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

func WaitForExit(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}

	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if !IsProcessRunning(pid) {
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

func SameExecutablePath(actual string, expected string) bool {
	return normalizePath(actual) == normalizePath(expected)
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

func normalizePath(path string) string {
	trimmed := strings.TrimPrefix(path, `\\?\`)
	return strings.ToLower(filepath.Clean(trimmed))
}
