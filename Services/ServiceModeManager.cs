using System.ComponentModel;
using System.Diagnostics;
using System.Text.Json;
using ClashForClaw.Models;

namespace ClashForClaw.Services;

public sealed class ServiceModeState
{
    public string Mode { get; init; } = "none";
    public string Status { get; init; } = "unknown";
    public string Error { get; init; } = string.Empty;
    public string Reason { get; init; } = string.Empty;
    public string Hint { get; init; } = string.Empty;
    public bool RequiresElevation { get; init; }
    public bool RequiresTaskSchedulerAccess { get; init; }

    public bool IsEnabled
        => !string.Equals(Mode, "none", StringComparison.OrdinalIgnoreCase);

    public bool IsServiceMode
        => string.Equals(Mode, "service", StringComparison.OrdinalIgnoreCase);

    public bool IsTaskFallback
        => string.Equals(Mode, "task", StringComparison.OrdinalIgnoreCase);

    public bool IsRunning
        => string.Equals(Status, "running", StringComparison.OrdinalIgnoreCase);
}

public sealed class ServiceModeResult
{
    public bool Failed { get; init; }
    public bool ServiceStarted { get; init; }
    public bool FallbackScheduled { get; init; }
    public bool DesktopFallbackStarted { get; init; }
    public string Message { get; init; } = string.Empty;
    public string Title { get; init; } = string.Empty;
}

public static class ServiceModeManager
{
    public static ServiceModeState Query(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        if (!File.Exists(cliPath))
        {
            return new ServiceModeState
            {
                Error = "未找到内置服务组件。",
                Reason = "cli_missing",
                Hint = "请确认发布目录中包含 ClashForClaw.Service.exe。",
            };
        }

        return ParseState(RunServiceCommand(cliPath, "status", allowElevation: false));
    }

    public static ServiceModeResult Enable(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        if (!File.Exists(cliPath))
        {
            return new ServiceModeResult
            {
                Failed = true,
                Title = "无法启用 Windows 服务模式",
                Message = "未找到内置服务组件，当前无法注册 Windows 服务。",
            };
        }

        try
        {
            AppPaths.EnsureServiceMihomoAvailable(cliPath);
        }
        catch (Exception ex)
        {
            return new ServiceModeResult
            {
                Failed = true,
                Title = "无法启用 Windows 服务模式",
                Message = $"准备 Windows 服务运行环境失败：{ex.Message}",
            };
        }

        var current = Query(settings);
        if (!current.IsEnabled)
        {
            var install = RunServiceCommand(cliPath, "install");
            if (!install.Ok)
            {
                return new ServiceModeResult
                {
                    Failed = true,
                    Title = "无法启用 Windows 服务模式",
                    Message = BuildEnableFailureMessage(install),
                };
            }

            current = Query(settings);
        }

        if (current.IsRunning)
        {
            return BuildEnabledResult(current);
        }

        var start = RunServiceCommand(cliPath, "start");
        if (!start.Ok)
        {
            var actual = Query(settings);
            if (actual.IsRunning)
            {
                return BuildEnabledResult(actual);
            }

            return new ServiceModeResult
            {
                Failed = true,
                Title = "无法启用 Windows 服务模式",
                Message = BuildEnableFailureMessage(start),
            };
        }

        var enabledState = Query(settings);
        if (enabledState.IsTaskFallback || enabledState.IsRunning)
        {
            return BuildEnabledResult(enabledState);
        }

        return new ServiceModeResult
        {
            Failed = true,
            Title = "Windows 服务模式未就绪",
            Message = "服务注册完成后没有进入预期状态，请检查服务日志与当前权限。",
        };
    }

    public static ServiceModeResult Disable(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        if (!File.Exists(cliPath))
        {
            return new ServiceModeResult
            {
                Title = "Windows 服务模式已关闭",
                Message = "未找到内置服务组件，当前已按桌面后台模式处理。",
            };
        }

        var current = Query(settings);
        if (!current.IsEnabled)
        {
            return new ServiceModeResult
            {
                Title = "Windows 服务模式已关闭",
                Message = "当前没有已注册的后台托管模式，已恢复为桌面后台。",
            };
        }

        var stop = RunServiceCommand(cliPath, "stop");
        var uninstall = RunServiceCommand(cliPath, "uninstall");
        var actual = Query(settings);

        if (actual.IsEnabled)
        {
            return new ServiceModeResult
            {
                Failed = true,
                Title = "关闭 Windows 服务模式失败",
                Message = BuildDisableFailureMessage(stop, uninstall),
            };
        }

        return new ServiceModeResult
        {
            Title = "已恢复为桌面后台",
            Message = "后台已切回桌面模式，现在可以继续使用托盘、静默启动和关闭最小化到托盘。",
        };
    }

