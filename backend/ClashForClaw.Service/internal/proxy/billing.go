package proxy

import (
	"context"
	"fmt"
	"math"
	"time"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/mihomo"
)

type BillingStatus struct {
	Upload    int64   `json:"upload,omitempty"`
	Download  int64   `json:"download,omitempty"`
	Used      float64 `json:"used"`
	Limit     float64 `json:"limit"`
	Unit      string  `json:"unit"`
	State     string  `json:"state"`
	Note      string  `json:"note,omitempty"`
	Provider  string  `json:"provider,omitempty"`
	ExpireAt  int64   `json:"expire_at,omitempty"`
	UpdatedAt string  `json:"updated_at,omitempty"`
	Source    string  `json:"source,omitempty"`
}

func (m *Manager) Billing(ctx context.Context) BillingStatus {
	status := m.Status()

	m.mu.Lock()
	controllerPort := m.controllerPort
	secret := m.controllerSecret
	m.mu.Unlock()

	if status.Mode != config.ProxyModeSubscription {
		return BillingStatus{
			Upload:   0,
			Download: 0,
			Used:     0,
			Limit:    0,
			Unit:     "GB",
			State:    "unknown",
			Note:     "Subscription usage unavailable in local port mode.",
		}
	}
	if !status.MihomoActive {
		return BillingStatus{
			Upload:   0,
			Download: 0,
			Used:     0,
			Limit:    0,
			Unit:     "GB",
			State:    "unknown",
			Note:     "Mihomo not running.",
		}
	}

	usage, err := mihomo.FetchSubscriptionUsage(ctx, controllerPort, secret)
	if err != nil {
		return BillingStatus{
			Upload:   0,
			Download: 0,
			Used:     0,
			Limit:    0,
			Unit:     "GB",
			State:    "unknown",
			Note:     "Subscription usage unavailable.",
		}
	}

	usedBytes := usage.Upload + usage.Download
	limitBytes := usage.Total
	usedGB := bytesToGB(usedBytes)
	limitGB := bytesToGB(limitBytes)
	state := "active"
	if limitBytes <= 0 {
		state = "unknown"
	}
	if limitBytes > 0 {
		ratio := float64(usedBytes) / float64(limitBytes)
		if ratio >= 0.9 {
			state = "limited"
		}
	}
	if usage.ExpireAt > 0 && time.Now().Unix() > usage.ExpireAt {
		state = "suspended"
	}

	note := ""
	if usage.Provider != "" {
		note = fmt.Sprintf("Provider: %s", usage.Provider)
	}
	if usage.ExpireAt > 0 {
		exp := time.Unix(usage.ExpireAt, 0).Format("2006-01-02")
		if note == "" {
			note = fmt.Sprintf("Renews %s", exp)
		} else {
			note = fmt.Sprintf("%s | Renews %s", note, exp)
		}
	}

	return BillingStatus{
		Upload:    usage.Upload,
		Download:  usage.Download,
		Used:      usedGB,
		Limit:     limitGB,
		Unit:      "GB",
		State:     state,
		Note:      note,
		Provider:  usage.Provider,
		ExpireAt:  usage.ExpireAt,
		UpdatedAt: usage.UpdatedAt,
		Source:    "mihomo",
	}
}

func bytesToGB(value int64) float64 {
	if value <= 0 {
		return 0
	}
	gb := float64(value) / 1_000_000_000
	return math.Round(gb*100) / 100
}

