package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type ProbeResult struct {
	GatewayOK  bool   `json:"gateway_ok"`
	InternetOK bool   `json:"internet_ok"`
	Error      string `json:"error,omitempty"`
}

func Probe(ctx context.Context, rawURL string, proxyURL string) ProbeResult {
	result := ProbeResult{}
	if rawURL == "" {
		result.Error = "gateway_url_required"
		return result
	}

	wsURLs := buildCandidateWSURLs(rawURL)
	for _, candidate := range wsURLs {
		if err := probeWebSocket(ctx, candidate, proxyURL); err == nil {
			result.GatewayOK = true
			break
		}
	}
	if !result.GatewayOK {
		result.Error = "gateway_unreachable"
	}

	if err := probeInternet(ctx, proxyURL); err == nil {
		result.InternetOK = true
	} else if result.Error == "" {
		result.Error = "internet_unreachable"
	}
	return result
}

func buildCandidateWSURLs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err == nil && u.Scheme != "" {
		switch u.Scheme {
		case "ws", "wss":
			return []string{raw}
		case "http":
			u.Scheme = "ws"
		case "https":
			u.Scheme = "wss"
		}
		return []string{u.String(), strings.TrimRight(u.String(), "/") + "/ws"}
	}
	return []string{raw}
}

func probeWebSocket(ctx context.Context, wsURL string, proxyURL string) error {
	dialer := websocket.Dialer{}
	if proxyURL != "" && !isLoopbackWS(wsURL) {
		if proxyParsed, err := url.Parse(proxyURL); err == nil {
			dialer.Proxy = http.ProxyURL(proxyParsed)
		}
	}
	dialer.HandshakeTimeout = 3 * time.Second
	headers := http.Header{}
	if origin := wsOrigin(wsURL); origin != "" {
		headers.Set("Origin", origin)
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func isLoopbackWS(wsURL string) bool {
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func wsOrigin(wsURL string) string {
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return ""
	}
	switch parsed.Scheme {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	default:
		return ""
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func probeInternet(ctx context.Context, proxyURL string) error {
	targets := []string{
		"https://www.gstatic.com/generate_204",
		"https://cp.cloudflare.com/generate_204",
		"https://www.qq.com/favicon.ico",
		"https://www.baidu.com/favicon.ico",
	}

	if strings.TrimSpace(proxyURL) != "" {
		return probeInternetTargets(ctx, targets, proxyURL)
	}
	return probeInternetTargets(ctx, targets, "")
}

func probeInternetTargets(ctx context.Context, targets []string, proxyURL string) error {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 3 * time.Second,
		}).DialContext,
	}
	if proxyURL != "" {
		if proxyParsed, err := url.Parse(proxyURL); err == nil {
			tr.Proxy = http.ProxyURL(proxyParsed)
		}
	}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	var lastErr error
	for _, target := range targets {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		// Any non-5xx means the network path is reachable.
		if resp.StatusCode < 500 {
			return nil
		}
		lastErr = errors.New("unexpected_status")
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("internet_probe_failed")
}
