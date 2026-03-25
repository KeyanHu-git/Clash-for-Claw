package subscription

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/gateway"
	"clash-for-claw-service/internal/proxy"
	"clash-for-claw-service/internal/runtime"
)

type fakeProxyManager struct {
	current string
	mode    string
	calls   int
	onApply func(*config.Config)
}

func (m *fakeProxyManager) Apply(cfg *config.Config) proxy.Status {
	if cfg != nil {
		m.calls++
		m.mode = cfg.Proxy.Mode
		if cfg.Proxy.Mode == config.ProxyModeLocalPort && cfg.Proxy.LocalPort > 0 {
			m.current = fmt.Sprintf("http://127.0.0.1:%d", cfg.Proxy.LocalPort)
		} else {
			m.current = cfg.Proxy.SubscriptionURL
		}
		if m.onApply != nil {
			m.onApply(cfg)
		}
	}
	return proxy.Status{
		Mode:          cfg.Proxy.Mode,
		EffectiveMode: cfg.Proxy.Mode,
		ProxyURL:      m.current,
	}
}

func (m *fakeProxyManager) ProxyURL() string {
	return m.current
}

func (m *fakeProxyManager) ApplyCount() int {
	return m.calls
}

func TestReconcilePreferredSubscriptionPromotesLowerLatencyHealthySubscription(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)
	manager.probeCandidate = func(_ context.Context, _ runtime.Paths, _ *config.Config, sub config.Subscription, _ string) gateway.ProbeResult {
		switch sub.URL {
		case subA:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 140, InternetLatencyMs: 120}
		case subB:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 30, InternetLatencyMs: 35}
		default:
			return gateway.ProbeResult{}
		}
	}

	if err := manager.ReconcilePreferredSubscription(); err != nil {
		t.Fatalf("ReconcilePreferredSubscription returned error: %v", err)
	}

	if got := cfg.Proxy.ActiveSubscriptionId; got != cfg.Proxy.Subscriptions[1].ID {
		t.Fatalf("active subscription = %q, want %q", got, cfg.Proxy.Subscriptions[1].ID)
	}
	if got := cfg.Proxy.SubscriptionURL; got != subB {
		t.Fatalf("subscription url = %q, want %q", got, subB)
	}
	if got := proxyMgr.ApplyCount(); got != 1 {
		t.Fatalf("apply count = %d, want 1 final switch", got)
	}
}

func TestReconcilePreferredSubscriptionKeepsCurrentWithinStickinessThreshold(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)
	manager.probeCandidate = func(_ context.Context, _ runtime.Paths, _ *config.Config, sub config.Subscription, _ string) gateway.ProbeResult {
		switch sub.URL {
		case subA:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 90, InternetLatencyMs: 90}
		case subB:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 40, InternetLatencyMs: 40}
		default:
			return gateway.ProbeResult{}
		}
	}

	if err := manager.ReconcilePreferredSubscription(); err != nil {
		t.Fatalf("ReconcilePreferredSubscription returned error: %v", err)
	}

	if got := cfg.Proxy.ActiveSubscriptionId; got != cfg.Proxy.Subscriptions[0].ID {
		t.Fatalf("active subscription = %q, want current %q", got, cfg.Proxy.Subscriptions[0].ID)
	}
	if got := proxyMgr.ApplyCount(); got != 0 {
		t.Fatalf("apply count = %d, want 0 when current subscription remains preferred", got)
	}
}

func TestProbeAndFailoverSwitchesWhenCurrentSubscriptionIsUnhealthy(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)
	manager.probeCandidate = func(_ context.Context, _ runtime.Paths, _ *config.Config, sub config.Subscription, _ string) gateway.ProbeResult {
		switch sub.URL {
		case subA:
			return gateway.ProbeResult{GatewayOK: false, InternetOK: false}
		case subB:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 45, InternetLatencyMs: 55}
		default:
			return gateway.ProbeResult{}
		}
	}

	if err := manager.ProbeAndFailover(); err != nil {
		t.Fatalf("ProbeAndFailover returned error: %v", err)
	}

	if got := cfg.Proxy.ActiveSubscriptionId; got != cfg.Proxy.Subscriptions[1].ID {
		t.Fatalf("active subscription = %q, want %q", got, cfg.Proxy.Subscriptions[1].ID)
	}
	if got := proxyMgr.ApplyCount(); got != 1 {
		t.Fatalf("apply count = %d, want 1 final switch", got)
	}
}

