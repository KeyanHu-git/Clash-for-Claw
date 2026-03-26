//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kardianos/service"
)

func installTask(exe string, baseDir string) error {
	return installTaskNamed(ServiceName, exe, baseDir)
}

func installTaskNamed(taskName string, exe string, baseDir string) error {
	task, err := buildTaskCommand(exe, baseDir)
	if err != nil {
		return err
	}
	if err := runSchtasks("/Create", "/TN", taskName, "/TR", task, "/SC", "ONLOGON", "/RL", "HIGHEST", "/F"); err == nil {
		return nil
	}
	// Fallback for non-admin contexts.
	return runSchtasks("/Create", "/TN", taskName, "/TR", task, "/SC", "ONLOGON", "/RL", "LIMITED", "/F")
}

func buildTaskCommand(exe string, baseDir string) (string, error) {
	if baseDir == "" {
		task := "\"" + exe + "\" --daemon"
		return task, nil
	}

	runtimeDir := filepath.Join(baseDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", err
	}

	launcherPath := filepath.Join(runtimeDir, "clash-for-claw-task.cmd")
	script := "@echo off\r\n"
	script += "\"" + exe + "\" --daemon --base-dir \"" + baseDir + "\"\r\n"
	if err := os.WriteFile(launcherPath, []byte(script), 0o700); err != nil {
		return "", err
	}

	return "\"" + launcherPath + "\"", nil
}

func uninstallTask() error {
	return uninstallTaskNamed(ServiceName)
}

func uninstallTaskNamed(taskName string) error {
	return runSchtasks("/Delete", "/TN", taskName, "/F")
}

func startTask() error {
	return runSchtasks("/Run", "/TN", ServiceName)
}

func stopTask() error {
	return runSchtasks("/End", "/TN", ServiceName)
}

func runSchtasks(args ...string) error {
	cmd := exec.Command("schtasks", args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, detail)
}

func queryTaskRegistration(taskName string) (bool, service.Status, error) {
	cmd := exec.Command("schtasks", "/Query", "/TN", taskName)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, service.StatusUnknown, nil
	}

	if exitCodeOf(err) == 1 {
		return false, service.StatusUnknown, nil
	}

	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return false, service.StatusUnknown, err
	}
	return false, service.StatusUnknown, fmt.Errorf("%w: %s", err, detail)
}

func queryServiceRegistration(name string) (bool, service.Status, error) {
	cmd := exec.Command("sc.exe", "query", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitCodeOf(err) == 1060 {
			return false, service.StatusUnknown, nil
		}

		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return false, service.StatusUnknown, err
		}
		return false, service.StatusUnknown, fmt.Errorf("%w: %s", err, detail)
	}

	return true, parseWindowsServiceStatus(string(output)), nil
}

func parseWindowsServiceStatus(output string) service.Status {
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(strings.ToUpper(line), "STATE") {
			continue
		}

		separator := strings.Index(line, ":")
		if separator < 0 || separator == len(line)-1 {
			continue
		}

		fields := strings.Fields(line[separator+1:])
		if len(fields) == 0 {
			continue
		}

		code, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		switch code {
		case 1:
			return service.StatusStopped
		case 4:
			return service.StatusRunning
		default:
			return service.StatusUnknown
		}
	}

	text := strings.ToLower(output)
	switch {
	case strings.Contains(text, "running"):
		return service.StatusRunning
	case strings.Contains(text, "stopped"):
		return service.StatusStopped
	default:
		return service.StatusUnknown
	}
}

func exitCodeOf(err error) int {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return -1
	}
	return exitErr.ExitCode()
}
