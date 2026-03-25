//go:build windows

package service

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"clash-for-claw-service/internal/runtime"
)

func TestEnsureServiceAccessGrantsLocalServiceAndUsers(t *testing.T) {
	tempDir := t.TempDir()
	argsPath := filepath.Join(tempDir, "icacls-args.txt")
	withFakeIcacls(t, tempDir, 0, "", argsPath)

	paths := runtime.Paths{
		BaseDir:    filepath.Join(tempDir, "base"),
		ConfigPath: filepath.Join(tempDir, "base", "config.json"),
		RuntimeDir: filepath.Join(tempDir, "base", "runtime"),
		BinDir:     filepath.Join(tempDir, "base", "bin"),
		LogsDir:    filepath.Join(tempDir, "base", "logs"),
		MihomoDir:  filepath.Join(tempDir, "base", "mihomo"),
	}

	if err := ensureServiceAccess(paths); err != nil {
		t.Fatalf("ensureServiceAccess returned error: %v", err)
	}

	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", argsPath, err)
	}
	args := string(data)
	for _, token := range []string{"*S-1-5-19:(OI)(CI)M", "*S-1-5-32-545:(OI)(CI)M", "/T", "/C"} {
		if !strings.Contains(args, token) {
			t.Fatalf("icacls args %q missing token %q", args, token)
		}
	}
}

func TestEnsureServiceAccessReturnsIcaclsOutputOnFailure(t *testing.T) {
	tempDir := t.TempDir()
	argsPath := filepath.Join(tempDir, "icacls-args.txt")
	withFakeIcacls(t, tempDir, 5, "Access is denied.", argsPath)

	paths := runtime.Paths{
		BaseDir:    filepath.Join(tempDir, "base"),
		ConfigPath: filepath.Join(tempDir, "base", "config.json"),
		RuntimeDir: filepath.Join(tempDir, "base", "runtime"),
		BinDir:     filepath.Join(tempDir, "base", "bin"),
		LogsDir:    filepath.Join(tempDir, "base", "logs"),
		MihomoDir:  filepath.Join(tempDir, "base", "mihomo"),
	}

	err := ensureServiceAccess(paths)
	if err == nil {
		t.Fatal("ensureServiceAccess returned nil, want error")
	}
	if !strings.Contains(err.Error(), "Access is denied.") {
		t.Fatalf("ensureServiceAccess error = %q, want icacls output", err.Error())
	}
}

func withFakeIcacls(t *testing.T, dir string, exitCode int, output string, argsPath string) {
	t.Helper()

	scriptPath := filepath.Join(dir, "icacls.cmd")
	script := "@echo off\r\n" +
		"setlocal\r\n" +
		"echo %*>" + argsPath + "\r\n"
	if output != "" {
		script += "echo " + output + "\r\n"
	}
	script += "exit /b " + strconv.Itoa(exitCode) + "\r\n"

	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", scriptPath, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
