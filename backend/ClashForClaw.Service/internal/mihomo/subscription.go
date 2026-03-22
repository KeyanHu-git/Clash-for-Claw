package mihomo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var ErrSubscriptionInfoMissing = errors.New("subscription_info_missing")

type SubscriptionUsage struct {
	Provider  string
	Upload    int64
	Download  int64
	Total     int64
	ExpireAt  int64
	UpdatedAt string
}

func FetchSubscriptionUsage(ctx context.Context, controllerPort int, secret string) (SubscriptionUsage, error) {
	if controllerPort <= 0 {
		return SubscriptionUsage{}, errors.New("controller_port_required")
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/providers/proxies", controllerPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return SubscriptionUsage{}, err
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return SubscriptionUsage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return SubscriptionUsage{}, fmt.Errorf("controller_status_%d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return SubscriptionUsage{}, err
	}
	providers, ok := payload["providers"].(map[string]any)
	if !ok {
		return SubscriptionUsage{}, ErrSubscriptionInfoMissing
	}

	for name, raw := range providers {
		info := mapFromAny(raw)
		sub := mapFromAny(info["subscriptionInfo"])
		if sub == nil {
			sub = mapFromAny(info["subscription-info"])
		}
		if sub == nil {
			sub = mapFromAny(info["subscription_info"])
		}
		if sub == nil {
			continue
		}
		usage := SubscriptionUsage{
			Provider:  name,
			Upload:    readInt64(sub["upload"]),
			Download:  readInt64(sub["download"]),
			Total:     readInt64(sub["total"]),
			ExpireAt:  readInt64(sub["expire"]),
			UpdatedAt: readString(info["updatedAt"]),
		}
		if usage.ExpireAt > 1_000_000_000_000 {
			usage.ExpireAt = usage.ExpireAt / 1000
		}
		return usage, nil
	}

	return SubscriptionUsage{}, ErrSubscriptionInfoMissing
}

func mapFromAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	if typed, ok := value.(map[string]interface{}); ok {
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[key] = val
		}
		return out
	}
	return nil
}

func readInt64(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case json.Number:
		if v, err := typed.Int64(); err == nil {
			return v
		}
	case string:
		if typed == "" {
			return 0
		}
		if v, err := strconv.ParseInt(typed, 10, 64); err == nil {
			return v
		}
	}
	return 0
}

func readString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}