func TestProbeAndFailoverAppliesOnlyAfterAllShadowProbesComplete(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	subC := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB, subC)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	events := make([]string, 0, 4)
	proxyMgr := &fakeProxyManager{
		current: subA,
		onApply: func(cfg *config.Config) {
			events = append(events, "apply:"+cfg.Proxy.SubscriptionURL)
		},
	}
	manager := NewManager(paths, cfg, proxyMgr)
	manager.probeCandidate = func(_ context.Context, _ runtime.Paths, _ *config.Config, sub config.Subscription, _ string) gateway.ProbeResult {
		events = append(events, "probe:"+sub.URL)
		switch sub.URL {
		case subA:
			return gateway.ProbeResult{GatewayOK: false, InternetOK: false}
		case subB:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 80, InternetLatencyMs: 90}
		case subC:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 20, InternetLatencyMs: 25}
		default:
			return gateway.ProbeResult{}
		}
	}

	if err := manager.ProbeAndFailover(); err != nil {
		t.Fatalf("ProbeAndFailover returned error: %v", err)
	}

	if got := proxyMgr.ApplyCount(); got != 1 {
		t.Fatalf("apply count = %d, want 1 final switch", got)
	}
	if len(events) != 4 {
		t.Fatalf("events = %v, want 4 entries", events)
	}
	for i := 0; i < 3; i++ {
		if events[i] != "probe:"+cfg.Proxy.Subscriptions[i].URL {
			t.Fatalf("events[%d] = %q, want probe for subscription %q", i, events[i], cfg.Proxy.Subscriptions[i].URL)
		}
	}
	if events[3] != "apply:"+subC {
		t.Fatalf("final event = %q, want %q", events[3], "apply:"+subC)
	}
}

func TestReconcilePreferredSubscriptionDoesNotOverrideManualActivationDuringProbe(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	subC := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB, subC)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	firstProbeStarted := make(chan struct{}, 1)
	releaseProbe := make(chan struct{})

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)
	manager.probeCandidate = func(_ context.Context, _ runtime.Paths, _ *config.Config, sub config.Subscription, _ string) gateway.ProbeResult {
		if sub.ID == cfg.Proxy.Subscriptions[0].ID {
			select {
			case firstProbeStarted <- struct{}{}:
			default:
			}
			<-releaseProbe
		}

		switch sub.URL {
		case subA:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 120, InternetLatencyMs: 120}
		case subB:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 25, InternetLatencyMs: 25}
		case subC:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 15, InternetLatencyMs: 15}
		default:
			return gateway.ProbeResult{}
		}
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- manager.ReconcilePreferredSubscription()
	}()

	select {
	case <-firstProbeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected first shadow probe to start")
	}

	activated, err := manager.Activate(cfg.Proxy.Subscriptions[1].ID)
	if err != nil {
		t.Fatalf("Activate returned error: %v", err)
	}
	if activated.ID != cfg.Proxy.Subscriptions[1].ID {
		t.Fatalf("activated id = %q, want %q", activated.ID, cfg.Proxy.Subscriptions[1].ID)
	}

	close(releaseProbe)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ReconcilePreferredSubscription returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reconcile to finish")
	}

	if got := cfg.Proxy.ActiveSubscriptionId; got != cfg.Proxy.Subscriptions[1].ID {
		t.Fatalf("active subscription = %q, want manual selection %q", got, cfg.Proxy.Subscriptions[1].ID)
	}
	if got := cfg.Proxy.SubscriptionURL; got != subB {
		t.Fatalf("subscription url = %q, want manual url %q", got, subB)
	}
	if got := proxyMgr.ApplyCount(); got != 1 {
		t.Fatalf("apply count = %d, want 1 manual apply only", got)
	}
}

func TestScheduledMaintenanceReconcilesHealthyMultiSubscriptionRuntime(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)
	manager.probeCandidate = func(_ context.Context, _ runtime.Paths, _ *config.Config, sub config.Subscription, _ string) gateway.ProbeResult {
		switch sub.URL {
		case subA:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 110, InternetLatencyMs: 100}
		case subB:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 30, InternetLatencyMs: 25}
		default:
			return gateway.ProbeResult{}
		}
	}

	if err := manager.scheduledMaintenance(); err != nil {
		t.Fatalf("scheduledMaintenance returned error: %v", err)
	}

	if got := cfg.Proxy.ActiveSubscriptionId; got != cfg.Proxy.Subscriptions[1].ID {
		t.Fatalf("active subscription = %q, want %q", got, cfg.Proxy.Subscriptions[1].ID)
	}
	if got := proxyMgr.ApplyCount(); got != 1 {
		t.Fatalf("apply count = %d, want 1 final switch", got)
	}
}

