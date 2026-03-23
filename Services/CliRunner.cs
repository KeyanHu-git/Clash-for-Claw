using System.Diagnostics;
using ClashForClaw.Models;

namespace ClashForClaw.Services;

public sealed class CliRunner
{
    private Process? process;
    private string? lastLaunchSignature;

    public bool EnsureRunning(AppSettings settings, bool force = false)
    {
        if (!settings.AutoRunCliEnabled && !force)
        {
            Stop();
            return false;
        }

        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        var args = settings.CliArgs ?? string.Empty;
        var logDirectory = AppPaths.ResolveDesktopLogDirectory(settings.LogDirectory);
        var launchSignature = BuildLaunchSignature(cliPath, args, logDirectory);

        if (process is not null && !process.HasExited && string.Equals(lastLaunchSignature, launchSignature, StringComparison.Ordinal))
        {
            return true;
        }

        Stop();
        return Start(cliPath, args, logDirectory, trackProcess: true);
    }

    public bool TryStartOnDemand(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        var args = settings.CliArgs ?? string.Empty;
        var logDirectory = AppPaths.ResolveDesktopLogDirectory(settings.LogDirectory);
        var launchSignature = BuildLaunchSignature(cliPath, args, logDirectory);

        if (process is not null
            && !process.HasExited
            && string.Equals(lastLaunchSignature, launchSignature, StringComparison.Ordinal))
        {
            return true;
        }

        return Start(cliPath, args, logDirectory, trackProcess: false);
    }

    public void Stop()
    {
        try
        {
            if (process is { HasExited: false })
            {
                process.Kill(true);
            }
        }
        catch
        {
            // Ignore stop failures.
        }

        process = null;
        lastLaunchSignature = null;
    }

    private static string BuildLaunchSignature(string cliPath, string args, string logDirectory)
        => $"\"{cliPath}\" {args} | logdir=\"{logDirectory}\"".Trim();

    private bool Start(string cliPath, string args, string logDirectory, bool trackProcess)
    {
        if (!File.Exists(cliPath))
        {
            return false;
        }

        var startInfo = new ProcessStartInfo
        {
            FileName = cliPath,
            Arguments = args,
            WorkingDirectory = Path.GetDirectoryName(cliPath) ?? AppPaths.AppDirectory,
            CreateNoWindow = true,
            UseShellExecute = false,
            WindowStyle = ProcessWindowStyle.Hidden,
        };
        startInfo.Environment[AppPaths.LogDirectoryEnvVar] = logDirectory;

        try
        {
            var started = Process.Start(startInfo);
            if (trackProcess)
            {
                process = started;
                lastLaunchSignature = BuildLaunchSignature(cliPath, args, logDirectory);
            }
            return started is not null;
        }
        catch
        {
            if (trackProcess)
            {
                process = null;
                lastLaunchSignature = null;
            }
            return false;
        }
    }
}


