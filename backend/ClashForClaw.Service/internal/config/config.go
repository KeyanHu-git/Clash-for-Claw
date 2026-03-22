package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	ProxyModeLocalPort      = "local_port"
	ProxyModeSubscription   = "subscription_url"
	DefaultHTTPBind         = "127.0.0.1"
	DefaultHTTPPort         = 13000
	DefaultMihomoMixedPort  = 7890
	DefaultMihomoHTTPPort   = 7891
	DefaultMihomoSocksPort  = 7892
	DefaultControllerPort   = 9090
	DefaultProviderInterval = 86400
)

type Config struct {
	Version     int               `json:"version"`
	HTTP        HTTPConfig        `json:"http"`
	Gateway     GatewayConfig     `json:"gateway"`
	Proxy       ProxyConfig       `json:"proxy"`
	SystemProxy SystemProxyConfig `json:"system_proxy"`
}

type HTTPConfig struct {
	Bind string `json:"bind"`
	Port int    `json:"port"`
}

type GatewayConfig struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type ProxyConfig struct {
	Mode                     string         `json:"mode"`
	LocalPort                int            `json:"local_port"`
	SubscriptionURL          string         `json:"subscription_url"`
	Subscriptions            []Subscription `json:"subscriptions,omitempty"`
	ActiveSubscriptionId     string         `json:"active_subscription_id,omitempty"`
	SubscriptionRefreshHours int            `json:"subscription_refresh_hours,omitempty"`
	SubscriptionProbeMinutes int            `json:"subscription_probe_minutes,omitempty"`
	Mihomo                   MihomoConfig   `json:"mihomo"`
}

type MihomoConfig struct {
	MixedPort      int `json:"mixed_port"`
	HTTPPort       int `json:"http_port"`
	SocksPort      int `json:"socks_port"`
	ControllerPort int `json:"controller_port"`
}

type SystemProxyConfig struct {
	Enabled bool `json:"enabled"`
}

type Subscription struct {
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

func DefaultConfig() *Config {
	return &Config{
		Version: 1,
		HTTP: HTTPConfig{
			Bind: DefaultHTTPBind,
			Port: DefaultHTTPPort,
		},
		Proxy: ProxyConfig{
			Mode:                     ProxyModeSubscription,
			LocalPort:                DefaultMihomoMixedPort,
			SubscriptionRefreshHours: 6,
			SubscriptionProbeMinutes: 60,
			Mihomo: MihomoConfig{
				MixedPort:      DefaultMihomoMixedPort,
				HTTPPort:       DefaultMihomoHTTPPort,
				SocksPort:      DefaultMihomoSocksPort,
				ControllerPort: DefaultControllerPort,
			},
		},
		SystemProxy: SystemProxyConfig{Enabled: false},
	}
}

func LoadOrInit(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := DefaultConfig()
			if err := Save(path, cfg); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	normalize(&cfg)
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	normalize(cfg)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func normalize(cfg *Config) {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.HTTP.Bind == "" {
		cfg.HTTP.Bind = DefaultHTTPBind
	}
	if cfg.HTTP.Port == 0 {
		cfg.HTTP.Port = DefaultHTTPPort
	}
	if cfg.Proxy.Mode == "" {
		cfg.Proxy.Mode = ProxyModeSubscription
	}
	if cfg.Proxy.LocalPort == 0 {
		cfg.Proxy.LocalPort = DefaultMihomoMixedPort
	}
	if cfg.Proxy.Mihomo.MixedPort == 0 {
		cfg.Proxy.Mihomo.MixedPort = DefaultMihomoMixedPort
	}
	if cfg.Proxy.Mihomo.HTTPPort == 0 {
		cfg.Proxy.Mihomo.HTTPPort = DefaultMihomoHTTPPort
	}
	if cfg.Proxy.Mihomo.SocksPort == 0 {
		cfg.Proxy.Mihomo.SocksPort = DefaultMihomoSocksPort
	}
	if cfg.Proxy.Mihomo.ControllerPort == 0 {
		cfg.Proxy.Mihomo.ControllerPort = DefaultControllerPort
	}
	if cfg.Proxy.SubscriptionRefreshHours <= 0 {
		cfg.Proxy.SubscriptionRefreshHours = 6
	}
	if cfg.Proxy.SubscriptionProbeMinutes <= 0 {
		cfg.Proxy.SubscriptionProbeMinutes = 60
	}
	stripPlaceholderSubscriptions(cfg)
}

func stripPlaceholderSubscriptions(cfg *Config) {
	if cfg == nil {
		return
	}
	const placeholderURL = "https://example.com/subscription"
	placeholder := strings.TrimSpace(placeholderURL)
	if placeholder == "" {
		return
	}
	subs := cfg.Proxy.Subscriptions
	if len(subs) == 0 && strings.TrimSpace(cfg.Proxy.SubscriptionURL) != placeholder {
		return
	}
	filtered := make([]Subscription, 0, len(subs))
	removedActive := false
	for _, sub := range subs {
		if strings.EqualFold(strings.TrimSpace(sub.URL), placeholder) {
			if sub.ID != "" && sub.ID == cfg.Proxy.ActiveSubscriptionId {
				removedActive = true
			}
			continue
		}
		filtered = append(filtered, sub)
	}
	if len(filtered) == len(subs) {
		return
	}
	cfg.Proxy.Subscriptions = filtered
	if removedActive {
		cfg.Proxy.ActiveSubscriptionId = ""
	}
	if cfg.Proxy.ActiveSubscriptionId == "" {
		for _, sub := range filtered {
			if sub.ID != "" {
				cfg.Proxy.ActiveSubscriptionId = sub.ID
				if sub.URL != "" {
					cfg.Proxy.SubscriptionURL = sub.URL
				}
				break
			}
		}
	}
	if len(filtered) == 0 && strings.EqualFold(strings.TrimSpace(cfg.Proxy.SubscriptionURL), placeholder) {
		cfg.Proxy.SubscriptionURL = ""
	}
}
