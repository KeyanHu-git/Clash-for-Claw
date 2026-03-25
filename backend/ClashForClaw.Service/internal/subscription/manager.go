package subscription

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/gateway"
	"clash-for-claw-service/internal/proxy"
	"clash-for-claw-service/internal/runtime"
)

const (
	SourceURL  = "url"
	SourceFile = "file"

	StateOK                      = "ok"
	StateError                   = "error"
	StateUnknown                 = "unknown"
	latencyStickinessThresholdMs = int64(150)
)

var errUserinfoMissing = errors.New("subscription_userinfo_missing")

type View struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Source        string  `json:"source"`
	URL           string  `json:"url,omitempty"`
	FilePath      string  `json:"file_path,omitempty"`
	State         string  `json:"state,omitempty"`
	LastSuccessAt int64   `json:"last_success_at,omitempty"`
	UsageUsed     float64 `json:"usage_used,omitempty"`
	UsageLimit    float64 `json:"usage_limit,omitempty"`
	UsageUnit     string  `json:"usage_unit,omitempty"`
	ExpireAt      int64   `json:"expire_at,omitempty"`
	UpdatedAt     int64   `json:"updated_at,omitempty"`
}

type Manager struct {
	mu         sync.Mutex
	failoverMu sync.Mutex
	paths      runtime.Paths
	cfg        *config.Config
	proxyMgr   proxyRuntime

	billingInterval time.Duration
	probeInterval   time.Duration
	billingUpdates  chan time.Duration
	probeUpdates    chan time.Duration
	loopCtx         context.Context
	loopCancel      context.CancelFunc
	probeCandidate  subscriptionProbeFunc
}

type proxyRuntime interface {
	Apply(*config.Config) proxy.Status
}

type subscriptionProbeFunc func(context.Context, runtime.Paths, *config.Config, config.Subscription, string) gateway.ProbeResult

func NewManager(paths runtime.Paths, cfg *config.Config, proxyMgr proxyRuntime) *Manager {
	return &Manager{
		paths:          paths,
		cfg:            cfg,
		proxyMgr:       proxyMgr,
		probeCandidate: probeSubscriptionWithShadowRuntime,
	}
}

func (m *Manager) Start() {
	m.mu.Lock()
	if m.billingUpdates != nil {
		m.mu.Unlock()
		return
	}
	m.compactDuplicatesLocked()
	m.normalizeNamesLocked()
	billing, probe := m.intervalsFromConfigLocked()
	m.billingInterval = billing
	m.probeInterval = probe
	m.billingUpdates = make(chan time.Duration, 1)
	m.probeUpdates = make(chan time.Duration, 1)
	m.loopCtx, m.loopCancel = context.WithCancel(context.Background())
	loopCtx := m.loopCtx
	m.mu.Unlock()

	go m.billingLoop(loopCtx)
	go m.probeLoop(loopCtx)
	go m.reconcileOnStart(loopCtx)
}

func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.loopCancel
	m.loopCancel = nil
	m.loopCtx = nil
	m.billingUpdates = nil
	m.probeUpdates = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) compactDuplicatesLocked() {
	if len(m.cfg.Proxy.Subscriptions) == 0 {
		return
	}

	seen := make(map[string]string)
	unique := make([]config.Subscription, 0, len(m.cfg.Proxy.Subscriptions))
	active := m.cfg.Proxy.ActiveSubscriptionId
	changed := false

	for _, sub := range m.cfg.Proxy.Subscriptions {
		key := subscriptionIdentityKey(sub)
		if key == "" {
			unique = append(unique, sub)
			continue
		}
		if keptID, ok := seen[key]; ok {
			changed = true
			if active == sub.ID && keptID != "" {
				active = keptID
			}
			continue
		}
		seen[key] = sub.ID
		unique = append(unique, sub)
	}

	if !changed {
		return
	}

	m.cfg.Proxy.Subscriptions = unique
	m.cfg.Proxy.ActiveSubscriptionId = active
	if active != "" {
		if sub, _ := m.findLocked(active); sub != nil && sub.URL != "" {
			m.cfg.Proxy.SubscriptionURL = sub.URL
		}
	}
	_ = config.Save(m.paths.ConfigPath, m.cfg)
}

