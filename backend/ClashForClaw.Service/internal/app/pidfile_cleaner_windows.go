//go:build windows

package app

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

func runPIDFileCleaner(watchPID int, pidFilePath string) error {
	if err := waitForPIDExit(watchPID); err != nil {
		return err
	}
	return removePIDFileIfMatches(pidFilePath, watchPID)
}

func waitForPIDExit(pid int) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(handle)

	state, err := windows.WaitForSingleObject(handle, windows.INFINITE)
	if err != nil {
		return err
	}
	if state != uint32(windows.WAIT_OBJECT_0) && state != uint32(windows.WAIT_ABANDONED) {
		return errors.New("wait_for_pid_exit_failed")
	}
	return nil
}

func removePIDFileIfMatches(pidFilePath string, expectedPID int) error {
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid == expectedPID {
		if removeErr := os.Remove(pidFilePath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
	}
	return nil
}