func TestStartTriggersImmediateReconcile(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)
	called := make(chan struct{}, 1)
	manager.probeCandidate = func(context.Context, runtime.Paths, *config.Config, config.Subscription, string) gateway.ProbeResult {
		select {
		case called <- struct{}{}:
		default:
		}
		return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 20, InternetLatencyMs: 20}
	}

	manager.Start()
	defer manager.Stop()

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("expected startup reconcile probe to run")
	}
}

func TestReconcilePreferredSubscriptionKeepsCurrentWhenAllCandidatesFailWithoutHistory(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)
	manager.probeCandidate = func(context.Context, runtime.Paths, *config.Config, config.Subscription, string) gateway.ProbeResult {
		return gateway.ProbeResult{GatewayOK: false, InternetOK: false}
	}

	if err := manager.ReconcilePreferredSubscription(); err != nil {
		t.Fatalf("ReconcilePreferredSubscription returned error: %v", err)
	}

	if got := cfg.Proxy.ActiveSubscriptionId; got != cfg.Proxy.Subscriptions[0].ID {
		t.Fatalf("active subscription = %q, want current %q", got, cfg.Proxy.Subscriptions[0].ID)
	}
	if got := cfg.Proxy.SubscriptionURL; got != subA {
		t.Fatalf("subscription url = %q, want current %q", got, subA)
	}
	if got := proxyMgr.ApplyCount(); got != 0 {
		t.Fatalf("apply count = %d, want 0 when all candidates fail and current is retained", got)
	}
}

func TestDeleteActiveSubscriptionFallsBackToLocalModeWhenLastSubscriptionIsRemoved(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA
	cfg.Proxy.Mode = config.ProxyModeSubscription

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)

	active, err := manager.Delete(cfg.Proxy.Subscriptions[0].ID)
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	if active != "" {
		t.Fatalf("active = %q, want empty", active)
	}
	if got := cfg.Proxy.ActiveSubscriptionId; got != "" {
		t.Fatalf("active subscription = %q, want empty", got)
	}
	if got := cfg.Proxy.SubscriptionURL; got != "" {
		t.Fatalf("subscription url = %q, want empty", got)
	}
	if got := cfg.Proxy.Mode; got != config.ProxyModeLocalPort {
		t.Fatalf("mode = %q, want %q", got, config.ProxyModeLocalPort)
	}
	if got := proxyMgr.mode; got != config.ProxyModeLocalPort {
		t.Fatalf("applied mode = %q, want %q", got, config.ProxyModeLocalPort)
	}
}

func TestRefreshAllMarksUsageUnknownWhenUserinfoHeaderIsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	cfg, paths := newTestConfig(t, server.URL)
	cfg.Proxy.Subscriptions[0].UsageUsed = 12.5
	cfg.Proxy.Subscriptions[0].UsageLimit = 80
	cfg.Proxy.Subscriptions[0].UsageUnit = "GB"

	manager := NewManager(paths, cfg, &fakeProxyManager{})
	subs, err := manager.RefreshAll()
	if err != nil {
		t.Fatalf("RefreshAll returned error: %v", err)
	}

	if got := subs[0].State; got != StateUnknown {
		t.Fatalf("state = %q, want %q", got, StateUnknown)
	}
	if got := subs[0].UsageUsed; got != 12.5 {
		t.Fatalf("usage used = %v, want preserved value %v", got, 12.5)
	}
	if got := subs[0].UsageLimit; got != 80 {
		t.Fatalf("usage limit = %v, want preserved value %v", got, 80.0)
	}
}

func TestStopClearsLoopStateSoManagerCanRestart(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	manager := NewManager(paths, cfg, &fakeProxyManager{current: subA})
	manager.probeCandidate = func(context.Context, runtime.Paths, *config.Config, config.Subscription, string) gateway.ProbeResult {
		return gateway.ProbeResult{GatewayOK: true, InternetOK: true}
	}

	manager.Start()
	manager.Stop()

	if manager.billingUpdates != nil {
		t.Fatal("billingUpdates was not cleared on Stop")
	}
	if manager.probeUpdates != nil {
		t.Fatal("probeUpdates was not cleared on Stop")
	}

	manager.Start()
	defer manager.Stop()

	if manager.billingUpdates == nil {
		t.Fatal("billingUpdates was not recreated on restart")
	}
	if manager.probeUpdates == nil {
		t.Fatal("probeUpdates was not recreated on restart")
	}
}

