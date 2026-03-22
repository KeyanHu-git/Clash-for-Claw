package mihomo

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/runtime"
)

type yamlConfig struct {
	MixedPort          int                       `yaml:"mixed-port,omitempty"`
	Port               int                       `yaml:"port,omitempty"`
	SocksPort          int                       `yaml:"socks-port,omitempty"`
	BindAddress        string                    `yaml:"bind-address,omitempty"`
	AllowLan           bool                      `yaml:"allow-lan"`
	Mode               string                    `yaml:"mode"`
	LogLevel           string                    `yaml:"log-level"`
	ExternalController string                    `yaml:"external-controller,omitempty"`
	Secret             string                    `yaml:"secret,omitempty"`
	DNS                map[string]any            `yaml:"dns"`
	ProxyProviders     map[string]map[string]any `yaml:"proxy-providers"`
	ProxyGroups        []map[string]any          `yaml:"proxy-groups"`
	Rules              []string                  `yaml:"rules"`
}

func buildConfig(cfg *config.Config, paths runtime.Paths) ([]byte, error) {
	providerPath := filepath.ToSlash(filepath.Join(paths.MihomoDir, "provider.yaml"))
	hasMMDB := hasLocalMMDB(paths)
	rules := []string{
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
	}
	if hasMMDB {
		rules = append(rules, "GEOIP,CN,DIRECT")
	}
	rules = append(rules,
		"DOMAIN-SUFFIX,cn,DIRECT",
		"MATCH,PROXY",
	)
	out := yamlConfig{
		MixedPort:          cfg.Proxy.Mihomo.MixedPort,
		Port:               cfg.Proxy.Mihomo.HTTPPort,
		SocksPort:          cfg.Proxy.Mihomo.SocksPort,
		BindAddress:        "127.0.0.1",
		AllowLan:           false,
		Mode:               "rule",
		LogLevel:           "info",
		ExternalController: fmt.Sprintf("127.0.0.1:%d", cfg.Proxy.Mihomo.ControllerPort),
		DNS: map[string]any{
			"enable":        true,
			"ipv6":          false,
			"enhanced-mode": "fake-ip",
			"nameserver":    []string{"1.1.1.1", "223.5.5.5"},
			"fallback":      []string{"8.8.8.8", "1.0.0.1"},
			"fallback-filter": map[string]any{
				"geoip":  hasMMDB,
				"ipcidr": []string{"240.0.0.0/4"},
			},
		},
		ProxyProviders: map[string]map[string]any{
			"provider": {
				"type":     "http",
				"url":      cfg.Proxy.SubscriptionURL,
				"interval": config.DefaultProviderInterval,
				"path":     providerPath,
				"health-check": map[string]any{
					"enable":   true,
					"url":      "http://www.google.com/generate_204",
					"interval": 300,
				},
			},
		},
		ProxyGroups: []map[string]any{
			{
				"name":     "PROXY",
				"type":     "url-test",
				"use":      []string{"provider"},
				"url":      "http://www.google.com/generate_204",
				"interval": 300,
			},
		},
		Rules: rules,
	}
	return yaml.Marshal(out)
}

func hasLocalMMDB(paths runtime.Paths) bool {
	candidates := []string{
		filepath.Join(paths.MihomoDir, "Country.mmdb"),
		filepath.Join(paths.MihomoDir, "country.mmdb"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.Size() > 0 {
			return true
		}
	}
	return false
}