func (m *Manager) UpdateIntervals() {
	m.mu.Lock()
	billing, probe := m.intervalsFromConfigLocked()
	m.billingInterval = billing
	m.probeInterval = probe
	billingCh := m.billingUpdates
	probeCh := m.probeUpdates
	m.mu.Unlock()

	sendInterval(billingCh, billing)
	sendInterval(probeCh, probe)
}

func (m *Manager) ListView() ([]View, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compactDuplicatesLocked()
	m.normalizeNamesLocked()

	active := m.cfg.Proxy.ActiveSubscriptionId
	views := make([]View, 0, len(m.cfg.Proxy.Subscriptions))
	for _, sub := range m.cfg.Proxy.Subscriptions {
		name := sub.Name
		if isInvalidName(name) {
			name = "订阅"
		}
		views = append(views, View{
			ID:            sub.ID,
			Name:          name,
			Source:        sub.Source,
			URL:           maskURL(sub.URL),
			FilePath:      sub.FilePath,
			State:         sub.State,
			LastSuccessAt: sub.LastSuccessAt,
			UsageUsed:     sub.UsageUsed,
			UsageLimit:    sub.UsageLimit,
			UsageUnit:     sub.UsageUnit,
			ExpireAt:      sub.ExpireAt,
			UpdatedAt:     sub.UpdatedAt,
		})
	}
	return views, active
}

func (m *Manager) AddURL(name string, raw string) (config.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return config.Subscription{}, errors.New("subscription_url_required")
	}
	candidateKey := subscriptionIdentityKey(config.Subscription{Source: SourceURL, URL: raw})

	for i := range m.cfg.Proxy.Subscriptions {
		sub := m.cfg.Proxy.Subscriptions[i]
		if strings.EqualFold(strings.TrimSpace(sub.URL), raw) || (candidateKey != "" && candidateKey == subscriptionIdentityKey(sub)) {
			if name != "" && (sub.Name == "" || isInvalidName(sub.Name)) {
				sub.Name = name
				m.cfg.Proxy.Subscriptions[i] = sub
			}
			m.cfg.Proxy.ActiveSubscriptionId = sub.ID
			m.cfg.Proxy.SubscriptionURL = sub.URL
			m.cfg.Proxy.Mode = config.ProxyModeSubscription
			if err := config.Save(m.paths.ConfigPath, m.cfg); err != nil {
				return config.Subscription{}, err
			}
			m.proxyMgr.Apply(m.cfg)
			return sub, nil
		}
	}

	if name == "" {
		name = m.nextDefaultNameLocked()
	}

	sub := config.Subscription{
		ID:        newID(),
		Name:      name,
		Source:    SourceURL,
		URL:       raw,
		State:     StateUnknown,
		UsageUnit: "GB",
	}

	m.cfg.Proxy.Subscriptions = append(m.cfg.Proxy.Subscriptions, sub)
	m.cfg.Proxy.ActiveSubscriptionId = sub.ID
	m.cfg.Proxy.SubscriptionURL = raw
	m.cfg.Proxy.Mode = config.ProxyModeSubscription

	m.refreshUsageLocked(&sub)
	m.replaceLocked(sub)

	if err := config.Save(m.paths.ConfigPath, m.cfg); err != nil {
		return config.Subscription{}, err
	}
	m.proxyMgr.Apply(m.cfg)
	return sub, nil
}

func (m *Manager) ImportFile(path string) (config.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	path = strings.TrimSpace(path)
	if path == "" {
		return config.Subscription{}, errors.New("file_path_required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config.Subscription{}, err
	}

	destDir := filepath.Join(m.paths.RuntimeDir, "subscriptions")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return config.Subscription{}, err
	}
	destName := time.Now().Format("20060102-150405") + "-" + filepath.Base(path)
	destPath := filepath.Join(destDir, destName)
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return config.Subscription{}, err
	}

	name, urlValue := parseProviderURL(data)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	sub := config.Subscription{
		ID:        newID(),
		Name:      name,
		Source:    SourceFile,
		URL:       urlValue,
		FilePath:  destPath,
		State:     StateUnknown,
		UsageUnit: "GB",
	}

	m.cfg.Proxy.Subscriptions = append(m.cfg.Proxy.Subscriptions, sub)
	if sub.URL != "" {
		m.cfg.Proxy.ActiveSubscriptionId = sub.ID
		m.cfg.Proxy.SubscriptionURL = sub.URL
		m.cfg.Proxy.Mode = config.ProxyModeSubscription
		m.refreshUsageLocked(&sub)
		m.replaceLocked(sub)
		if err := config.Save(m.paths.ConfigPath, m.cfg); err != nil {
			return config.Subscription{}, err
		}
		m.proxyMgr.Apply(m.cfg)
		return sub, nil
	}

	if err := config.Save(m.paths.ConfigPath, m.cfg); err != nil {
		return config.Subscription{}, err
	}
	return sub, nil
}

