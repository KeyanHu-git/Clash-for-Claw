package app

import (
	"os"
	"path/filepath"
	"testing"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/runtime"
)

func TestResolveServiceCommandPathsKeepsUserAndServiceBasesSeparate(t *testing.T) {
	root := t.TempDir()
	appData := filepath.Join(root, "appdata")
	programData := filepath.Join(root, "programdata")
	serviceBase := filepath.Join(programData, runtime.AppFolderName)

	t.Setenv("APPDATA", appData)
	t.Setenv("ProgramData", programData)

	userPaths, servicePaths, err := resolveServiceCommandPaths(serviceBase)
	if err != nil {
		t.Fatalf("resolveServiceCommandPaths returned error: %v", err)
	}

	wantUserBase := filepath.Join(appData, runtime.AppFolderName)
	if userPaths.BaseDir != wantUserBase {
		t.Fatalf("user base = %q, want %q", userPaths.BaseDir, wantUserBase)
	}
	if servicePaths.BaseDir != serviceBase {
		t.Fatalf("service base = %q, want %q", servicePaths.BaseDir, serviceBase)
	}
	if runtime.SameBaseDir(userPaths.BaseDir, servicePaths.BaseDir) {
		t.Fatal("user and service bases unexpectedly resolved to the same directory")
	}
}

func TestPrepareOperationalConfigSyncsDefaultSharedDaemonBase(t *testing.T) {
	root := t.TempDir()
	appData := filepath.Join(root, "appdata")
	programData := filepath.Join(root, "programdata")
	t.Setenv("APPDATA", appData)
	t.Setenv("ProgramData", programData)

	userPaths, err := runtime.ResolvePaths("")
	if err != nil {
		t.Fatalf("ResolvePaths(user) failed: %v", err)
	}
	servicePaths, err := runtime.ResolveServicePaths("")
	if err != nil {
		t.Fatalf("ResolveServicePaths(service) failed: %v", err)
	}

	writeConfigForTest(t, servicePaths.ConfigPath, "https://stale.example/sub", "sub-stale")
	writeConfigForTest(t, userPaths.ConfigPath, "https://source.example/sub", "sub-source")

	if err := prepareOperationalConfig(runModeDaemon, servicePaths); err != nil {
		t.Fatalf("prepareOperationalConfig returned error: %v", err)
	}

	got, err := config.LoadOrInit(servicePaths.ConfigPath)
	if err != nil {
		t.Fatalf("LoadOrInit(service) failed: %v", err)
	}
	if got.Proxy.SubscriptionURL != "https://source.example/sub" {
		t.Fatalf("service subscription url = %q, want %q", got.Proxy.SubscriptionURL, "https://source.example/sub")
	}
	if got.Proxy.ActiveSubscriptionId != "sub-source" {
		t.Fatalf("service active subscription = %q, want %q", got.Proxy.ActiveSubscriptionId, "sub-source")
	}
}

func TestPrepareOperationalConfigSkipsServiceMode(t *testing.T) {
	root := t.TempDir()
	appData := filepath.Join(root, "appdata")
	programData := filepath.Join(root, "programdata")
	t.Setenv("APPDATA", appData)
	t.Setenv("ProgramData", programData)

	userPaths, err := runtime.ResolvePaths("")
	if err != nil {
		t.Fatalf("ResolvePaths(user) failed: %v", err)
	}
	servicePaths, err := runtime.ResolveServicePaths("")
	if err != nil {
		t.Fatalf("ResolveServicePaths(service) failed: %v", err)
	}

	writeConfigForTest(t, userPaths.ConfigPath, "https://source.example/sub", "sub-source")
	writeConfigForTest(t, servicePaths.ConfigPath, "https://service.example/sub", "sub-service")

	if err := prepareOperationalConfig(runModeService, servicePaths); err != nil {
		t.Fatalf("prepareOperationalConfig returned error: %v", err)
	}

	got, err := config.LoadOrInit(servicePaths.ConfigPath)
	if err != nil {
		t.Fatalf("LoadOrInit(service) failed: %v", err)
	}
	if got.Proxy.SubscriptionURL != "https://service.example/sub" {
		t.Fatalf("service subscription url = %q, want unchanged value %q", got.Proxy.SubscriptionURL, "https://service.example/sub")
	}
}

func TestPrepareOperationalConfigSkipsCustomDaemonBase(t *testing.T) {
	root := t.TempDir()
	appData := filepath.Join(root, "appdata")
	programData := filepath.Join(root, "programdata")
	customBase := filepath.Join(root, "custom-base")
	t.Setenv("APPDATA", appData)
	t.Setenv("ProgramData", programData)

	userPaths, err := runtime.ResolvePaths("")
	if err != nil {
		t.Fatalf("ResolvePaths(user) failed: %v", err)
	}
	customPaths, err := runtime.ResolvePaths(customBase)
	if err != nil {
		t.Fatalf("ResolvePaths(custom) failed: %v", err)
	}

	writeConfigForTest(t, userPaths.ConfigPath, "https://source.example/sub", "sub-source")
	writeConfigForTest(t, customPaths.ConfigPath, "https://custom.example/sub", "sub-custom")

	if err := prepareOperationalConfig(runModeDaemon, customPaths); err != nil {
		t.Fatalf("prepareOperationalConfig returned error: %v", err)
	}

	got, err := config.LoadOrInit(customPaths.ConfigPath)
	if err != nil {
		t.Fatalf("LoadOrInit(custom) failed: %v", err)
	}
	if got.Proxy.SubscriptionURL != "https://custom.example/sub" {
		t.Fatalf("custom subscription url = %q, want unchanged value %q", got.Proxy.SubscriptionURL, "https://custom.example/sub")
	}
}

func TestPrepareOperationalConfigSkipsWhenUserAndSharedBaseMatch(t *testing.T) {
	root := t.TempDir()
	commonRoot := filepath.Join(root, "shared")
	t.Setenv("APPDATA", commonRoot)
	t.Setenv("ProgramData", commonRoot)

	paths, err := runtime.ResolvePaths("")
	if err != nil {
		t.Fatalf("ResolvePaths failed: %v", err)
	}
	writeConfigForTest(t, paths.ConfigPath, "https://shared.example/sub", "sub-shared")

	infoBefore, err := os.Stat(paths.ConfigPath)
	if err != nil {
		t.Fatalf("Stat before failed: %v", err)
	}

	if err := prepareOperationalConfig(runModeDaemon, paths); err != nil {
		t.Fatalf("prepareOperationalConfig returned error: %v", err)
	}

	infoAfter, err := os.Stat(paths.ConfigPath)
	if err != nil {
		t.Fatalf("Stat after failed: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatal("shared config timestamp changed even though user and service base are the same")
	}
}

func writeConfigForTest(t *testing.T, path string, subscriptionURL string, activeID string) {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.Proxy.SubscriptionURL = subscriptionURL
	cfg.Proxy.ActiveSubscriptionId = activeID
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("config.Save(%q) failed: %v", path, err)
	}
}
