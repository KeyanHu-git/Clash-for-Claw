using System;
using System.IO;
using System.Text.Json;
using ClashForClaw.Models;

namespace ClashForClaw.Services;

public sealed class ServiceModeState
{
    public string Mode { get; init; } = "none";
    public string Status { get; init; } = "unknown";

    public bool IsEnabled
        => !string.Equals(Mode, "none", StringComparison.OrdinalIgnoreCase);

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
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    private sealed class ServiceCommandResponse
    {
        public bool Ok { get; init; }
        public string Action { get; init; } = string.Empty;
        public string Mode { get; init; } = string.Empty;
        public string Status { get; init; } = string.Empty;
        public string Error { get; init; } = string.Empty;
    }

    public static ServiceModeState Query(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        if (!File.Exists(cliPath))
        {
            return new ServiceModeState();
        }

        var status = RunServiceCommand(cliPath, "status");
        if (!status.Ok)
        {
            return new ServiceModeState();
        }

        return ParseState(status);
    }

    public static ServiceModeResult Enable(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        if (!File.Exists(cliPath))
        {
            return new ServiceModeResult
            {
                Failed = true,
                Title = "未找到后台组件",
                Message = "未找到内置后台服务，暂时无法切换到服务模式。",
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
                Title = "服务模式未启用",
                Message = $"准备服务模式运行时失败：{ex.Message}",
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
                    Title = "服务模式未启用",
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
                Title = "服务模式未启用",
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
            Title = "服务模式未启用",
            Message = "后台服务没有进入预期状态。",
        };
    }

    public static ServiceModeResult Disable(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        if (!File.Exists(cliPath))
        {
            return new ServiceModeResult
            {
                Title = "服务模式已关闭",
                Message = "未找到内置后台组件，已按桌面后台模式处理。",
            };
        }

        var current = Query(settings);
        if (!current.IsEnabled)
        {
            return new ServiceModeResult
            {
                Title = "服务模式已关闭",
                Message = "当前没有已注册的服务模式，已按桌面后台模式处理。",
            };
        }

        _ = RunServiceCommand(cliPath, "stop");
        var uninstall = RunServiceCommand(cliPath, "uninstall");
        var actual = Query(settings);

        if (actual.IsEnabled)
        {
            return new ServiceModeResult
            {
                Failed = true,
                Title = "关闭服务模式失败",
                Message = ExtractError(uninstall, "后台服务未能卸载，请检查当前账户是否具备管理权限。"),
            };
        }

        return new ServiceModeResult
        {
            Title = "已恢复到桌面后台",
            Message = "后台已切回桌面模式，现在可以继续使用托盘、静默启动和关闭最小化到托盘。",
        };
    }

    private static ProcessResult RunCli(string cliPath, string command)
    {
        var args = $"--base-dir \"{AppPaths.ServiceBaseDirectory}\" service {command} --json";
        return ProcessRunner.Run(cliPath, args);
    }

    private static ServiceCommandResponse RunServiceCommand(string cliPath, string command)
        => ParseServiceCommand(RunCli(cliPath, command), command);

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
            return JsonSerializer.Deserialize<ServiceCommandResponse>(output, JsonOptions);
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
        };
    }

    private static ServiceModeResult BuildEnabledResult(ServiceModeState state)
    {
        var isTaskFallback = state.IsTaskFallback;
        return new ServiceModeResult
        {
            Title = isTaskFallback ? "已切换到静默后台（计划任务）" : "已切换到静默后台（Windows 服务）",
            ServiceStarted = !isTaskFallback,
            FallbackScheduled = isTaskFallback,
            Message = isTaskFallback
                ? $"已使用计划任务接管后台驻留。当前窗口与托盘可以关闭，后台会在登录后静默运行。数据目录：{AppPaths.ServiceBaseDirectory}"
                : $"已使用低权限 LocalService 后台服务接管运行。当前窗口与托盘可以关闭，后台会持续静默运行。数据目录：{AppPaths.ServiceBaseDirectory}",
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
        => text.Contains(token, StringComparison.OrdinalIgnoreCase);

    public static ServiceModeResult WithDesktopFallback(ServiceModeResult result)
    {
        if (result.ServiceStarted || result.FallbackScheduled || result.DesktopFallbackStarted)
        {
            return result;
        }

        return new ServiceModeResult
        {
            Title = "未切换到服务模式，已回退桌面后台",
            Message = $"{result.Message}\n\n已自动保留桌面后台，你可以继续使用托盘、静默启动和关闭最小化到托盘，不会中断当前连接。",
            DesktopFallbackStarted = true,
        };
    }

    private static string BuildEnableFailureMessage(ServiceCommandResponse result)
    {
        var detail = ExtractError(result, "注册后台服务失败。");
        if (ContainsKnownMessage(detail, "install_requires_elevation_or_task_scheduler_access")
            || ContainsKnownMessage(detail, "Access is denied"))
        {
            return "当前会话没有提升权限，暂时无法注册 Windows 服务或计划任务。可以右键“以管理员身份运行”后再次启用；这次会先自动保留桌面后台，保证代理链路不中断。";
        }

        return detail;
    }
}