func (m *Manager) Activate(id string) (config.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, _ := m.findLocked(id)
	if sub == nil {
		return config.Subscription{}, errors.New("subscription_not_found")
	}
	if sub.URL == "" {
		return config.Subscription{}, errors.New("subscription_url_missing")
	}

	m.cfg.Proxy.ActiveSubscriptionId = sub.ID
	m.cfg.Proxy.SubscriptionURL = sub.URL
	m.cfg.Proxy.Mode = config.ProxyModeSubscription
	if err := config.Save(m.paths.ConfigPath, m.cfg); err != nil {
		return config.Subscription{}, err
	}
	m.proxyMgr.Apply(m.cfg)
	return *sub, nil
}

func (m *Manager) Refresh(id string) (config.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, _ := m.findLocked(id)
	if sub == nil {
		return config.Subscription{}, errors.New("subscription_not_found")
	}
	if sub.URL == "" {
		return *sub, nil
	}
	m.refreshUsageLocked(sub)
	if err := config.Save(m.paths.ConfigPath, m.cfg); err != nil {
		return config.Subscription{}, err
	}
	return *sub, nil
}

func (m *Manager) RefreshAll() ([]config.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.cfg.Proxy.Subscriptions {
		sub := &m.cfg.Proxy.Subscriptions[i]
		if sub.URL == "" {
			continue
		}
		m.refreshUsageLocked(sub)
	}
	if err := config.Save(m.paths.ConfigPath, m.cfg); err != nil {
		return nil, err
	}
	return append([]config.Subscription(nil), m.cfg.Proxy.Subscriptions...), nil
}

func (m *Manager) Rename(id string, name string) (config.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return config.Subscription{}, errors.New("name_required")
	}
	sub, _ := m.findLocked(id)
	if sub == nil {
		return config.Subscription{}, errors.New("subscription_not_found")
	}
	sub.Name = name
	if err := config.Save(m.paths.ConfigPath, m.cfg); err != nil {
		return config.Subscription{}, err
	}
	return *sub, nil
}

func (m *Manager) Delete(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	index := -1
	for i, sub := range m.cfg.Proxy.Subscriptions {
		if sub.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return m.cfg.Proxy.ActiveSubscriptionId, errors.New("subscription_not_found")
	}

	m.cfg.Proxy.Subscriptions = append(m.cfg.Proxy.Subscriptions[:index], m.cfg.Proxy.Subscriptions[index+1:]...)

	active := m.cfg.Proxy.ActiveSubscriptionId
	if active == id {
		active = m.selectFallbackLocked()
		m.cfg.Proxy.ActiveSubscriptionId = active
		if active != "" {
			if sub, _ := m.findLocked(active); sub != nil && sub.URL != "" {
				m.cfg.Proxy.SubscriptionURL = sub.URL
				m.cfg.Proxy.Mode = config.ProxyModeSubscription
				m.proxyMgr.Apply(m.cfg)
			}
		} else {
			m.cfg.Proxy.SubscriptionURL = ""
			m.cfg.Proxy.Mode = config.ProxyModeLocalPort
			m.proxyMgr.Apply(m.cfg)
		}
	}

	if err := config.Save(m.paths.ConfigPath, m.cfg); err != nil {
		return active, err
	}
	return active, nil
}

func (m *Manager) CopyURL(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, _ := m.findLocked(id)
	if sub == nil {
		return "", errors.New("subscription_not_found")
	}
	if sub.URL == "" {
		return "", errors.New("subscription_url_missing")
	}
	return sub.URL, nil
}

