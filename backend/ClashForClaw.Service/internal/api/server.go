package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"clash-for-claw-service/internal/config"
	"clash-for-claw-service/internal/gateway"
	"clash-for-claw-service/internal/proxy"
	"clash-for-claw-service/internal/runtime"
	"clash-for-claw-service/internal/service"
	"clash-for-claw-service/internal/subscription"
	"clash-for-claw-service/internal/systemproxy"
)

var ErrServerClosed = http.ErrServerClosed

type Server struct {
	paths             runtime.Paths
	cfgPath           string
	cfg               *config.Config
	allowSelfShutdown bool
	router            *mux.Router
	httpServer        *http.Server
	proxyMgr          *proxy.Manager
	prober            *gateway.Prober
	subMgr            *subscription.Manager
	sysProxy          *systemproxy.Manager
	nonce             string
	done              chan error
	shutdownMu        sync.Once
	shutdownErr       error
}

func NewServer(paths runtime.Paths, allowSelfShutdown bool) (*Server, error) {
	cfg, err := config.LoadOrInit(paths.ConfigPath)
	if err != nil {
		return nil, err
	}
	server := &Server{
		paths:             paths,
		cfgPath:           paths.ConfigPath,
		cfg:               cfg,
		allowSelfShutdown: allowSelfShutdown,
		proxyMgr:          proxy.NewManager(paths),
		prober:            gateway.NewProber(),
		sysProxy:          systemproxy.NewManager(paths),
		nonce:             randomNonce(),
		done:              make(chan error, 1),
	}
	server.subMgr = subscription.NewManager(paths, cfg, server.proxyMgr)
	server.router = server.routes()
	server.httpServer = &http.Server{
		Addr:    server.bindAddr(),
		Handler: server.router,
	}
	server.applyConfig(cfg)
	server.subMgr.Start()
	return server, nil
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	go func() {
		err := s.httpServer.Serve(ln)
		s.done <- err
	}()
	return nil
}

func (s *Server) Wait() error {
	return <-s.done
}

func (s *Server) Done() <-chan error {
	return s.done
}

func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.shutdownMu.Do(func() {
		s.subMgr.Stop()
		proxyErr := s.proxyMgr.Stop()
		httpErr := s.httpServer.Shutdown(ctx)
		s.shutdownErr = errors.Join(proxyErr, httpErr)
	})
	return s.shutdownErr
}

func (s *Server) bindAddr() string {
	return net.JoinHostPort(s.cfg.HTTP.Bind, fmtInt(s.cfg.HTTP.Port))
}

func (s *Server) routes() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/health", s.handleHealth).Methods("GET")
	r.HandleFunc("/nonce", s.handleGetNonce).Methods("GET")
	r.HandleFunc("/status", s.handleStatus).Methods("GET")
	r.HandleFunc("/config", s.handleGetConfig).Methods("GET")
	r.HandleFunc("/config", s.handlePostConfig).Methods("POST")
	r.HandleFunc("/config/reload", s.handleReload).Methods("POST")
	r.HandleFunc("/subscriptions", s.handleGetSubscriptions).Methods("GET")
	r.HandleFunc("/subscriptions", s.handlePostSubscription).Methods("POST")
	r.HandleFunc("/subscriptions/import", s.handleImportSubscription).Methods("POST")
	r.HandleFunc("/subscriptions/refresh", s.handleRefreshSubscriptions).Methods("POST")
	r.HandleFunc("/subscriptions/{id}/activate", s.handleActivateSubscription).Methods("POST")
	r.HandleFunc("/subscriptions/{id}/refresh", s.handleRefreshSubscription).Methods("POST")
	r.HandleFunc("/subscriptions/{id}/rename", s.handleRenameSubscription).Methods("POST")
	r.HandleFunc("/subscriptions/{id}/copy", s.handleCopySubscription).Methods("POST")
	r.HandleFunc("/subscriptions/{id}", s.handleDeleteSubscription).Methods("DELETE")
	r.HandleFunc("/service/install", s.handleServiceInstall).Methods("POST")
	r.HandleFunc("/service/start", s.handleServiceStart).Methods("POST")
	r.HandleFunc("/service/stop", s.handleServiceStop).Methods("POST")
	r.HandleFunc("/service/uninstall", s.handleServiceUninstall).Methods("POST")
	r.HandleFunc("/daemon/shutdown", s.handleDaemonShutdown).Methods("POST")
	r.HandleFunc("/system-proxy/enable", s.handleSystemProxyEnable).Methods("POST")
	r.HandleFunc("/system-proxy/disable", s.handleSystemProxyDisable).Methods("POST")
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().Unix()})
}