func TestIntervalsFromConfigUsesFasterProbeForMultiSubscriptionDefault(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB)
	cfg.Proxy.SubscriptionProbeMinutes = 60

	manager := NewManager(paths, cfg, &fakeProxyManager{})
	_, probe := manager.intervalsFromConfigLocked()

	if probe != 5*time.Minute {
		t.Fatalf("probe interval = %s, want %s", probe, 5*time.Minute)
	}
}

func TestScheduledMaintenanceSoakKeepsCurrentStableUnderLatencyJitter(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)
	iteration := 0
	manager.probeCandidate = func(_ context.Context, _ runtime.Paths, _ *config.Config, sub config.Subscription, _ string) gateway.ProbeResult {
		iteration++
		jitter := int64(iteration % 7)
		switch sub.URL {
		case subA:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 92 + jitter, InternetLatencyMs: 90 + jitter}
		case subB:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 28 + jitter, InternetLatencyMs: 26 + jitter}
		default:
			return gateway.ProbeResult{}
		}
	}

	for i := 0; i < 500; i++ {
		if err := manager.scheduledMaintenance(); err != nil {
			t.Fatalf("scheduledMaintenance iteration %d returned error: %v", i, err)
		}
	}

	if got := cfg.Proxy.ActiveSubscriptionId; got != cfg.Proxy.Subscriptions[0].ID {
		t.Fatalf("active subscription = %q, want stable current %q", got, cfg.Proxy.Subscriptions[0].ID)
	}
	if got := cfg.Proxy.SubscriptionURL; got != subA {
		t.Fatalf("subscription url = %q, want stable current %q", got, subA)
	}
	if got := proxyMgr.ApplyCount(); got != 0 {
		t.Fatalf("apply count = %d, want 0 under stable jitter soak", got)
	}
}

func TestReconcilePreferredSubscriptionSerializesConcurrentFailovers(t *testing.T) {
	subA := newSubscriptionEndpoint(t)
	subB := newSubscriptionEndpoint(t)
	cfg, paths := newTestConfig(t, subA, subB)
	cfg.Proxy.ActiveSubscriptionId = cfg.Proxy.Subscriptions[0].ID
	cfg.Proxy.SubscriptionURL = subA

	proxyMgr := &fakeProxyManager{current: subA}
	manager := NewManager(paths, cfg, proxyMgr)

	var inFlight int32
	var maxInFlight int32
	manager.probeCandidate = func(_ context.Context, _ runtime.Paths, _ *config.Config, sub config.Subscription, _ string) gateway.ProbeResult {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			seen := atomic.LoadInt32(&maxInFlight)
			if current <= seen || atomic.CompareAndSwapInt32(&maxInFlight, seen, current) {
				break
			}
		}

		time.Sleep(25 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)

		switch sub.URL {
		case subA:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 80, InternetLatencyMs: 80}
		case subB:
			return gateway.ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 30, InternetLatencyMs: 30}
		default:
			return gateway.ProbeResult{}
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- manager.ReconcilePreferredSubscription()
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("ReconcilePreferredSubscription returned error: %v", err)
		}
	}
	if got := atomic.LoadInt32(&maxInFlight); got != 1 {
		t.Fatalf("max concurrent shadow probes = %d, want 1", got)
	}
}

func newSubscriptionEndpoint(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("subscription-userinfo", "upload=1000; download=2000; total=1000000000; expire=2000000000")
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func newTestConfig(t *testing.T, urls ...string) (*config.Config, runtime.Paths) {
	t.Helper()

	root := t.TempDir()
	paths := runtime.Paths{
		BaseDir:    root,
		ConfigPath: filepath.Join(root, "config.json"),
		RuntimeDir: filepath.Join(root, "runtime"),
		BinDir:     filepath.Join(root, "bin"),
		LogsDir:    filepath.Join(root, "logs"),
		MihomoDir:  filepath.Join(root, "mihomo"),
	}

	cfg := config.DefaultConfig()
	cfg.Gateway.URL = "ws://gateway.test/ws"
	cfg.Proxy.Mode = config.ProxyModeSubscription
	cfg.Proxy.Subscriptions = nil
	cfg.Proxy.ActiveSubscriptionId = ""
	cfg.Proxy.SubscriptionURL = ""

	for i, rawURL := range urls {
		sub := config.Subscription{
			ID:        "sub-" + string(rune('A'+i)),
			Name:      "sub",
			Source:    SourceURL,
			URL:       rawURL,
			State:     StateUnknown,
			UsageUnit: "GB",
		}
		cfg.Proxy.Subscriptions = append(cfg.Proxy.Subscriptions, sub)
	}

	if err := config.Save(paths.ConfigPath, cfg); err != nil {
		t.Fatalf("config.Save failed: %v", err)
	}

	return cfg, paths
}
