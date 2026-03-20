using System;
using System.IO;
using OpenClawAdapter.Models;

namespace OpenClawAdapter.Services;

public sealed class ServiceModeResult
{
    public bool ServiceStarted { get; init; }
    public bool FallbackScheduled { get; init; }
    public string Message { get; init; } = string.Empty;
    public string Title { get; init; } = string.Empty;
}

public static class ServiceModeManager
{
    public static ServiceModeResult Enable(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        if (!File.Exists(cliPath))
        {
            return new ServiceModeResult
            {
                Title = "找不到后台核心",
                Message = "未找到 OpenClaw-Adapter.exe，无法切换到服务模式。",
            };
        }

        var status = RunCli(cliPath, "service status");
        var installMode = ParseMode(status.StandardOutput);
        if (status.ExitCode != 0 || string.IsNullOrWhiteSpace(installMode))
        {
            var install = RunCli(cliPath, "service install");
            if (install.ExitCode != 0 && !LooksAlreadyInstalled(install))
            {
                return new ServiceModeResult
                {
                    Title = "服务模式未启用",
                    Message = ExtractError(install, "注册后台服务失败，请以管理员身份重试。"),
                };
            }

            installMode = ParseInstalledMode(install.StandardOutput);
        }

        var start = RunCli(cliPath, "service start");
        if (start.ExitCode != 0 && !LooksAlreadyRunning(start))
        {
            return new ServiceModeResult
            {
                Title = "服务模式未启用",
                Message = ExtractError(start, "后台服务未能启动，请确认当前账户具备安装服务权限。"),
            };
        }

        var isTaskFallback = string.Equals(installMode, "task", StringComparison.OrdinalIgnoreCase);
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

    public static ServiceModeResult Disable(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        if (!File.Exists(cliPath))
        {
            return new ServiceModeResult
            {
                Title = "服务模式已关闭",
                Message = "后台服务入口不存在，已按桌面后台模式处理。",
            };
        }

        var stop = RunCli(cliPath, "service stop");
        var uninstall = RunCli(cliPath, "service uninstall");
        var success = uninstall.ExitCode == 0
            || ContainsKnownMessage(uninstall.StandardOutput, "uninstalled")
            || ContainsKnownMessage(uninstall.StandardError, "is not installed");

        if (!success)
        {
            return new ServiceModeResult
            {
                Title = "关闭服务模式失败",
                Message = ExtractError(uninstall, "后台服务未能卸载，请确认当前账户具备管理员权限。"),
            };
        }

        _ = stop;
        return new ServiceModeResult
        {
            Title = "已恢复到桌面后台",
            Message = "后台已切回桌面模式。现在可以继续使用托盘、静默启动和关闭最小化到托盘。",
        };
    }

    private static ProcessResult RunCli(string cliPath, string command)
    {
        var args = $"--base-dir \"{AppPaths.ServiceBaseDirectory}\" {command}";
        return ProcessRunner.Run(cliPath, args);
    }

    private static string ParseInstalledMode(string output)
    {
        if (ContainsKnownMessage(output, "installed: task"))
        {
            return "task";
        }
        if (ContainsKnownMessage(output, "installed: service"))
        {
            return "service";
        }
        return string.Empty;
    }

    private static string ParseMode(string output)
    {
        if (ContainsKnownMessage(output, "mode: task"))
        {
            return "task";
        }
        if (ContainsKnownMessage(output, "mode: service"))
        {
            return "service";
        }
        return string.Empty;
    }

    private static string ExtractError(ProcessResult result, string fallback)
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

    private static bool ContainsKnownMessage(string text, string token)
        => text.Contains(token, StringComparison.OrdinalIgnoreCase);

    private static bool LooksAlreadyInstalled(ProcessResult result)
        => ContainsKnownMessage(result.StandardOutput, "already exists")
            || ContainsKnownMessage(result.StandardError, "already exists");

    private static bool LooksAlreadyRunning(ProcessResult result)
        => ContainsKnownMessage(result.StandardOutput, "already been started")
            || ContainsKnownMessage(result.StandardError, "already been started")
            || ContainsKnownMessage(result.StandardOutput, "already running")
            || ContainsKnownMessage(result.StandardError, "already running");
}