func (s *Server) handleGetNonce(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "nonce": s.nonce})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	proxyStatus := s.proxyMgr.Status()
	probe := s.prober.Snapshot(r.Context(), s.cfg.Gateway.URL, proxyStatus.ProxyURL)
	enabled, server := s.sysProxy.Status()
	ok := probe.GatewayOK && probe.InternetOK
	if proxyStatus.Mode == config.ProxyModeSubscription && (proxyStatus.Fallback || !proxyStatus.MihomoActive) {
		ok = false
	}
	status := map[string]any{
		"ok":    ok,
		"proxy": proxyStatus,
		"probe": probe,
	}
	status["system_proxy"] = map[string]any{"enabled": enabled, "server": server}
	status["billing"] = s.proxyMgr.Billing(r.Context())
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := *s.cfg
	token := cfg.Gateway.Token
	cfg.Gateway.Token = ""
	cfg.Proxy.SubscriptionURL = subscription.MaskURL(cfg.Proxy.SubscriptionURL)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"config":        cfg,
		"token_present": token != "",
		"token_hint":    last4(token),
	})
}

func (s *Server) handlePostConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	var input config.Config
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_request"})
		return
	}
	merged := mergeConfig(s.cfg, &input)
	if err := config.Save(s.cfgPath, merged); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	*s.cfg = *merged
	s.subMgr.UpdateIntervals()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	cfg, err := config.LoadOrInit(s.cfgPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	*s.cfg = *cfg
	s.subMgr.UpdateIntervals()
	proxyStatus := s.applyConfig(cfg)
	probe := gateway.Probe(context.Background(), cfg.Gateway.URL, s.proxyMgr.ProxyURL())
	if !probe.GatewayOK {
		_ = s.subMgr.ProbeAndFailover()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "proxy": proxyStatus, "probe": probe})
}

type createSubscriptionRequest struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type importSubscriptionRequest struct {
	Path string `json:"path"`
}

type renameSubscriptionRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleGetSubscriptions(w http.ResponseWriter, r *http.Request) {
	subscriptions, active := s.subMgr.ListView()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"active_id":     active,
		"subscriptions": subscriptions,
	})
}