func (m *Manager) ProbeAndFailover() error {
	m.mu.Lock()
	gatewayURL := m.cfg.Gateway.URL
	mode := m.cfg.Proxy.Mode
	m.mu.Unlock()

	if gatewayURL == "" || mode != config.ProxyModeSubscription {
		return nil
	}
	return m.failover(gatewayURL)
}

func (m *Manager) ReconcilePreferredSubscription() error {
	m.mu.Lock()
	gatewayURL := m.cfg.Gateway.URL
	mode := m.cfg.Proxy.Mode
	m.mu.Unlock()

	if gatewayURL == "" || mode != config.ProxyModeSubscription {
		return nil
	}

	return m.failover(gatewayURL)
}

func (m *Manager) billingLoop(ctx context.Context) {
	m.RefreshAll()
	m.mu.Lock()
	interval := m.billingInterval
	updates := m.billingUpdates
	m.mu.Unlock()

	if interval <= 0 {
		interval = 6 * time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.RefreshAll()
		case next := <-updates:
			if next <= 0 {
				next = interval
			}
			if next != interval {
				interval = next
				ticker.Reset(interval)
			}
		}
	}
}

func (m *Manager) probeLoop(ctx context.Context) {
	m.mu.Lock()
	interval := m.probeInterval
	updates := m.probeUpdates
	m.mu.Unlock()

	if interval <= 0 {
		interval = 60 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = m.scheduledMaintenance()
		case next := <-updates:
			if next <= 0 {
				next = interval
			}
			if next != interval {
				interval = next
				ticker.Reset(interval)
			}
		}
	}
}

func (m *Manager) scheduledMaintenance() error {
	m.mu.Lock()
	shouldReconcile := m.cfg.Proxy.Mode == config.ProxyModeSubscription && m.cfg.Gateway.URL != "" && len(m.cfg.Proxy.Subscriptions) > 1
	m.mu.Unlock()

	if shouldReconcile {
		return m.ReconcilePreferredSubscription()
	}
	return m.ProbeAndFailover()
}

func (m *Manager) reconcileOnStart(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	_ = m.ReconcilePreferredSubscription()
}

func (m *Manager) failover(gatewayURL string) error {
	m.failoverMu.Lock()
	defer m.failoverMu.Unlock()

	snapshot := m.snapshotForFailover()
	if len(snapshot.Subscriptions) == 0 {
		return nil
	}

	results := make([]probedSubscriptionCandidate, 0, len(snapshot.Subscriptions))
	for i, sub := range snapshot.Subscriptions {
		if strings.TrimSpace(sub.URL) == "" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		probe := m.probeCandidate(ctx, m.paths, snapshot.Config, sub, gatewayURL)
		cancel()

		results = append(results, probedSubscriptionCandidate{
			Index:        i,
			Subscription: sub,
			Probe:        probe,
		})
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if gatewayURL == "" {
		return nil
	}
	if m.cfg.Proxy.Mode != config.ProxyModeSubscription {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(m.cfg.Gateway.URL), strings.TrimSpace(gatewayURL)) {
		return nil
	}
	if !matchesFailoverSnapshot(m.cfg, snapshot) {
		return nil
	}

	originalActiveID := m.cfg.Proxy.ActiveSubscriptionId
	now := time.Now().Unix()
	candidates := make([]subscriptionCandidate, 0, len(results))

	for _, result := range results {
		sub, idx := m.findLocked(result.Subscription.ID)
		if sub == nil || idx < 0 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(sub.URL), strings.TrimSpace(result.Subscription.URL)) {
			continue
		}
		if result.Probe.GatewayOK && result.Probe.InternetOK {
			sub.State = StateOK
			sub.LastSuccessAt = now
		} else if result.Probe.GatewayOK {
			sub.State = StateUnknown
		} else {
			sub.State = StateError
		}
		candidates = append(candidates, subscriptionCandidate{
			Index:   idx,
			Current: sub.ID == originalActiveID,
			Probe:   result.Probe,
		})
	}

	var chosen *config.Subscription
	if hasHealthyCandidate(candidates) {
		chosen = pickBestHealthySubscription(m.cfg.Proxy.Subscriptions, candidates)
	} else {
		chosen = pickLatestSuccess(m.cfg.Proxy.Subscriptions)
	}

	if chosen != nil && chosen.URL != "" {
		if chosen.ID != originalActiveID ||
			!strings.EqualFold(strings.TrimSpace(m.cfg.Proxy.SubscriptionURL), strings.TrimSpace(chosen.URL)) ||
			m.cfg.Proxy.Mode != config.ProxyModeSubscription {
			m.cfg.Proxy.ActiveSubscriptionId = chosen.ID
			m.cfg.Proxy.SubscriptionURL = chosen.URL
			m.cfg.Proxy.Mode = config.ProxyModeSubscription
			m.proxyMgr.Apply(m.cfg)
		}
	}

	return config.Save(m.paths.ConfigPath, m.cfg)
}

