package gateway

import (
	"context"
	"sync"
	"time"
)

const (
	defaultProbeFreshness = 3 * time.Second
	defaultProbeTimeout   = 3 * time.Second
	internetFailureGrace  = 20 * time.Second
)

type Prober struct {
	mu        sync.Mutex
	probeFunc func(context.Context, string, string) ProbeResult
	freshFor  time.Duration
	timeout   time.Duration
	state     probeCacheState
}

type probeCacheState struct {
	gatewayURL    string
	proxyURL      string
	result        ProbeResult
	lastHealthy   ProbeResult
	lastHealthyAt time.Time
	probedAt      time.Time
	refreshing    bool
}

func NewProber() *Prober {
	return &Prober{
		probeFunc: Probe,
		freshFor:  defaultProbeFreshness,
		timeout:   defaultProbeTimeout,
	}
}

func (p *Prober) Snapshot(ctx context.Context, gatewayURL string, proxyURL string) ProbeResult {
	if ctx == nil {
		ctx = context.Background()
	}

	p.mu.Lock()
	sameKey := p.state.gatewayURL == gatewayURL && p.state.proxyURL == proxyURL
	if sameKey && p.isFreshLocked(time.Now()) {
		result := p.state.result
		p.mu.Unlock()
		return result
	}

	if sameKey && !p.state.probedAt.IsZero() {
		if !p.state.refreshing {
			p.state.refreshing = true
			go p.refreshAsync(gatewayURL, proxyURL)
		}
		result := p.state.result
		p.mu.Unlock()
		return result
	}

	p.state.refreshing = true
	p.mu.Unlock()
	return p.refreshSync(ctx, gatewayURL, proxyURL)
}

func (p *Prober) refreshAsync(gatewayURL string, proxyURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	p.runProbe(ctx, gatewayURL, proxyURL)
}

func (p *Prober) refreshSync(ctx context.Context, gatewayURL string, proxyURL string) ProbeResult {
	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	return p.runProbe(probeCtx, gatewayURL, proxyURL)
}

func (p *Prober) runProbe(ctx context.Context, gatewayURL string, proxyURL string) ProbeResult {
	raw := p.probeFunc(ctx, gatewayURL, proxyURL)
	now := time.Now()

	p.mu.Lock()
	result, lastHealthy, lastHealthyAt := p.stabilizeResultLocked(gatewayURL, proxyURL, raw, now)
	p.state = probeCacheState{
		gatewayURL:    gatewayURL,
		proxyURL:      proxyURL,
		result:        result,
		lastHealthy:   lastHealthy,
		lastHealthyAt: lastHealthyAt,
		probedAt:      now,
		refreshing:    false,
	}
	p.mu.Unlock()

	return result
}

func (p *Prober) stabilizeResultLocked(gatewayURL string, proxyURL string, raw ProbeResult, now time.Time) (ProbeResult, ProbeResult, time.Time) {
	sameKey := p.state.gatewayURL == gatewayURL && p.state.proxyURL == proxyURL
	if !sameKey {
		if raw.GatewayOK && raw.InternetOK {
			return raw, raw, now
		}
		return raw, ProbeResult{}, time.Time{}
	}

	if raw.GatewayOK && raw.InternetOK {
		return raw, raw, now
	}

	if raw.GatewayOK && !raw.InternetOK {
		lastHealthy := p.state.lastHealthy
		lastHealthyAt := p.state.lastHealthyAt
		if lastHealthyAt.IsZero() && p.state.result.GatewayOK && p.state.result.InternetOK {
			lastHealthy = p.state.result
			lastHealthyAt = p.state.probedAt
		}
		if !lastHealthyAt.IsZero() && now.Sub(lastHealthyAt) <= internetFailureGrace && lastHealthy.GatewayOK && lastHealthy.InternetOK {
			return lastHealthy, lastHealthy, lastHealthyAt
		}
		return raw, lastHealthy, lastHealthyAt
	}

	return raw, ProbeResult{}, time.Time{}
}

func (p *Prober) isFreshLocked(now time.Time) bool {
	if p.state.probedAt.IsZero() {
		return false
	}
	return now.Sub(p.state.probedAt) <= p.freshFor
}
