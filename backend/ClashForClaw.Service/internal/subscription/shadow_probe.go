package subscription

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/gateway"
	"clash-for-claw-service/internal/mihomo"
	"clash-for-claw-service/internal/runtime"
)

func probeSubscriptionWithShadowRuntime(ctx context.Context, paths runtime.Paths, baseCfg *config.Config, sub config.Subscription, gatewayURL string) gateway.ProbeResult {
	if baseCfg == nil {
		return gateway.ProbeResult{Error: "probe_config_missing"}
	}
	if strings.TrimSpace(sub.URL) == "" {
		return gateway.ProbeResult{Error: "subscription_url_missing"}
	}

	ports, release, err := reserveShadowPorts(4)
	if err != nil {
		return gateway.ProbeResult{Error: err.Error()}
	}

	shadowCfg := cloneConfig(baseCfg)
	shadowCfg.Proxy.Mode = config.ProxyModeSubscription
	shadowCfg.Proxy.SubscriptionURL = sub.URL
	shadowCfg.Proxy.ActiveSubscriptionId = sub.ID
	shadowCfg.SystemProxy.Enabled = false
	shadowCfg.Proxy.Mihomo.MixedPort = ports[0]
	shadowCfg.Proxy.Mihomo.HTTPPort = ports[1]
	shadowCfg.Proxy.Mihomo.SocksPort = ports[2]
	shadowCfg.Proxy.Mihomo.ControllerPort = ports[3]

	shadowPaths := buildShadowPaths(paths, sub)
	manager := mihomo.NewManager(shadowPaths)
	release()

	if err := manager.Apply(shadowCfg); err != nil {
		return gateway.ProbeResult{Error: err.Error()}
	}
	defer func() {
		_ = manager.Stop()
	}()

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", shadowCfg.Proxy.Mihomo.MixedPort)
	return gateway.Probe(ctx, gatewayURL, proxyURL)
}

func buildShadowPaths(paths runtime.Paths, sub config.Subscription) runtime.Paths {
	base := filepath.Join(paths.RuntimeDir, "shadow-probes", shadowProbeKey(sub))
	return runtime.Paths{
		BaseDir:    base,
		ConfigPath: filepath.Join(base, "config.json"),
		RuntimeDir: filepath.Join(base, "runtime"),
		BinDir:     filepath.Join(base, "bin"),
		LogsDir:    filepath.Join(base, "logs"),
		MihomoDir:  filepath.Join(base, "mihomo"),
	}
}

func shadowProbeKey(sub config.Subscription) string {
	candidate := strings.TrimSpace(sub.ID)
	if candidate == "" {
		candidate = strings.TrimSpace(sub.URL)
	}
	if candidate == "" {
		return "candidate"
	}

	builder := strings.Builder{}
	for _, r := range candidate {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_':
			builder.WriteRune(r)
		}
	}
	if builder.Len() > 0 {
		return builder.String()
	}

	sum := sha1.Sum([]byte(candidate))
	return hex.EncodeToString(sum[:8])
}

func reserveShadowPorts(count int) ([]int, func(), error) {
	listeners := make([]net.Listener, 0, count)
	ports := make([]int, 0, count)

	cleanup := func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}

	for i := 0; i < count; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		listeners = append(listeners, ln)
		addr, ok := ln.Addr().(*net.TCPAddr)
		if !ok || addr.Port <= 0 {
			cleanup()
			return nil, nil, fmt.Errorf("shadow_probe_port_unavailable_%d", i)
		}
		ports = append(ports, addr.Port)
	}

	return ports, cleanup, nil
}