func (m *Manager) snapshotForFailover() failoverSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	return failoverSnapshot{
		Config:        cloneConfig(m.cfg),
		Subscriptions: append([]config.Subscription(nil), m.cfg.Proxy.Subscriptions...),
	}
}

func (m *Manager) refreshUsageLocked(sub *config.Subscription) bool {
	used, limit, expire, name, err := fetchUsage(sub.URL)
	if name != "" {
		sub.Name = name
	}
	if err != nil {
		if errors.Is(err, errUserinfoMissing) {
			sub.State = StateUnknown
			sub.UpdatedAt = time.Now().Unix()
			return true
		}
		sub.State = StateError
		return false
	}
	sub.UsageUsed = used
	sub.UsageLimit = limit
	sub.UsageUnit = "GB"
	sub.ExpireAt = expire
	sub.UpdatedAt = time.Now().Unix()
	sub.State = StateOK
	return true
}

func (m *Manager) findLocked(id string) (*config.Subscription, int) {
	for i := range m.cfg.Proxy.Subscriptions {
		sub := &m.cfg.Proxy.Subscriptions[i]
		if sub.ID == id {
			return sub, i
		}
	}
	return nil, -1
}

func (m *Manager) replaceLocked(sub config.Subscription) {
	for i := range m.cfg.Proxy.Subscriptions {
		if m.cfg.Proxy.Subscriptions[i].ID == sub.ID {
			m.cfg.Proxy.Subscriptions[i] = sub
			return
		}
	}
}

func (m *Manager) selectFallbackLocked() string {
	if len(m.cfg.Proxy.Subscriptions) == 0 {
		return ""
	}
	chosen := pickLatestSuccess(m.cfg.Proxy.Subscriptions)
	if chosen != nil {
		return chosen.ID
	}
	return m.cfg.Proxy.Subscriptions[0].ID
}

type subscriptionCandidate struct {
	Index   int
	Current bool
	Probe   gateway.ProbeResult
}

type probedSubscriptionCandidate struct {
	Index        int
	Subscription config.Subscription
	Probe        gateway.ProbeResult
}

type failoverSnapshot struct {
	Config        *config.Config
	Subscriptions []config.Subscription
}

func matchesFailoverSnapshot(current *config.Config, snapshot failoverSnapshot) bool {
	if current == nil || snapshot.Config == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.Proxy.ActiveSubscriptionId), strings.TrimSpace(snapshot.Config.Proxy.ActiveSubscriptionId)) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(current.Proxy.SubscriptionURL), strings.TrimSpace(snapshot.Config.Proxy.SubscriptionURL)) {
		return false
	}
	if len(current.Proxy.Subscriptions) != len(snapshot.Subscriptions) {
		return false
	}

	for i := range snapshot.Subscriptions {
		left := current.Proxy.Subscriptions[i]
		right := snapshot.Subscriptions[i]
		if left.ID != right.ID {
			return false
		}
		if !strings.EqualFold(strings.TrimSpace(left.URL), strings.TrimSpace(right.URL)) {
			return false
		}
	}

	return true
}

func hasHealthyCandidate(candidates []subscriptionCandidate) bool {
	for _, candidate := range candidates {
		if candidate.Probe.GatewayOK && candidate.Probe.InternetOK {
			return true
		}
	}
	return false
}

