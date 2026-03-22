package service

import (
	"fmt"
	"strings"
)

const (
	ReasonAccessDenied                                  = "access_denied"
	ReasonInstallRequiresElevationOrTaskSchedulerAccess = "install_requires_elevation_or_task_scheduler_access"
	ReasonInstallFailed                                 = "install_failed"
	ReasonServiceStartRequiresElevation                 = "service_start_requires_elevation"
	ReasonTaskStartRequiresTaskSchedulerAccess          = "task_start_requires_task_scheduler_access"
	ServiceNotInstalledReason                           = "service_not_installed"
	ReasonStartFailed                                   = "start_failed"
	ReasonStopFailed                                    = "stop_failed"
	ReasonStatusFailed                                  = "status_failed"
	ReasonInitFailed                                    = "init_failed"
)

type CommandErrorDetails struct {
	Reason                      string
	Hint                        string
	RequiresElevation           bool
	RequiresTaskSchedulerAccess bool
}

func DescribeCommandError(action string, mode Mode, err error) CommandErrorDetails {
	if err == nil {
		return CommandErrorDetails{}
	}

	reason := reasonFromError(err)
	if reason == "" && hasAccessDenied(err) {
		reason = ReasonAccessDenied
	}
	if reason == "" {
		reason = fallbackReason(action)
	}

	details := CommandErrorDetails{Reason: reason}
	switch reason {
	case ReasonInstallRequiresElevationOrTaskSchedulerAccess:
		details.Hint = "Run elevated to install the Windows service, or allow Scheduled Tasks creation for the current user."
		details.RequiresElevation = true
		details.RequiresTaskSchedulerAccess = true
	case ReasonServiceStartRequiresElevation:
		details.Hint = "Start the Windows service from an elevated session or through Service Control Manager."
		details.RequiresElevation = true
	case ReasonTaskStartRequiresTaskSchedulerAccess:
		details.Hint = "Start the scheduled task from the owning user session or grant Task Scheduler access."
		details.RequiresTaskSchedulerAccess = true
	case ServiceNotInstalledReason:
		details.Hint = "Install the Windows service or scheduled task before starting it."
	case ReasonAccessDenied:
		switch {
		case action == "install":
			details.Hint = "The current session cannot install the service or scheduled task."
			details.RequiresElevation = true
		case mode == ModeService:
			details.Hint = "The current session cannot control the Windows service."
			details.RequiresElevation = true
		case mode == ModeTask:
			details.Hint = "The current session cannot control the scheduled task."
			details.RequiresTaskSchedulerAccess = true
		default:
			details.Hint = "The current session does not have permission to complete the service command."
		}
	}

	return details
}

func wrapCommandReason(reason string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", reason, err)
}

func reasonFromError(err error) string {
	if err == nil {
		return ""
	}

	text := err.Error()
	for _, reason := range []string{
		ReasonInstallRequiresElevationOrTaskSchedulerAccess,
		ReasonServiceStartRequiresElevation,
		ReasonTaskStartRequiresTaskSchedulerAccess,
		ServiceNotInstalledReason,
		ReasonAccessDenied,
		ReasonInstallFailed,
		ReasonStartFailed,
		ReasonStopFailed,
		ReasonStatusFailed,
		ReasonInitFailed,
	} {
		if strings.HasPrefix(text, reason) {
			return reason
		}
	}
	return ""
}

func fallbackReason(action string) string {
	switch action {
	case "install":
		return ReasonInstallFailed
	case "start":
		return ReasonStartFailed
	case "stop":
		return ReasonStopFailed
	case "status":
		return ReasonStatusFailed
	case "init":
		return ReasonInitFailed
	default:
		return "command_failed"
	}
}
