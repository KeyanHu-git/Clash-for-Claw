//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/winproc"
)

func TestEnsureServicePortFreeStopsMatchingOwner(t *testing.T) {
	port := 13131
	helperExe := buildPortOwnerHelper(t)
	cmd := startPortOwnerHelper(t, helperExe, port)

	cfgPath := writeServicePortConfig(t, port)
	if err := ensureServicePortFree(helperExe, cfgPath); err != nil {
		t.Fatalf("ensureServicePortFree returned error: %v", err)
	}

	waitForProcessExit(t, cmd.Process.Pid, 3*time.Second)
	owners, err := winproc.ListeningPortOwners([]int{port})
	if err != nil {
		t.Fatalf("listeningPortOwners failed: %v", err)
	}
	if _, ok := owners[port]; ok {
		t.Fatalf("port %d still owned after cleanup", port)
	}
}

func TestEnsureServicePortFreeStopsMatchingOwnerFromDifferentPath(t *testing.T) {
	port := 13133
	ownerExe := buildPortOwnerHelperNamed(t, filepath.Join(t.TempDir(), "owner", "ClashForClaw.Service.exe"))
	cmd := startPortOwnerHelper(t, ownerExe, port)

	expectedExe := buildPortOwnerHelperNamed(t, filepath.Join(t.TempDir(), "expected", "ClashForClaw.Service.exe"))
	cfgPath := writeServicePortConfig(t, port)
	if err := ensureServicePortFree(expectedExe, cfgPath); err != nil {
		t.Fatalf("ensureServicePortFree returned error for same-name different-path owner: %v", err)
	}

	waitForProcessExit(t, cmd.Process.Pid, 3*time.Second)
}

func TestEnsureServicePortFreeKeepsForeignOwner(t *testing.T) {
	port := 13132
	cmd := startPortOwnerHelper(t, buildPortOwnerHelper(t), port)
	cfgPath := writeServicePortConfig(t, port)

	err := ensureServicePortFree(filepath.Join(t.TempDir(), "foreign-owner.exe"), cfgPath)
	if err == nil {
		t.Fatal("ensureServicePortFree succeeded for a foreign owner")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("service_port_%d_in_use_by_", port)) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !winproc.IsProcessRunning(cmd.Process.Pid) {
		t.Fatalf("foreign owner process %d was terminated unexpectedly", cmd.Process.Pid)
	}
}

func buildPortOwnerHelper(t *testing.T) string {
	t.Helper()
	return buildPortOwnerHelperNamed(t, filepath.Join(t.TempDir(), "helper.exe"))
}

func buildPortOwnerHelperNamed(t *testing.T, helperExe string) string {
	t.Helper()

	dir := filepath.Dir(helperExe)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll failed: %v", err)
	}

	sourcePath := filepath.Join(dir, "main.go")
	source := `package main

import (
	"net"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+os.Args[1])
	if err != nil {
		os.Exit(3)
	}
	defer ln.Close()
	for {
		time.Sleep(time.Second)
	}
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatalf("os.WriteFile failed: %v", err)
	}

	build := exec.Command("go", "build", "-o", helperExe, sourcePath)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("helper build failed: %v\n%s", err, string(output))
	}

	return helperExe
}

func startPortOwnerHelper(t *testing.T, helperExe string, port int) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(helperExe, fmt.Sprintf("%d", port))
	if err := cmd.Start(); err != nil {
		t.Fatalf("helper start failed: %v", err)
	}

	t.Cleanup(func() {
		if cmd.Process != nil && winproc.IsProcessRunning(cmd.Process.Pid) {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		owners, err := winproc.ListeningPortOwners([]int{port})
		if err == nil {
			if pid, ok := owners[port]; ok && pid > 0 {
				return cmd
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("helper did not bind to port %d", port)
	return nil
}

func writeServicePortConfig(t *testing.T, port int) string {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.HTTP.Bind = "127.0.0.1"
	cfg.HTTP.Port = port
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("config.Save failed: %v", err)
	}
	return cfgPath
}

func waitForProcessExit(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !winproc.IsProcessRunning(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("process %d did not exit in time", pid)
}
