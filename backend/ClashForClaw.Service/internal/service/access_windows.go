//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"

	"clash-for-claw-service/internal/runtime"
)

func ensureServiceAccess(paths runtime.Paths) error {
	dirs := []string{
		paths.BaseDir,
		paths.RuntimeDir,
		paths.BinDir,
		paths.LogsDir,
		paths.MihomoDir,
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// LocalService uses SID S-1-5-19; grant modify once at the service root so
	// the service can rotate logs, update config, and manage the embedded runtime.
	cmd := exec.Command("icacls", paths.BaseDir, "/grant", "*S-1-5-19:(OI)(CI)M", "/T", "/C")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("grant_localservice_access_failed: %w (%s)", err, string(output))
	}
	return nil
}

