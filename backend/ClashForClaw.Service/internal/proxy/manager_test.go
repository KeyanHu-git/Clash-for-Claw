package proxy

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/mihomo"
	"clash-for-claw-service/internal/runtime"
)

type fakeMihomoRuntime struct {
	mu         sync.Mutex
	applyCalls int
	plan       []fakeApplyStep
	active     bool
	runtimeErr string
}

type fakeApplyStep struct {
	err        error
	active     bool
	runtimeErr string
}

func (f *fakeMihomoRuntime) Apply(*config.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	step := fakeApplyStep{active: true}
	if f.applyCalls < len(f.plan) {
		step = f.plan[f.applyCalls]
	}
	f.applyCalls++
	f.active = step.active
	f.runtimeErr = step.runtimeErr
	return step.err
}

func (f *fakeMihomoRuntime) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = false
	f.runtimeErr = ""
	return nil
}

func (f *fakeMihomoRuntime) RuntimeState() mihomo.RuntimeState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return mihomo.RuntimeState{
		Active: f.active,
		Error:  f.runtimeErr,
	}
}

func (f *fakeMihomoRuntime) ApplyCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.applyCalls
}

func (f *fakeMihomoRuntime) SetRuntime(active bool, runtimeErr string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = active
	f.runtimeErr = runtimeErr
}

func TestApplyFallsBackAndRecoverLoopRestoresSubscriptionMode(t *testing.T) {
	paths := newProxyTestPaths(t)
	fake := &fakeMihomoRuntime{
		plan: []fakeApplyStep{
			{err: errors.New("start_failed"), active: false, runtimeErr: "start_failed"},
			{active: true},
		},
	}

	manager := NewManager(paths)
	manager.newMihomo = func(runtime.Paths) mihomoRuntime { return fake }
	defer manager.Stop()

	status := manager.Apply(newSubscriptionConfig())
	if !status.Fallback {
		t.Fatalf("fallback = %v, want true", status.Fallback)
	}
	if status.EffectiveMode != config.ProxyModeLocalPort {
		t.Fatalf("effective mode = %q, want %q", status.EffectiveMode, config.ProxyModeLocalPort)
	}

	waitForProxyCondition(t, 3*time.Second, func() bool {
		current := manager.Status()
		return current.MihomoActive && !current.Fallback && current.EffectiveMode == config.ProxyModeSubscription
	})

	if fake.ApplyCalls() < 2 {
		t.Fatalf("apply calls = %d, want at least 2", fake.ApplyCalls())
	}
}

func TestStatusDetectsRuntimeExitAndTriggersRecovery(t *testing.T) {
	paths := newProxyTestPaths(t)
	fake := &fakeMihomoRuntime{
		plan: []fakeApplyStep{
			{active: true},
			{active: true},
		},
	}

	manager := NewManager(paths)
	manager.newMihomo = func(runtime.Paths) mihomoRuntime { return fake }
	defer manager.Stop()

	status := manager.Apply(newSubscriptionConfig())
	if !status.MihomoActive {
		t.Fatalf("initial mihomo active = %v, want true", status.MihomoActive)
	}

	fake.SetRuntime(false, "mihomo_process_exited")
	_ = manager.Status()

	waitForProxyCondition(t, 3*time.Second, func() bool {
		current := manager.Status()
		return current.MihomoActive && !current.Fallback && current.MihomoError == ""
	})

	if fake.ApplyCalls() < 2 {
		t.Fatalf("apply calls = %d, want at least 2", fake.ApplyCalls())
	}
}

func newProxyTestPaths(t *testing.T) runtime.Paths {
	t.Helper()

	root := t.TempDir()
	return runtime.Paths{
		BaseDir:    root,
		ConfigPath: filepath.Join(root, "config.json"),
		RuntimeDir: filepath.Join(root, "runtime"),
		BinDir:     filepath.Join(root, "bin"),
		LogsDir:    filepath.Join(root, "logs"),
		MihomoDir:  filepath.Join(root, "mihomo"),
	}
}

func newSubscriptionConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Proxy.Mode = config.ProxyModeSubscription
	cfg.Proxy.LocalPort = 7890
	cfg.Proxy.SubscriptionURL = "https://example.com/sub"
	cfg.Proxy.Mihomo.MixedPort = 7890
	cfg.Proxy.Mihomo.HTTPPort = 7891
	cfg.Proxy.Mihomo.SocksPort = 7892
	cfg.Proxy.Mihomo.ControllerPort = 9090
	return cfg
}

func waitForProxyCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("timed out waiting for proxy condition")
}