    private static ProcessResult RunCli(string cliPath, string command)
    {
        var args = $"--base-dir \"{AppPaths.ServiceBaseDirectory}\" service {command} --json";
        return ProcessRunner.Run(cliPath, args);
    }

    private static ServiceCommandResponse RunServiceCommand(string cliPath, string command, bool allowElevation = true)
    {
        var response = ParseServiceCommand(RunCli(cliPath, command), command);
        if (!allowElevation || !ShouldRetryElevated(command, response))
        {
            return response;
        }

        return RunServiceCommandElevated(cliPath, command);
    }

    private static ServiceCommandResponse ParseServiceCommand(ProcessResult result, string action)
    {
        var parsed = TryParseServiceCommand(result.StandardOutput);
        if (parsed is not null)
        {
            return parsed;
        }

        return new ServiceCommandResponse
        {
            Action = action,
            Error = ExtractProcessError(result, "后台服务返回了无法识别的响应。"),
        };
    }

    private static ServiceCommandResponse? TryParseServiceCommand(string output)
    {
        if (string.IsNullOrWhiteSpace(output))
        {
            return null;
        }

        try
        {
            return JsonSerializer.Deserialize(output, AppJsonContext.Default.ServiceCommandResponse);
        }
        catch (JsonException)
        {
            return null;
        }
    }

    private static ServiceModeState ParseState(ServiceCommandResponse response)
    {
        return new ServiceModeState
        {
            Mode = string.IsNullOrWhiteSpace(response.Mode) ? "none" : response.Mode,
            Status = string.IsNullOrWhiteSpace(response.Status) ? "unknown" : response.Status,
            Error = response.Error,
            Reason = response.Reason,
            Hint = response.Hint,
            RequiresElevation = response.RequiresElevation,
            RequiresTaskSchedulerAccess = response.RequiresTaskSchedulerAccess,
        };
    }

    private static ServiceModeResult BuildEnabledResult(ServiceModeState state)
    {
        if (state.IsTaskFallback)
        {
            return new ServiceModeResult
            {
                Title = "未注册为 Windows 服务，已回退为计划任务",
                FallbackScheduled = true,
                Message = $"当前未能注册 Windows 服务，系统已回退为计划任务以维持后台运行。这并不等同于 Windows 服务模式；若要注册真正的 Windows 服务，请接受系统提权或使用管理员权限重新启用。数据目录：{AppPaths.ServiceBaseDirectory}",
            };
        }

        return new ServiceModeResult
        {
            Title = "已切换到 Windows 服务模式",
            ServiceStarted = true,
            Message = $"当前已注册为 Windows 服务，由 LocalService 账户接管后台运行。前台窗口与托盘可以关闭，后台仍会继续运行。数据目录：{AppPaths.ServiceBaseDirectory}",
        };
    }

    private static string ExtractProcessError(ProcessResult result, string fallback)
    {
        if (!string.IsNullOrWhiteSpace(result.StandardError))
        {
            return result.StandardError;
        }

        if (!string.IsNullOrWhiteSpace(result.StandardOutput))
        {
            return result.StandardOutput;
        }

        return fallback;
    }

    private static string ExtractError(ServiceCommandResponse response, string fallback)
    {
        if (!string.IsNullOrWhiteSpace(response.Error))
        {
            return response.Error;
        }

        return fallback;
    }

    private static bool ContainsKnownMessage(string text, string token)
        => !string.IsNullOrWhiteSpace(text) && text.Contains(token, StringComparison.OrdinalIgnoreCase);

    private static bool ShouldRetryElevated(string command, ServiceCommandResponse response)
    {
        if (response.Ok || !IsPrivilegedServiceCommand(command))
        {
            return false;
        }

        if (response.RequiresElevation)
        {
            return true;
        }

        return ContainsKnownMessage(response.Reason, "requires_elevation")
            || ContainsKnownMessage(response.Reason, "access_denied")
            || ContainsKnownMessage(response.Error, "Access is denied")
            || ContainsKnownMessage(response.Error, "拒绝访问")
            || ContainsKnownMessage(response.Error, "service_start_requires_elevation");
    }

    private static bool IsPrivilegedServiceCommand(string command)
        => command is "install" or "start" or "stop" or "uninstall";