func pickBestHealthySubscription(subs []config.Subscription, candidates []subscriptionCandidate) *config.Subscription {
	if len(candidates) == 0 {
		return nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return betterCandidate(candidates[i], candidates[j])
	})

	best := candidates[0]
	if best.Index < 0 || best.Index >= len(subs) {
		return nil
	}
	return &subs[best.Index]
}

func betterCandidate(left subscriptionCandidate, right subscriptionCandidate) bool {
	leftTier := candidateTier(left.Probe)
	rightTier := candidateTier(right.Probe)
	if leftTier != rightTier {
		return leftTier > rightTier
	}

	leftLatency := candidateLatencyScore(left.Probe)
	rightLatency := candidateLatencyScore(right.Probe)

	if left.Current != right.Current {
		if left.Current && leftLatency <= rightLatency+latencyStickinessThresholdMs {
			return true
		}
		if right.Current && rightLatency <= leftLatency+latencyStickinessThresholdMs {
			return false
		}
	}

	if leftLatency != rightLatency {
		return leftLatency < rightLatency
	}

	return left.Index < right.Index
}

func candidateTier(probe gateway.ProbeResult) int {
	switch {
	case probe.GatewayOK && probe.InternetOK:
		return 2
	case probe.GatewayOK:
		return 1
	default:
		return 0
	}
}

func candidateLatencyScore(probe gateway.ProbeResult) int64 {
	const unknownLatencyPenaltyMs = int64(4000)

	total := probe.GatewayLatencyMs
	if total <= 0 {
		total = unknownLatencyPenaltyMs
	}

	if probe.InternetOK {
		if probe.InternetLatencyMs > 0 {
			total += probe.InternetLatencyMs
		}
		return total
	}

	return total + unknownLatencyPenaltyMs
}

func pickLatestSuccess(subs []config.Subscription) *config.Subscription {
	var chosen *config.Subscription
	var last int64
	for i := range subs {
		sub := &subs[i]
		if sub.LastSuccessAt > last {
			last = sub.LastSuccessAt
			chosen = sub
		}
	}
	return chosen
}

func parseProviderURL(data []byte) (string, string) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return "", ""
	}
	providers, ok := raw["proxy-providers"].(map[string]any)
	if !ok {
		return "", ""
	}
	for name, value := range providers {
		if item, ok := value.(map[string]any); ok {
			if urlValue, ok := item["url"].(string); ok && urlValue != "" {
				return name, urlValue
			}
		}
	}
	return "", ""
}

func subscriptionIdentityKey(sub config.Subscription) string {
	if sub.Source == SourceFile && strings.TrimSpace(sub.FilePath) != "" {
		clean := filepath.Clean(strings.TrimSpace(sub.FilePath))
		return "file:" + strings.ToLower(clean)
	}

	raw := strings.TrimSpace(sub.URL)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "url:" + strings.ToLower(raw)
	}

	// Mirror domains often share one token; keep only one record for that token.
	token := strings.TrimSpace(parsed.Query().Get("token"))
	if token != "" {
		return "token:" + strings.ToLower(token)
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	return "url:" + parsed.String()
}

func fetchUsage(raw string) (float64, float64, int64, string, error) {
	if raw == "" {
		return 0, 0, 0, "", errors.New("subscription_url_missing")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, raw, nil)
	if err != nil {
		return 0, 0, 0, "", err
	}
	req.Header.Set("User-Agent", "ClashforWindows/0.20.39")
	req.Header.Set("Accept", "*/*")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, 0, "", err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	name := extractProfileName(resp.Header)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, 0, 0, name, fmt.Errorf("subscription_status_%d", resp.StatusCode)
	}

	header := resp.Header.Get("subscription-userinfo")
	if header == "" {
		return 0, 0, 0, name, errUserinfoMissing
	}
	upload, download, total, expire := parseUserinfo(header)
	used := bytesToGB(upload + download)
	limit := bytesToGB(total)
	return used, limit, expire, name, nil
}

func extractProfileName(header http.Header) string {
	name := strings.TrimSpace(header.Get("profile-name"))
	if name == "" {
		name = strings.TrimSpace(header.Get("profile-title"))
	}
	if name == "" {
		name = parseContentDisposition(header.Get("content-disposition"))
	}
	return name
}

