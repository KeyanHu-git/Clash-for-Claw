using System;
using System.Diagnostics;
using System.IO;

namespace OpenClawAdapter.Services;

public static class AppPaths
{
    public static string ServiceBaseDirectory
        => Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData),
            "OpenClawAdapter");

    public static string AppExecutablePath
        => Environment.ProcessPath
            ?? Process.GetCurrentProcess().MainModule?.FileName
            ?? "OpenClawAdapter.exe";

    public static string AppDirectory
        => Path.GetDirectoryName(AppExecutablePath) ?? AppContext.BaseDirectory;

    public static string ServiceLogPath
        => Path.Combine(ServiceBaseDirectory, "logs", "adapter.log");

    public static string ResolveCliPath(string path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return Path.Combine(AppDirectory, "OpenClaw-Adapter.exe");
        }

        return Path.IsPathRooted(path)
            ? path
            : Path.Combine(AppDirectory, path);
    }

    public static string? FindDefaultCliPath()
    {
        var appDir = AppDirectory;
        var candidates = new[]
        {
            Path.Combine(appDir, "OpenClaw-Adapter.exe"),
            Path.Combine(appDir, "openclaw-adapter.exe"),
            Path.Combine(appDir, "openclaw-adapter-cli.exe"),
        };

        foreach (var candidate in candidates)
        {
            if (File.Exists(candidate))
            {
                return candidate;
            }
        }

        var current = new DirectoryInfo(appDir);
        for (var i = 0; i < 8 && current is not null; i++)
        {
            var sibling = Path.Combine(current.FullName, "openclaw-adapter");
            var siblingCandidates = new[]
            {
                Path.Combine(sibling, "OpenClaw-Adapter.exe"),
                Path.Combine(sibling, "openclaw-adapter.exe"),
            };

            foreach (var candidate in siblingCandidates)
            {
                if (File.Exists(candidate))
                {
                    return candidate;
                }
            }

            current = current.Parent;
        }

        return null;
    }
}




