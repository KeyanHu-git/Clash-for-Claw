package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kardianos/service"

	"clash-for-claw-service/internal/api"
	"clash-for-claw-service/internal/runtime"
	svc "clash-for-claw-service/internal/service"
)

func Run(args []string) int {
	baseDir, args := extractBaseDirArg(args)
	if handled, code := tryRunPIDFileCleaner(args); handled {
		return code
	}
	if len(args) > 0 && args[0] == "service" {
		paths, err := runtime.ResolvePaths(baseDir)
		if err != nil {
			log.Printf("paths init failed: %v", err)
			return 1
		}
		setupLogging(paths)
		return runServiceCommand(baseDir, paths, args[1:])
	}

	mode := resolveRunMode(args)

	var (
		paths runtime.Paths
		err   error
	)
	if mode == runModeService {
		paths, err = runtime.ResolveServicePaths(baseDir)
	} else {
		paths, err = runtime.ResolvePaths(baseDir)
	}
	if err != nil {
		log.Printf("paths init failed: %v", err)
		return 1
	}
	setupLogging(paths)

	switch mode {
	case runModeService:
		return runAsService(paths)
	}
	return runDaemon(paths)
}

type runMode string

const (
	runModeService runMode = "service"
	runModeDaemon  runMode = "daemon"
)

func resolveRunMode(args []string) runMode {
	if hasFlag(args, "--service") {
		return runModeService
	}
	return runModeDaemon
}

func runAsService(paths runtime.Paths) int {
	daemon, err := NewDaemon(paths)
	if err != nil {
		log.Printf("daemon init failed: %v", err)
		return 1
	}
	program := &serviceProgram{daemon: daemon}
	cfg := &service.Config{
		Name:        svc.ServiceName,
		DisplayName: svc.ServiceDisplayName,
		Description: svc.ServiceDescription,
		Arguments:   svc.ServiceArgs(paths.BaseDir),
	}
	s, err := service.New(program, cfg)
	if err != nil {
		log.Printf("service init failed: %v", err)
		return 1
	}
	if err := s.Run(); err != nil {
		log.Printf("service run failed: %v", err)
		return 1
	}
	return 0
}

type serviceProgram struct {
	daemon *Daemon
}

func (p *serviceProgram) Start(service.Service) error {
	return p.daemon.Start()
}

func (p *serviceProgram) Stop(service.Service) error {
	return p.daemon.Stop()
}

func runDaemon(paths runtime.Paths) int {
	daemon, err := NewDaemon(paths)
	if err != nil {
		log.Printf("daemon init failed: %v", err)
		return 1
	}
	if err := daemon.Start(); err != nil {
		log.Printf("daemon start failed: %v", err)
		return 1
	}
	return waitForSignal(daemon.server)
}

func waitForSignal(server *api.Server) int {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
		_ = server.Shutdown(nil)
	case err := <-server.Done():
		if err != nil && !errors.Is(err, api.ErrServerClosed) {
			log.Printf("server stopped: %v", err)
			return 1
		}
	}
	return 0
}