func parseContentDisposition(value string) string {
	if value == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(value)
	if err == nil {
		if name := strings.TrimSpace(params["filename"]); name != "" {
			return name
		}
		if name := strings.TrimSpace(params["filename*"]); name != "" {
			return decodeRFC5987(name)
		}
	}
	return ""
}

func decodeRFC5987(value string) string {
	clean := strings.TrimSpace(value)
	lower := strings.ToLower(clean)
	if strings.HasPrefix(lower, "utf-8''") {
		clean = clean[7:]
	}
	if decoded, err := url.QueryUnescape(clean); err == nil {
		return decoded
	}
	return clean
}

func parseUserinfo(value string) (int64, int64, int64, int64) {
	var upload, download, total, expire int64
	parts := strings.Split(value, ";")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := kv[0]
		val := kv[1]
		switch key {
		case "upload":
			upload = parseInt64(val)
		case "download":
			download = parseInt64(val)
		case "total":
			total = parseInt64(val)
		case "expire":
			expire = parseInt64(val)
		}
	}
	if expire > 1_000_000_000_000 {
		expire = expire / 1000
	}
	return upload, download, total, expire
}

func parseInt64(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func bytesToGB(value int64) float64 {
	if value <= 0 {
		return 0
	}
	gb := float64(value) / 1_000_000_000
	return math.Round(gb*100) / 100
}

func (m *Manager) intervalsFromConfigLocked() (time.Duration, time.Duration) {
	refreshHours := m.cfg.Proxy.SubscriptionRefreshHours
	if refreshHours <= 0 {
		refreshHours = 6
	}
	probeMinutes := m.cfg.Proxy.SubscriptionProbeMinutes
	if probeMinutes <= 0 {
		probeMinutes = 60
	}
	if len(m.cfg.Proxy.Subscriptions) > 1 && probeMinutes == 60 {
		probeMinutes = 5
	}
	return time.Duration(refreshHours) * time.Hour, time.Duration(probeMinutes) * time.Minute
}

func (m *Manager) normalizeNamesLocked() {
	if len(m.cfg.Proxy.Subscriptions) == 0 {
		return
	}

	used := make(map[string]bool, len(m.cfg.Proxy.Subscriptions))
	for _, sub := range m.cfg.Proxy.Subscriptions {
		if !isInvalidName(sub.Name) {
			used[sub.Name] = true
		}
	}

	changed := false
	index := 1
	for i := range m.cfg.Proxy.Subscriptions {
		sub := m.cfg.Proxy.Subscriptions[i]
		if !isInvalidName(sub.Name) {
			continue
		}
		for {
			name := fmt.Sprintf("订阅 %d", index)
			index++
			if !used[name] {
				sub.Name = name
				used[name] = true
				changed = true
				m.cfg.Proxy.Subscriptions[i] = sub
				break
			}
		}
	}

	if changed {
		_ = config.Save(m.paths.ConfigPath, m.cfg)
	}
}

func (m *Manager) nextDefaultNameLocked() string {
	base := "订阅"
	used := make(map[string]bool, len(m.cfg.Proxy.Subscriptions))
	for _, sub := range m.cfg.Proxy.Subscriptions {
		if sub.Name != "" {
			used[sub.Name] = true
		}
	}
	for i := 1; i < 10000; i++ {
		name := fmt.Sprintf("%s %d", base, i)
		if !used[name] {
			return name
		}
	}
	return base
}

func maskURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	if parsed.User != nil {
		parsed.User = url.User("****")
	}
	q := parsed.Query()
	for _, key := range []string{"token", "access_token", "auth", "key"} {
		if q.Has(key) {
			q.Set(key, "****")
		}
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// MaskURL hides sensitive tokens in URLs for display.
func MaskURL(raw string) string {
	return maskURL(raw)
}

func newID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func isInvalidName(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	if strings.Trim(trimmed, "?") == "" {
		return true
	}
	return strings.ContainsRune(trimmed, '\uFFFD')
}

func sendInterval(ch chan time.Duration, value time.Duration) {
	if ch == nil {
		return
	}
	select {
	case ch <- value:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- value:
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