    private static ServiceCommandResponse RunServiceCommandElevated(string cliPath, string command)
    {
        var jsonPath = Path.Combine(Path.GetTempPath(), $"clashforclaw-service-{command}-{Guid.NewGuid():N}.json");
        try
        {
            using var process = new Process();
            process.StartInfo = new ProcessStartInfo
            {
                FileName = cliPath,
                Arguments = $"--base-dir \"{AppPaths.ServiceBaseDirectory}\" service {command} --json-file \"{jsonPath}\"",
                UseShellExecute = true,
                Verb = "runas",
                WindowStyle = ProcessWindowStyle.Hidden,
            };

            process.Start();
            process.WaitForExit();

            if (File.Exists(jsonPath))
            {
                var output = File.ReadAllText(jsonPath);
                var parsed = TryParseServiceCommand(output);
                if (parsed is not null)
                {
                    return parsed;
                }

                return new ServiceCommandResponse
                {
                    Action = command,
                    Error = "elevated_service_command_response_invalid",
                };
            }

            return new ServiceCommandResponse
            {
                Action = command,
                Error = $"elevated_command_exit_code_{process.ExitCode}",
            };
        }
        catch (Win32Exception ex) when (ex.NativeErrorCode == 1223)
        {
            return new ServiceCommandResponse
            {
                Action = command,
                Error = "elevation_cancelled",
                Reason = "elevation_cancelled",
                Hint = command is "stop" or "uninstall"
                    ? "Accept the UAC prompt to disable Windows service mode."
                    : "Accept the UAC prompt to continue enabling Windows service mode.",
                RequiresElevation = true,
            };
        }
        catch (Exception ex)
        {
            return new ServiceCommandResponse
            {
                Action = command,
                Error = ex.Message,
            };
        }
        finally
        {
            try
            {
                if (File.Exists(jsonPath))
                {
                    File.Delete(jsonPath);
                }
            }
            catch
            {
            }
        }
    }

    public static ServiceModeResult WithDesktopFallback(ServiceModeResult result)
    {
        if (result.ServiceStarted || result.FallbackScheduled || result.DesktopFallbackStarted)
        {
            return result;
        }

        return new ServiceModeResult
        {
            Title = "未启用 Windows 服务模式，已回退为桌面后台",
            Message = $"{result.Message}\n\n当前已自动保留桌面后台，托盘、静默启动和关闭最小化到托盘仍可继续使用，不会中断现有连接。",
            DesktopFallbackStarted = true,
        };
    }

    private static string BuildEnableFailureMessage(ServiceCommandResponse result)
    {
        var detail = ExtractError(result, "注册 Windows 服务失败。");
        if (ContainsKnownMessage(result.Reason, "elevation_cancelled"))
        {
            return "已取消管理员授权，Windows 服务模式未启用。";
        }

        if (result.RequiresElevation
            || ContainsKnownMessage(result.Reason, "install_requires_elevation_or_task_scheduler_access")
            || ContainsKnownMessage(detail, "Access is denied")
            || ContainsKnownMessage(detail, "拒绝访问"))
        {
            return "当前操作需要管理员权限。应用会弹出系统提权窗口；如果取消 UAC，Windows 服务模式不会启用。";
        }

        if (!string.IsNullOrWhiteSpace(result.Hint))
        {
            return $"{detail}\n\n{result.Hint}";
        }

        return detail;
    }

    private static string BuildDisableFailureMessage(ServiceCommandResponse stop, ServiceCommandResponse uninstall)
    {
        var detail = ExtractError(uninstall, ExtractError(stop, "关闭后台托管模式失败。"));
        if (ContainsKnownMessage(stop.Reason, "elevation_cancelled") || ContainsKnownMessage(uninstall.Reason, "elevation_cancelled"))
        {
            return "已取消管理员授权，Windows 服务模式未关闭。";
        }

        if (stop.RequiresElevation
            || uninstall.RequiresElevation
            || ContainsKnownMessage(stop.Reason, "access_denied")
            || ContainsKnownMessage(uninstall.Reason, "access_denied")
            || ContainsKnownMessage(detail, "Access is denied")
            || ContainsKnownMessage(detail, "拒绝访问"))
        {
            return "当前操作需要管理员权限。应用会弹出系统提权窗口；如果取消 UAC，Windows 服务模式不会关闭。";
        }

        if (!string.IsNullOrWhiteSpace(uninstall.Hint))
        {
            return $"{detail}\n\n{uninstall.Hint}";
        }

        if (!string.IsNullOrWhiteSpace(stop.Hint))
        {
            return $"{detail}\n\n{stop.Hint}";
        }

        return detail;
    }
}
