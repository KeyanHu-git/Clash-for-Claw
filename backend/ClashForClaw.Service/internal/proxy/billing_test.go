package proxy

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/runtime"
)

func TestBillingFallsBackToCachedSubscriptionUsage(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(runtime.Paths{
		BaseDir:    root,
		ConfigPath: filepath.Join(root, "config.json"),
		RuntimeDir: filepath.Join(root, "runtime"),
		BinDir:     filepath.Join(root, "bin"),
		LogsDir:    filepath.Join(root, "logs"),
		MihomoDir:  filepath.Join(root, "mihomo"),
	})
	defer manager.Stop()

	manager.mu.Lock()
	manager.status = Status{Mode: config.ProxyModeSubscription, MihomoActive: true}
	manager.controllerPort = 65530
	manager.activeCfg = &config.Config{
		Proxy: config.ProxyConfig{
			Mode:                 config.ProxyModeSubscription,
			SubscriptionURL:      "https://example.com/sub",
			ActiveSubscriptionId: "sub-1",
			Subscriptions: []config.Subscription{
				{
					ID:         "sub-1",
					Name:       "Primary",
					URL:        "https://example.com/sub",
					State:      "ok",
					UsageUsed:  23.5,
					UsageLimit: 100,
					UsageUnit:  "GB",
					ExpireAt:   time.Now().Add(24 * time.Hour).Unix(),
					UpdatedAt:  time.Now().Unix(),
				},
			},
		},
	}
	manager.mu.Unlock()

	billing := manager.Billing(context.Background())
	if billing.Source != "subscription_cache" {
		t.Fatalf("source = %q, want %q", billing.Source, "subscription_cache")
	}
	if billing.Used != 23.5 {
		t.Fatalf("used = %v, want %v", billing.Used, 23.5)
	}
	if billing.Limit != 100 {
		t.Fatalf("limit = %v, want %v", billing.Limit, 100.0)
	}
	if billing.Provider != "Primary" {
		t.Fatalf("provider = %q, want %q", billing.Provider, "Primary")
	}
	if billing.State != "ok" {
		t.Fatalf("state = %q, want %q", billing.State, "ok")
	}
}
