using System.Diagnostics;
using OpenClawAdapter.Models;

namespace OpenClawAdapter.Services;

public sealed class CliRunner
{
    private Process? process;
    private string? lastCommand;

    public void EnsureRunning(AppSettings settings)
    {
        if (!settings.AutoRunCliEnabled)
        {
            Stop();
            return;
        }

        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        var args = settings.CliArgs ?? string.Empty;
        var command = $"\"{cliPath}\" {args}".Trim();

        if (process is not null && !process.HasExited && string.Equals(lastCommand, command, StringComparison.Ordinal))
        {
            return;
        }

        Stop();
        Start(cliPath, args, trackProcess: true);
    }

    public bool TryStartOnDemand(AppSettings settings)
    {
        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        var args = settings.CliArgs ?? string.Empty;
        var command = BuildCommand(cliPath, args);

        if (process is not null
            && !process.HasExited
            && string.Equals(lastCommand, command, StringComparison.Ordinal))
        {
            return true;
        }

        return Start(cliPath, args, trackProcess: false);
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
        lastCommand = null;
    }

    private static string BuildCommand(string cliPath, string args)
        => $"\"{cliPath}\" {args}".Trim();

    private bool Start(string cliPath, string args, bool trackProcess)
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

        try
        {
            var started = Process.Start(startInfo);
            if (trackProcess)
            {
                process = started;
                lastCommand = BuildCommand(cliPath, args);
            }
            return started is not null;
        }
        catch
        {
            if (trackProcess)
            {
                process = null;
                lastCommand = null;
            }
            return false;
        }
    }
}

