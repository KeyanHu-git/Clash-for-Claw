//go:build windows

package systemproxy

import (
	"encoding/json"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"clash-for-claw-service/internal/runtime"
)

type Manager struct {
	paths runtime.Paths
}

type backup struct {
	Enabled int    `json:"enabled"`
	Server  string `json:"server"`
	Bypass  string `json:"bypass"`
}

func NewManager(paths runtime.Paths) *Manager {
	return &Manager{paths: paths}
}

func (m *Manager) Enable(proxyURL string) error {
	prev, _ := readCurrent()
	if shouldRefreshBackup(m.paths.RuntimeDir, prev, proxyURL) {
		_ = writeBackup(m.paths.RuntimeDir, prev)
	}
	return writeProxy(1, proxyURL, prev.Bypass)
}

func (m *Manager) Disable() error {
	if prev, err := readBackup(m.paths.RuntimeDir); err == nil {
		_ = os.Remove(backupPath(m.paths.RuntimeDir))
		if prev.Enabled == 1 || prev.Server != "" || prev.Bypass != "" {
			return writeProxy(prev.Enabled, prev.Server, prev.Bypass)
		}
	}
	return writeProxy(0, "", "")
}

func (m *Manager) Status() (bool, string) {
	cur, err := readCurrent()
	if err != nil {
		return false, ""
	}
	return cur.Enabled == 1, cur.Server
}

func readCurrent() (backup, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return backup{}, err
	}
	defer key.Close()
	enabled, _, _ := key.GetIntegerValue("ProxyEnable")
	server, _, _ := key.GetStringValue("ProxyServer")
	bypass, _, _ := key.GetStringValue("ProxyOverride")
	return backup{Enabled: int(enabled), Server: server, Bypass: bypass}, nil
}

func writeProxy(enabled int, server string, bypass string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetDWordValue("ProxyEnable", uint32(enabled)); err != nil {
		return err
	}
	if server == "" {
		_ = key.DeleteValue("ProxyServer")
	} else {
		if err := key.SetStringValue("ProxyServer", server); err != nil {
			return err
		}
	}
	if bypass != "" {
		_ = key.SetStringValue("ProxyOverride", bypass)
	} else {
		_ = key.DeleteValue("ProxyOverride")
	}
	return nil
}

func shouldRefreshBackup(runtimeDir string, current backup, proxyURL string) bool {
	if _, err := readBackup(runtimeDir); err != nil {
		return true
	}

	return current.Enabled != 1 || current.Server != proxyURL
}

func backupPath(runtimeDir string) string {
	return filepath.Join(runtimeDir, "system_proxy.json")
}

func writeBackup(runtimeDir string, data backup) error {
	raw, _ := json.MarshalIndent(data, "", "  ")
	return os.WriteFile(backupPath(runtimeDir), raw, 0o600)
}

func readBackup(runtimeDir string) (backup, error) {
	raw, err := os.ReadFile(backupPath(runtimeDir))
	if err != nil {
		return backup{}, err
	}
	var out backup
	if err := json.Unmarshal(raw, &out); err != nil {
		return backup{}, err
	}
	return out, nil
}