func runServiceCommand(baseOverride string, paths runtime.Paths, args []string) int {
	jsonOutput, args := extractFlag(args, "--json")
	if len(args) == 0 {
		fmt.Println("usage: ClashForClaw.Service.exe service install|start|stop|uninstall|status")
		return 2
	}
	manager, err := svc.NewManager(baseOverride, paths)
	if err != nil {
		log.Printf("service manager init failed: %v", err)
		writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(false, "init", svc.ModeNone, "", err))
		return 1
	}
	switch strings.ToLower(args[0]) {
	case "install":
		mode, err := manager.Install()
		if err != nil {
			log.Printf("install failed: %v", err)
			writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(false, "install", svc.ModeNone, "", err))
			return 1
		}
		writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(true, "install", mode, "", nil))
		if !jsonOutput {
			fmt.Printf("installed: %s\n", mode)
		}
	case "uninstall":
		if err := manager.Uninstall(); err != nil {
			log.Printf("uninstall failed: %v", err)
			writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(false, "uninstall", svc.ModeNone, "", err))
			return 1
		}
		writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(true, "uninstall", svc.ModeNone, "", nil))
		if !jsonOutput {
			fmt.Println("uninstalled")
		}
	case "start":
		mode, err := manager.StartWithMode()
		if err != nil {
			log.Printf("start failed: %v", err)
			writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(false, "start", mode, "", err))
			return 1
		}
		writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(true, "start", mode, "", nil))
		if !jsonOutput {
			fmt.Println("started")
		}
	case "stop":
		if err := manager.Stop(); err != nil {
			log.Printf("stop failed: %v", err)
			writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(false, "stop", svc.ModeNone, "", err))
			return 1
		}
		writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(true, "stop", svc.ModeNone, "", nil))
		if !jsonOutput {
			fmt.Println("stopped")
		}
	case "status":
		mode, status, err := manager.Status()
		if err != nil {
			log.Printf("status failed: %v", err)
			writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(false, "status", svc.ModeNone, "", err))
			return 1
		}
		writeServiceCommandJSON(jsonOutput, buildServiceCommandPayload(true, "status", mode, serviceStatusString(status), nil))
		if !jsonOutput {
			fmt.Printf("mode: %s, status: %s\n", mode, serviceStatusString(status))
		}
	default:
		fmt.Println("unknown service command")
		return 2
	}
	return 0
}

func setupLogging(paths runtime.Paths) {
	logPath := filepath.Join(paths.LogsDir, "service.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func extractFlag(args []string, flag string) (bool, []string) {
	filtered := make([]string, 0, len(args))
	found := false
	for _, arg := range args {
		if arg == flag {
			found = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return found, filtered
}

func extractBaseDirArg(args []string) (string, []string) {
	filtered := make([]string, 0, len(args))
	baseDir := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--base-dir" && i+1 < len(args) {
			baseDir = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--base-dir=") {
			baseDir = strings.TrimPrefix(arg, "--base-dir=")
			continue
		}
		filtered = append(filtered, arg)
	}
	return baseDir, filtered
}

func serviceStatusString(status service.Status) string {
	switch status {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	case service.StatusUnknown:
		return "unknown"
	default:
		return fmt.Sprintf("status_%d", status)
	}
}

type serviceCommandPayload struct {
	Ok                          bool   `json:"ok"`
	Action                      string `json:"action,omitempty"`
	Mode                        string `json:"mode,omitempty"`
	Status                      string `json:"status,omitempty"`
	Error                       string `json:"error,omitempty"`
	Reason                      string `json:"reason,omitempty"`
	Hint                        string `json:"hint,omitempty"`
	RequiresElevation           bool   `json:"requiresElevation,omitempty"`
	RequiresTaskSchedulerAccess bool   `json:"requiresTaskSchedulerAccess,omitempty"`
}

func buildServiceCommandPayload(ok bool, action string, mode svc.Mode, status string, err error) serviceCommandPayload {
	payload := serviceCommandPayload{
		Ok:     ok,
		Action: action,
		Mode:   string(mode),
		Status: status,
	}
	if err == nil {
		return payload
	}

	payload.Error = err.Error()
	details := svc.DescribeCommandError(action, mode, err)
	payload.Reason = details.Reason
	payload.Hint = details.Hint
	payload.RequiresElevation = details.RequiresElevation
	payload.RequiresTaskSchedulerAccess = details.RequiresTaskSchedulerAccess
	return payload
}

func writeServiceCommandJSON(enabled bool, payload serviceCommandPayload) {
	if !enabled {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("{\"ok\":false,\"action\":\"%s\",\"error\":\"json_marshal_failed\"}\n", payload.Action)
		return
	}
	fmt.Println(string(data))
}
