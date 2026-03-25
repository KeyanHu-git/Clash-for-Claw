//go:build windows

package service

import (
	"errors"
	"fmt"
	"os"
	"time"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/winproc"
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
		owners, err := winproc.ListeningPortOwners([]int{port})
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
	imagePath, err := winproc.ProcessImagePath(pid)
	if err != nil {
		return false, err
	}

	return winproc.SameExecutablePath(imagePath, serviceExe), nil
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

func isProcessRunning(pid int) bool {
	return winproc.IsProcessRunning(pid)
}

func waitForServiceProcessExit(pid int, timeout time.Duration) error {
	return winproc.WaitForExit(pid, timeout)
}
