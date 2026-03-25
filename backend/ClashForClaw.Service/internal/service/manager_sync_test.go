package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kardianos/service"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/runtime"
)

func TestSyncOperationalConfigCopiesFileSubscriptionsToTargetPaths(t *testing.T) {
	sourcePaths := newSyncTestPaths(t, "source")
	targetPaths := newSyncTestPaths(t, "target")

	sourceFile := filepath.Join(sourcePaths.RuntimeDir, "subscriptions", "source.yaml")
	if err := os.MkdirAll(filepath.Dir(sourceFile), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(sourceFile, []byte("proxy: source"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	targetCfg := config.DefaultConfig()
	targetCfg.Proxy.SubscriptionURL = "https://example.com/stale"
	targetCfg.Proxy.ActiveSubscriptionId = "stale"
	if err := config.Save(targetPaths.ConfigPath, targetCfg); err != nil {
		t.Fatalf("config.Save(target) failed: %v", err)
	}

	sourceCfg := config.DefaultConfig()
	sourceCfg.Proxy.SubscriptionURL = "https://example.com/source"
	sourceCfg.Proxy.ActiveSubscriptionId = "sub-1"
	sourceCfg.Proxy.Subscriptions = []config.Subscription{
		{
			ID:       "sub-1",
			Name:     "source",
			Source:   "file",
			URL:      "https://example.com/source",
			FilePath: sourceFile,
		},
	}
	if err := config.Save(sourcePaths.ConfigPath, sourceCfg); err != nil {
		t.Fatalf("config.Save(source) failed: %v", err)
	}

	if err := runtime.SyncOperationalConfig(sourcePaths, targetPaths); err != nil {
		t.Fatalf("SyncOperationalConfig returned error: %v", err)
	}

	got, err := config.LoadOrInit(targetPaths.ConfigPath)
	if err != nil {
		t.Fatalf("config.LoadOrInit(target) failed: %v", err)
	}

	if got.Proxy.ActiveSubscriptionId != "sub-1" {
		t.Fatalf("active subscription = %q, want %q", got.Proxy.ActiveSubscriptionId, "sub-1")
	}
	if got.Proxy.SubscriptionURL != "https://example.com/source" {
		t.Fatalf("subscription url = %q, want source url", got.Proxy.SubscriptionURL)
	}
	if len(got.Proxy.Subscriptions) != 1 {
		t.Fatalf("subscriptions len = %d, want 1", len(got.Proxy.Subscriptions))
	}
	if got.Proxy.Subscriptions[0].FilePath == sourceFile {
		t.Fatal("subscription file path was not rewritten for target runtime")
	}
	if _, err := os.Stat(got.Proxy.Subscriptions[0].FilePath); err != nil {
		t.Fatalf("target subscription file missing: %v", err)
	}
}

func TestStopSyncsServiceConfigBackToUserPathsWhenServiceAlreadyStopped(t *testing.T) {
	withFakeWindowsTools(t,
		fakeToolConfig{
			Default: fakeServiceRegistered(service.StatusStopped),
		},
		fakeToolConfig{
			Default: fakeTaskMissing(),
		},
	)

	userPaths := newSyncTestPaths(t, "user")
	servicePaths := newSyncTestPaths(t, "service")

	userCfg := config.DefaultConfig()
	userCfg.Proxy.SubscriptionURL = "https://example.com/stale"
	userCfg.Proxy.ActiveSubscriptionId = "stale"
	if err := config.Save(userPaths.ConfigPath, userCfg); err != nil {
		t.Fatalf("config.Save(user) failed: %v", err)
	}

	serviceCfg := config.DefaultConfig()
	serviceCfg.Proxy.SubscriptionURL = "https://example.com/live"
	serviceCfg.Proxy.ActiveSubscriptionId = "live"
	serviceCfg.Proxy.Subscriptions = []config.Subscription{
		{
			ID:     "live",
			Name:   "live",
			Source: "url",
			URL:    "https://example.com/live",
		},
	}
	if err := config.Save(servicePaths.ConfigPath, serviceCfg); err != nil {
		t.Fatalf("config.Save(service) failed: %v", err)
	}

	manager := &Manager{
		userPaths:    userPaths,
		servicePaths: servicePaths,
		exe:          `C:\Program Files\ClashForClaw\ClashForClaw.Service.exe`,
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	got, err := config.LoadOrInit(userPaths.ConfigPath)
	if err != nil {
		t.Fatalf("config.LoadOrInit(user) failed: %v", err)
	}

	if got.Proxy.ActiveSubscriptionId != "live" {
		t.Fatalf("active subscription = %q, want %q", got.Proxy.ActiveSubscriptionId, "live")
	}
	if got.Proxy.SubscriptionURL != "https://example.com/live" {
		t.Fatalf("subscription url = %q, want live url", got.Proxy.SubscriptionURL)
	}
}

func newSyncTestPaths(t *testing.T, name string) runtime.Paths {
	t.Helper()

	root := filepath.Join(t.TempDir(), name)
	return runtime.Paths{
		BaseDir:    root,
		ConfigPath: filepath.Join(root, "config.json"),
		RuntimeDir: filepath.Join(root, "runtime"),
		BinDir:     filepath.Join(root, "bin"),
		LogsDir:    filepath.Join(root, "logs"),
		MihomoDir:  filepath.Join(root, "mihomo"),
	}
}
