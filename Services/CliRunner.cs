using System.Diagnostics;
using ClashForClaw.Models;

namespace ClashForClaw.Services;

public sealed class CliRunner
{
    private const int PortReleasePollMilliseconds = 150;

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

        return Start(cliPath, args, logDirectory, trackProcess: true);
    }

    public void Stop()
    {
        _ = StopTrackedProcess();
    }

    public async Task<bool> StopManagedBackendAsync(AppSettings settings, int port, TimeSpan timeout)
    {
        var deadline = DateTimeOffset.UtcNow + timeout;
        var trackedProcess = StopTrackedProcess();
        if (trackedProcess is not null)
        {
            try
            {
                await WaitForExitAsync(trackedProcess, Remaining(deadline));
            }
            catch
            {
                // Ignore wait failures and fall back to port ownership checks below.
            }
        }

        if (await WaitForPortReleaseAsync(port, deadline))
        {
            return true;
        }

        var cliPath = AppPaths.ResolveCliPath(settings.CliPath);
        await StopManagedPortOwnersAsync(cliPath, port, deadline);
        return await WaitForPortReleaseAsync(port, deadline);
    }

    private Process? StopTrackedProcess()
    {
        var trackedProcess = process;
        process = null;
        lastLaunchSignature = null;

        try
        {
            if (trackedProcess is { HasExited: false })
            {
                trackedProcess.Kill(true);
            }
        }
        catch
        {
            // Ignore stop failures.
        }

        return trackedProcess;
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

    private static async Task WaitForExitAsync(Process trackedProcess, TimeSpan timeout)
    {
        if (trackedProcess.HasExited || timeout <= TimeSpan.Zero)
        {
            return;
        }

        await trackedProcess.WaitForExitAsync().WaitAsync(timeout);
    }

    private static async Task<bool> WaitForPortReleaseAsync(int port, DateTimeOffset deadline)
    {
        while (DateTimeOffset.UtcNow < deadline)
        {
            if (!IsPortInUse(port))
            {
                return true;
            }

            await Task.Delay(PortReleasePollMilliseconds);
        }

        return !IsPortInUse(port);
    }

    private static async Task StopManagedPortOwnersAsync(string cliPath, int port, DateTimeOffset deadline)
    {
        foreach (var pid in GetListeningPortOwners(port))
        {
            if (!IsManagedCliProcess(pid, cliPath))
            {
                continue;
            }

            try
            {
                using var ownedProcess = Process.GetProcessById(pid);
                if (!ownedProcess.HasExited)
                {
                    ownedProcess.Kill(true);
                    await WaitForExitAsync(ownedProcess, Remaining(deadline));
                }
            }
            catch
            {
                // Ignore failures and let the final port release probe decide.
            }
        }
    }

    private static bool IsPortInUse(int port)
        => GetListeningPortOwners(port).Count > 0;

    private static IReadOnlyCollection<int> GetListeningPortOwners(int port)
    {
        if (port <= 0)
        {
            return Array.Empty<int>();
        }

        var startInfo = new ProcessStartInfo
        {
            FileName = "netstat",
            Arguments = "-ano -p tcp",
            CreateNoWindow = true,
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };

        try
        {
            using var netstat = Process.Start(startInfo);
            if (netstat is null)
            {
                return Array.Empty<int>();
            }

            var output = netstat.StandardOutput.ReadToEnd();
            netstat.WaitForExit();
            if (netstat.ExitCode != 0 || string.IsNullOrWhiteSpace(output))
            {
                return Array.Empty<int>();
            }

            var owners = new HashSet<int>();
            using var reader = new StringReader(output);
            string? line;
            while ((line = reader.ReadLine()) is not null)
            {
                if (TryParseListeningPortOwner(line, out var parsedPort, out var pid) && parsedPort == port)
                {
                    owners.Add(pid);
                }
            }

            return owners.Count == 0 ? Array.Empty<int>() : owners.ToArray();
        }
        catch
        {
            return Array.Empty<int>();
        }
    }

    private static bool TryParseListeningPortOwner(string line, out int port, out int pid)
    {
        port = 0;
        pid = 0;
        if (string.IsNullOrWhiteSpace(line))
        {
            return false;
        }

        var fields = line.Split((char[]?)null, StringSplitOptions.RemoveEmptyEntries);
        if (fields.Length < 5 || !string.Equals(fields[0], "TCP", StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        if (!string.Equals(fields[^2], "LISTENING", StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        if (!TryParsePort(fields[1], out port) || !int.TryParse(fields[^1], out pid))
        {
            port = 0;
            pid = 0;
            return false;
        }

        return pid > 0;
    }

    private static bool TryParsePort(string address, out int port)
    {
        port = 0;
        if (string.IsNullOrWhiteSpace(address))
        {
            return false;
        }

        var separatorIndex = address.LastIndexOf(':');
        if (separatorIndex < 0 || separatorIndex == address.Length - 1)
        {
            return false;
        }

        return int.TryParse(address[(separatorIndex + 1)..], out port) && port > 0;
    }

    private static bool IsManagedCliProcess(int pid, string cliPath)
    {
        try
        {
            using var ownedProcess = Process.GetProcessById(pid);
            var candidatePath = ownedProcess.MainModule?.FileName;
            if (!string.IsNullOrWhiteSpace(candidatePath))
            {
                return SamePath(candidatePath, cliPath);
            }

            return string.Equals(ownedProcess.ProcessName, Path.GetFileNameWithoutExtension(cliPath), StringComparison.OrdinalIgnoreCase);
        }
        catch
        {
            return false;
        }
    }

    private static bool SamePath(string left, string right)
        => string.Equals(Path.GetFullPath(left), Path.GetFullPath(right), StringComparison.OrdinalIgnoreCase);

    private static TimeSpan Remaining(DateTimeOffset deadline)
    {
        var remaining = deadline - DateTimeOffset.UtcNow;
        return remaining <= TimeSpan.Zero ? TimeSpan.Zero : remaining;
    }
}
