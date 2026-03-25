package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"clash-for-claw-service/internal/config"
)

func TestSyncOperationalConfigCopiesNewerConfigAndSubscriptionFiles(t *testing.T) {
	source := newRuntimeTestPaths(t, "source")
	target := newRuntimeTestPaths(t, "target")

	sourceSubscriptionDir := filepath.Join(source.RuntimeDir, "subscriptions")
	if err := os.MkdirAll(sourceSubscriptionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	sourceFile := filepath.Join(sourceSubscriptionDir, "provider.yaml")
	if err := os.WriteFile(sourceFile, []byte("proxies: []"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Proxy.Subscriptions = []config.Subscription{
		{
			ID:       "sub-1",
			Name:     "imported",
			Source:   "file",
			FilePath: sourceFile,
		},
	}
	cfg.Proxy.ActiveSubscriptionId = "sub-1"

	if err := config.Save(source.ConfigPath, cfg); err != nil {
		t.Fatalf("config.Save failed: %v", err)
	}

	if err := SyncOperationalConfig(source, target); err != nil {
		t.Fatalf("SyncOperationalConfig returned error: %v", err)
	}

	synced, err := config.LoadOrInit(target.ConfigPath)
	if err != nil {
		t.Fatalf("config.LoadOrInit failed: %v", err)
	}

	if got := synced.Proxy.ActiveSubscriptionId; got != "sub-1" {
		t.Fatalf("active subscription = %q, want %q", got, "sub-1")
	}
	if len(synced.Proxy.Subscriptions) != 1 {
		t.Fatalf("subscription count = %d, want 1", len(synced.Proxy.Subscriptions))
	}

	copiedPath := synced.Proxy.Subscriptions[0].FilePath
	if copiedPath == sourceFile {
		t.Fatal("subscription file path was not rewritten for target runtime")
	}
	if filepath.Dir(copiedPath) != filepath.Join(target.RuntimeDir, "subscriptions") {
		t.Fatalf("copied path dir = %q, want target subscriptions dir", filepath.Dir(copiedPath))
	}

	data, err := os.ReadFile(copiedPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "proxies: []" {
		t.Fatalf("copied file contents = %q, want %q", string(data), "proxies: []")
	}
}

func TestSyncOperationalConfigSkipsWhenTargetIsNewer(t *testing.T) {
	source := newRuntimeTestPaths(t, "source")
	target := newRuntimeTestPaths(t, "target")

	sourceCfg := config.DefaultConfig()
	sourceCfg.Proxy.SubscriptionURL = "https://source.example/sub"
	if err := config.Save(source.ConfigPath, sourceCfg); err != nil {
		t.Fatalf("config.Save source failed: %v", err)
	}

	targetCfg := config.DefaultConfig()
	targetCfg.Proxy.SubscriptionURL = "https://target.example/sub"
	if err := config.Save(target.ConfigPath, targetCfg); err != nil {
		t.Fatalf("config.Save target failed: %v", err)
	}

	now := time.Now()
	if err := os.Chtimes(source.ConfigPath, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("Chtimes source failed: %v", err)
	}
	if err := os.Chtimes(target.ConfigPath, now, now); err != nil {
		t.Fatalf("Chtimes target failed: %v", err)
	}

	if err := SyncOperationalConfig(source, target); err != nil {
		t.Fatalf("SyncOperationalConfig returned error: %v", err)
	}

	loaded, err := config.LoadOrInit(target.ConfigPath)
	if err != nil {
		t.Fatalf("config.LoadOrInit failed: %v", err)
	}
	if got := loaded.Proxy.SubscriptionURL; got != "https://target.example/sub" {
		t.Fatalf("target subscription url = %q, want %q", got, "https://target.example/sub")
	}
}

func TestSyncOperationalConfigCopiesWhenTimestampsMatch(t *testing.T) {
	source := newRuntimeTestPaths(t, "source")
	target := newRuntimeTestPaths(t, "target")

	sourceCfg := config.DefaultConfig()
	sourceCfg.Proxy.SubscriptionURL = "https://source.example/sub"
	sourceCfg.Proxy.ActiveSubscriptionId = "source"
	if err := config.Save(source.ConfigPath, sourceCfg); err != nil {
		t.Fatalf("config.Save source failed: %v", err)
	}

	targetCfg := config.DefaultConfig()
	targetCfg.Proxy.SubscriptionURL = "https://target.example/sub"
	targetCfg.Proxy.ActiveSubscriptionId = "target"
	if err := config.Save(target.ConfigPath, targetCfg); err != nil {
		t.Fatalf("config.Save target failed: %v", err)
	}

	now := time.Now()
	if err := os.Chtimes(source.ConfigPath, now, now); err != nil {
		t.Fatalf("Chtimes source failed: %v", err)
	}
	if err := os.Chtimes(target.ConfigPath, now, now); err != nil {
		t.Fatalf("Chtimes target failed: %v", err)
	}

	if err := SyncOperationalConfig(source, target); err != nil {
		t.Fatalf("SyncOperationalConfig returned error: %v", err)
	}

	loaded, err := config.LoadOrInit(target.ConfigPath)
	if err != nil {
		t.Fatalf("config.LoadOrInit failed: %v", err)
	}
	if got := loaded.Proxy.SubscriptionURL; got != "https://source.example/sub" {
		t.Fatalf("target subscription url = %q, want %q", got, "https://source.example/sub")
	}
	if got := loaded.Proxy.ActiveSubscriptionId; got != "source" {
		t.Fatalf("active subscription = %q, want %q", got, "source")
	}
}

func newRuntimeTestPaths(t *testing.T, name string) Paths {
	t.Helper()

	root := filepath.Join(t.TempDir(), name)
	return Paths{
		BaseDir:    root,
		ConfigPath: filepath.Join(root, "config.json"),
		RuntimeDir: filepath.Join(root, "runtime"),
		BinDir:     filepath.Join(root, "bin"),
		LogsDir:    filepath.Join(root, "logs"),
		MihomoDir:  filepath.Join(root, "mihomo"),
	}
}
