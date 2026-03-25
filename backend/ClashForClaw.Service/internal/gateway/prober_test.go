package gateway

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProberReturnsFreshCacheWithoutReprobing(t *testing.T) {
	var calls atomic.Int32
	prober := &Prober{
		probeFunc: func(context.Context, string, string) ProbeResult {
			calls.Add(1)
			return ProbeResult{GatewayOK: true, InternetOK: true}
		},
		freshFor: time.Minute,
		timeout:  time.Second,
	}

	first := prober.Snapshot(context.Background(), "http://gateway", "http://proxy")
	second := prober.Snapshot(context.Background(), "http://gateway", "http://proxy")

	if !first.GatewayOK || !second.GatewayOK {
		t.Fatal("expected cached healthy probe results")
	}
	if calls.Load() != 1 {
		t.Fatalf("probe count = %d, want 1", calls.Load())
	}
}

func TestProberReturnsStaleCacheWhileBackgroundRefreshRuns(t *testing.T) {
	var calls atomic.Int32
	firstReturned := make(chan struct{})
	secondMayFinish := make(chan struct{})
	secondStarted := make(chan struct{})
	var mu sync.Mutex
	results := []ProbeResult{
		{GatewayOK: true, InternetOK: true},
		{GatewayOK: false, InternetOK: false, Error: "gateway_unreachable"},
	}

	prober := &Prober{
		probeFunc: func(context.Context, string, string) ProbeResult {
			index := int(calls.Add(1) - 1)
			if index == 0 {
				close(firstReturned)
				return results[0]
			}

			close(secondStarted)
			<-secondMayFinish
			mu.Lock()
			defer mu.Unlock()
			return results[1]
		},
		freshFor: 50 * time.Millisecond,
		timeout:  time.Second,
	}

	_ = prober.Snapshot(context.Background(), "http://gateway", "http://proxy")
	<-firstReturned
	time.Sleep(60 * time.Millisecond)

	startedAt := time.Now()
	second := prober.Snapshot(context.Background(), "http://gateway", "http://proxy")
	elapsed := time.Since(startedAt)

	if !second.GatewayOK || !second.InternetOK {
		t.Fatal("expected stale cached result while refresh is still running")
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("stale cache path took too long: %v", elapsed)
	}

	<-secondStarted
	close(secondMayFinish)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		third := prober.Snapshot(context.Background(), "http://gateway", "http://proxy")
		if !third.GatewayOK && !third.InternetOK && third.Error == "gateway_unreachable" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timed out waiting for refreshed probe result")
}

func TestProberRefreshesSynchronouslyWhenProbeKeyChanges(t *testing.T) {
	var calls atomic.Int32
	prober := &Prober{
		probeFunc: func(context.Context, string, string) ProbeResult {
			switch calls.Add(1) {
			case 1:
				return ProbeResult{GatewayOK: true, InternetOK: true}
			case 2:
				return ProbeResult{GatewayOK: false, InternetOK: false, Error: "gateway_unreachable"}
			default:
				return ProbeResult{}
			}
		},
		freshFor: time.Minute,
		timeout:  time.Second,
	}

	first := prober.Snapshot(context.Background(), "http://gateway-a", "http://proxy-a")
	second := prober.Snapshot(context.Background(), "http://gateway-b", "http://proxy-b")

	if !first.GatewayOK {
		t.Fatal("expected first probe to be healthy")
	}
	if second.GatewayOK || second.InternetOK || second.Error != "gateway_unreachable" {
		t.Fatal("expected key change to force a new synchronous probe")
	}
	if calls.Load() != 2 {
		t.Fatalf("probe count = %d, want 2", calls.Load())
	}
}

func TestProberDebouncesTransientInternetFailures(t *testing.T) {
	prober := &Prober{}

	healthy := ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 42, InternetLatencyMs: 99}
	transientFailure := ProbeResult{GatewayOK: true, InternetOK: false, GatewayLatencyMs: 55, Error: "internet_unreachable"}
	now := time.Now()

	first, lastHealthy, lastHealthyAt := prober.stabilizeResultLocked("http://gateway", "http://proxy", healthy, now)
	prober.state = probeCacheState{
		gatewayURL:    "http://gateway",
		proxyURL:      "http://proxy",
		result:        first,
		lastHealthy:   lastHealthy,
		lastHealthyAt: lastHealthyAt,
	}

	second, secondLastHealthy, secondLastHealthyAt := prober.stabilizeResultLocked("http://gateway", "http://proxy", transientFailure, now.Add(5*time.Second))
	if !second.GatewayOK || !second.InternetOK {
		t.Fatal("expected first transient internet failure to keep the last healthy status")
	}
	if secondLastHealthyAt != lastHealthyAt || secondLastHealthy != lastHealthy {
		t.Fatal("expected first transient failure to preserve the last healthy observation")
	}

	prober.state.result = second
	prober.state.lastHealthy = secondLastHealthy
	prober.state.lastHealthyAt = secondLastHealthyAt

	third, thirdLastHealthy, thirdLastHealthyAt := prober.stabilizeResultLocked("http://gateway", "http://proxy", transientFailure, now.Add(15*time.Second))
	if !third.GatewayOK || !third.InternetOK {
		t.Fatal("expected transient internet failures within the grace window to keep the last healthy status")
	}
	if thirdLastHealthyAt != lastHealthyAt || thirdLastHealthy != lastHealthy {
		t.Fatal("expected repeated transient failures to preserve the original healthy observation")
	}
}

func TestProberPublishesInternetFailureAfterGraceExpires(t *testing.T) {
	prober := &Prober{
		state: probeCacheState{
			gatewayURL:    "http://gateway",
			proxyURL:      "http://proxy",
			result:        ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 42, InternetLatencyMs: 99},
			lastHealthy:   ProbeResult{GatewayOK: true, InternetOK: true, GatewayLatencyMs: 42, InternetLatencyMs: 99},
			lastHealthyAt: time.Now().Add(-internetFailureGrace - time.Second),
			probedAt:      time.Now().Add(-internetFailureGrace - time.Second),
		},
	}

	fourth, fourthLastHealthy, fourthLastHealthyAt := prober.stabilizeResultLocked(
		"http://gateway",
		"http://proxy",
		ProbeResult{GatewayOK: true, InternetOK: false, GatewayLatencyMs: 55, Error: "internet_unreachable"},
		time.Now(),
	)
	if !fourth.GatewayOK || fourth.InternetOK {
		t.Fatal("expected internet failure to be published after the grace window expires")
	}
	if !fourthLastHealthy.GatewayOK || !fourthLastHealthy.InternetOK || fourthLastHealthyAt.IsZero() {
		t.Fatal("expected the last healthy observation to be retained for future recovery")
	}
}

func TestProberPublishesGatewayFailuresImmediately(t *testing.T) {
	prober := &Prober{
		state: probeCacheState{
			gatewayURL: "http://gateway",
			proxyURL:   "http://proxy",
			result:     ProbeResult{GatewayOK: true, InternetOK: true},
		},
	}

	result, lastHealthy, lastHealthyAt := prober.stabilizeResultLocked(
		"http://gateway",
		"http://proxy",
		ProbeResult{GatewayOK: false, InternetOK: false, Error: "gateway_unreachable"},
		time.Now(),
	)

	if result.GatewayOK || result.InternetOK || result.Error != "gateway_unreachable" {
		t.Fatal("expected gateway failure to bypass the internet debounce filter")
	}
	if !lastHealthyAt.IsZero() || lastHealthy.GatewayOK || lastHealthy.InternetOK {
		t.Fatal("expected gateway failure to clear the transient internet debounce state")
	}
}
