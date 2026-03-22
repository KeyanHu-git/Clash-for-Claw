package proxy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/mihomo"
	"clash-for-claw-service/internal/runtime"
)

type Status struct {
	Mode           string `json:"mode"`
	EffectiveMode  string `json:"effective_mode,omitempty"`
	ProxyURL       string `json:"proxy_url"`
	MihomoActive   bool   `json:"mihomo_active"`
	MihomoError    string `json:"mihomo_error,omitempty"`
	Fallback       bool   `json:"fallback,omitempty"`
	FallbackReason string `json:"fallback_reason,omitempty"`
}

type mihomoRuntime interface {
	Apply(*config.Config) error
	Stop() error
	RuntimeState() mihomo.RuntimeState
}

type mihomoFactory func(runtime.Paths) mihomoRuntime

const recoveryInterval = 2 * time.Second

type Manager struct {
	mu               sync.Mutex
	mihomo           mihomoRuntime
	proxyURL         string
	status           Status
	paths            runtime.Paths
	controllerPort   int
	controllerSecret string
	activeCfg        *config.Config
	newMihomo        mihomoFactory
	recoverCh        chan struct{}
	loopCancel       context.CancelFunc
	lastRecoverAt    time.Time
}

func NewManager(paths runtime.Paths) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		paths:      paths,
		newMihomo:  func(paths runtime.Paths) mihomoRuntime { return mihomo.NewManager(paths) },
		recoverCh:  make(chan struct{}, 1),
		loopCancel: cancel,
	}
	go manager.watchRecoverLoop(ctx)
	return manager
}

func (m *Manager) Apply(cfg *config.Config) Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.activeCfg = cloneConfig(cfg)
	status := Status{Mode: cfg.Proxy.Mode, EffectiveMode: cfg.Proxy.Mode}
	m.proxyURL = ""
	m.controllerPort = cfg.Proxy.Mihomo.ControllerPort
	switch cfg.Proxy.Mode {
	case config.ProxyModeSubscription:
		if m.mihomo == nil {
			m.mihomo = m.newMihomo(m.paths)
		}
		if cfg.Proxy.SubscriptionURL == "" {
			status.MihomoError = "subscription_url_required"
			m.applyFallbackLocal(&status, cfg, "subscription_url_missing")
			break
		}
		if err := m.mihomo.Apply(cfg); err != nil {
			status.MihomoError = err.Error()
			m.applyFallbackLocal(&status, cfg, "mihomo_start_failed")
		} else {
			status.ProxyURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.Proxy.Mihomo.MixedPort)
			m.proxyURL = status.ProxyURL
		}
	default:
		if m.mihomo != nil {
			_ = m.mihomo.Stop()
		}
		status.EffectiveMode = config.ProxyModeLocalPort
		if cfg.Proxy.LocalPort > 0 {
			status.ProxyURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.Proxy.LocalPort)
			m.proxyURL = status.ProxyURL
		}
	}

	m.status = status
	current := m.statusLocked()
	if current.Mode == config.ProxyModeSubscription && !current.MihomoActive {
		m.signalRecoverLocked()
	}
	return current
}

func (m *Manager) applyFallbackLocal(status *Status, cfg *config.Config, reason string) {
	if m.mihomo != nil {
		_ = m.mihomo.Stop()
	}
	status.Fallback = true
	status.FallbackReason = reason
	status.EffectiveMode = config.ProxyModeLocalPort
	if cfg.Proxy.LocalPort > 0 {
		status.ProxyURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.Proxy.LocalPort)
		m.proxyURL = status.ProxyURL
	} else {
		status.FallbackReason = reason + "_local_port_missing"
	}
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var err error
	if m.loopCancel != nil {
		m.loopCancel()
		m.loopCancel = nil
	}
	if m.mihomo != nil {
		err = m.mihomo.Stop()
	}
	m.activeCfg = nil
	m.proxyURL = ""
	m.status = Status{}
	return err
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked()
}

func (m *Manager) ProxyURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proxyURL
}

func (m *Manager) statusLocked() Status {
	status := m.status
	if status.Mode != config.ProxyModeSubscription || m.mihomo == nil {
		return status
	}

	runtime := m.mihomo.RuntimeState()
	status.MihomoActive = runtime.Active
	if runtime.Active {
		status.MihomoError = ""
	} else if runtime.Error != "" {
		if shouldReplaceRuntimeError(status.MihomoError) {
			status.MihomoError = runtime.Error
		}
		m.signalRecoverLocked()
	}
	m.status = status
	return status
}

func (m *Manager) watchRecoverLoop(ctx context.Context) {
	ticker := time.NewTicker(recoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-m.recoverCh:
		}
		m.recoverIfNeeded()
	}
}

func (m *Manager) recoverIfNeeded() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.shouldRecoverLocked() {
		return
	}
	if !m.lastRecoverAt.IsZero() && time.Since(m.lastRecoverAt) < recoveryInterval {
		return
	}
	cfg := cloneConfig(m.activeCfg)
	m.lastRecoverAt = time.Now()

	if err := m.mihomo.Apply(cfg); err != nil {
		m.status.MihomoActive = false
		m.status.MihomoError = err.Error()
		return
	}

	m.status.Fallback = false
	m.status.FallbackReason = ""
	m.status.EffectiveMode = config.ProxyModeSubscription
	m.status.ProxyURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.Proxy.Mihomo.MixedPort)
	m.proxyURL = m.status.ProxyURL

	runtime := m.mihomo.RuntimeState()
	m.status.MihomoActive = runtime.Active
	if runtime.Active {
		m.status.MihomoError = ""
		return
	}
	m.status.MihomoError = runtime.Error
}

func (m *Manager) shouldRecoverLocked() bool {
	if m.activeCfg == nil || m.mihomo == nil {
		return false
	}
	if m.status.Mode != config.ProxyModeSubscription || m.activeCfg.Proxy.Mode != config.ProxyModeSubscription {
		return false
	}
	if strings.TrimSpace(m.activeCfg.Proxy.SubscriptionURL) == "" {
		return false
	}
	runtime := m.mihomo.RuntimeState()
	if runtime.Active {
		if !m.status.MihomoActive {
			m.status.MihomoActive = true
			m.status.MihomoError = ""
		}
		return false
	}
	if runtime.Error != "" {
		m.status.MihomoActive = false
		m.status.MihomoError = runtime.Error
	}
	return true
}

func (m *Manager) signalRecoverLocked() {
	if m.recoverCh == nil {
		return
	}
	select {
	case m.recoverCh <- struct{}{}:
	default:
	}
}

func cloneConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	if cfg.Proxy.Subscriptions != nil {
		cloned.Proxy.Subscriptions = append([]config.Subscription(nil), cfg.Proxy.Subscriptions...)
	}
	return &cloned
}

func shouldReplaceRuntimeError(current string) bool {
	switch strings.TrimSpace(current) {
	case "", "mihomo_not_started", "mihomo_process_exited":
		return true
	default:
		return false
	}
}