func (s *Server) handlePostSubscription(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	var input createSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_request"})
		return
	}
	if _, err := s.subMgr.AddURL(input.Name, input.URL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleImportSubscription(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	var input importSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_request"})
		return
	}
	if _, err := s.subMgr.ImportFile(input.Path); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleActivateSubscription(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	id := mux.Vars(r)["id"]
	if _, err := s.subMgr.Activate(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRefreshSubscription(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	id := mux.Vars(r)["id"]
	if _, err := s.subMgr.Refresh(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRefreshSubscriptions(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	if _, err := s.subMgr.RefreshAll(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRenameSubscription(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	id := mux.Vars(r)["id"]
	var input renameSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_request"})
		return
	}
	if _, err := s.subMgr.Rename(id, input.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	id := mux.Vars(r)["id"]
	active, err := s.subMgr.Delete(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active_id": active})
}

func (s *Server) handleCopySubscription(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	id := mux.Vars(r)["id"]
	urlValue, err := s.subMgr.CopyURL(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": urlValue})
}

func (s *Server) applyConfig(cfg *config.Config) proxy.Status {
	status := s.proxyMgr.Apply(cfg)
	server := toSystemProxyServer(status.ProxyURL)
	if cfg.SystemProxy.Enabled && server != "" {
		_ = s.sysProxy.Enable(server)
	}
	return status
}

func (s *Server) handleServiceInstall(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	manager, err := service.NewManager("", s.paths)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	mode, err := manager.Install()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": mode})
}

func (s *Server) handleServiceStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	manager, err := service.NewManager("", s.paths)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := manager.Start(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleServiceStop(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	manager, err := service.NewManager("", s.paths)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := manager.Stop(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleServiceUninstall(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	manager, err := service.NewManager("", s.paths)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := manager.Uninstall(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDaemonShutdown(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	if !s.allowSelfShutdown {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	go func() {
		time.Sleep(150 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}()
}

func (s *Server) handleSystemProxyEnable(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	proxyURL := s.proxyMgr.ProxyURL()
	if proxyURL == "" && s.cfg.Proxy.LocalPort > 0 {
		proxyURL = "http://127.0.0.1:" + strconv.Itoa(s.cfg.Proxy.LocalPort)
	}
	server := toSystemProxyServer(proxyURL)
	if server == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": "proxy_url_missing",
		})
		return
	}
	if err := s.sysProxy.Enable(server); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	s.cfg.SystemProxy.Enabled = true
	_ = config.Save(s.cfgPath, s.cfg)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"enabled": true,
		"server":  server,
	})
}

func (s *Server) handleSystemProxyDisable(w http.ResponseWriter, r *http.Request) {
	if !s.requireNonce(w, r) {
		return
	}
	if err := s.sysProxy.Disable(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	s.cfg.SystemProxy.Enabled = false
	_ = config.Save(s.cfgPath, s.cfg)
	enabled, server := s.sysProxy.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"enabled": enabled,
		"server":  server,
	})
}

func (s *Server) requireNonce(w http.ResponseWriter, r *http.Request) bool {
	if !isLoopback(r.RemoteAddr) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return false
	}
	if r.Method == http.MethodGet {
		return true
	}
	if r.Header.Get("X-Adapter-Nonce") != s.nonce {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return false
	}
	return true
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func mergeConfig(current *config.Config, input *config.Config) *config.Config {
	out := *current
	if input.HTTP.Bind != "" {
		out.HTTP.Bind = input.HTTP.Bind
	}
	if input.HTTP.Port != 0 {
		out.HTTP.Port = input.HTTP.Port
	}
	if input.Gateway.URL != "" {
		out.Gateway.URL = input.Gateway.URL
	}
	if input.Gateway.Token != "" {
		out.Gateway.Token = input.Gateway.Token
	}
	if input.Proxy.Mode != "" {
		out.Proxy.Mode = input.Proxy.Mode
	}
	if input.Proxy.LocalPort != 0 {
		out.Proxy.LocalPort = input.Proxy.LocalPort
	}
	if input.Proxy.SubscriptionURL != "" {
		out.Proxy.SubscriptionURL = input.Proxy.SubscriptionURL
	}
	if input.Proxy.Subscriptions != nil {
		out.Proxy.Subscriptions = input.Proxy.Subscriptions
	}
	if input.Proxy.ActiveSubscriptionId != "" {
		out.Proxy.ActiveSubscriptionId = input.Proxy.ActiveSubscriptionId
	}
	if input.Proxy.SubscriptionRefreshHours != 0 {
		out.Proxy.SubscriptionRefreshHours = input.Proxy.SubscriptionRefreshHours
	}
	if input.Proxy.SubscriptionProbeMinutes != 0 {
		out.Proxy.SubscriptionProbeMinutes = input.Proxy.SubscriptionProbeMinutes
	}
	if input.Proxy.Mihomo.MixedPort != 0 {
		out.Proxy.Mihomo.MixedPort = input.Proxy.Mihomo.MixedPort
	}
	if input.Proxy.Mihomo.HTTPPort != 0 {
		out.Proxy.Mihomo.HTTPPort = input.Proxy.Mihomo.HTTPPort
	}
	if input.Proxy.Mihomo.SocksPort != 0 {
		out.Proxy.Mihomo.SocksPort = input.Proxy.Mihomo.SocksPort
	}
	if input.Proxy.Mihomo.ControllerPort != 0 {
		out.Proxy.Mihomo.ControllerPort = input.Proxy.Mihomo.ControllerPort
	}
	if input.SystemProxy.Enabled {
		out.SystemProxy.Enabled = true
	}

	if normalized, token := extractToken(out.Gateway.URL); token != "" {
		out.Gateway.URL = normalized
		if out.Gateway.Token == "" {
			out.Gateway.Token = token
		}
	}
	return &out
}

func extractToken(raw string) (string, string) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw, ""
	}
	q := parsed.Query()
	token := ""
	for _, key := range []string{"token", "gateway_token", "gatewayToken"} {
		if value := q.Get(key); value != "" {
			token = value
			q.Del(key)
			break
		}
	}
	parsed.RawQuery = q.Encode()
	return parsed.String(), token
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func randomNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func last4(token string) string {
	if len(token) <= 4 {
		return token
	}
	return token[len(token)-4:]
}

func fmtInt(value int) string {
	return strconv.Itoa(value)
}

func toSystemProxyServer(proxyURL string) string {
	raw := strings.TrimSpace(proxyURL)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}
