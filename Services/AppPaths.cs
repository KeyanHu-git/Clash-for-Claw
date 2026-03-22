using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;

namespace ClashForClaw.Services;

public static class AppPaths
{
    private const string ProductFolderName = "ClashForClaw";
    private const string LegacyProductFolderName = "OpenClawAdapter";
    private const string BackendExecutableName = "ClashForClaw.Service.exe";
    private const string MihomoExecutableName = "mihomo.exe";
    private const long MinimumMihomoSizeBytes = 1024 * 1024;
    private static readonly Lazy<string> UserDataDirectoryValue = new(() => Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), ProductFolderName));
    private static readonly Lazy<string> ServiceBaseDirectoryValue = new(() => Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), ProductFolderName));

    public static string ServiceBaseDirectory => ServiceBaseDirectoryValue.Value;

    public static string UserDataDirectory => UserDataDirectoryValue.Value;

    public static string UserMihomoPath
        => Path.Combine(UserDataDirectory, "bin", MihomoExecutableName);

    public static string ServiceMihomoPath
        => Path.Combine(ServiceBaseDirectory, "bin", MihomoExecutableName);

    public static string AppExecutablePath
        => Environment.ProcessPath
            ?? Process.GetCurrentProcess().MainModule?.FileName
            ?? "ClashForClaw.exe";

    public static string AppDirectory
        => Path.GetDirectoryName(AppExecutablePath) ?? AppContext.BaseDirectory;

    public static string ServiceLogPath
        => Path.Combine(ServiceBaseDirectory, "logs", "service.log");

    public static string ResolveCliPath(string path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return Path.Combine(AppDirectory, BackendExecutableName);
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
            Path.Combine(appDir, BackendExecutableName),
            Path.Combine(appDir, "ClashForClaw.Service.win-x64.exe"),
            Path.Combine(appDir, "ClashForClaw.Service.win-arm64.exe"),
            Path.Combine(appDir, "ClashForClaw.Service.win-x86.exe"),
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
            var repoCandidates = new[]
            {
                Path.Combine(current.FullName, "backend", "ClashForClaw.Service", "bin", BackendExecutableName),
                Path.Combine(current.FullName, "artifacts", "release", BackendExecutableName),
            };

            foreach (var candidate in repoCandidates)
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

    public static string? FindAvailableMihomoPath(string? cliPath = null)
    {
        foreach (var candidate in EnumerateMihomoCandidates(cliPath))
        {
            if (IsUsableMihomo(candidate))
            {
                return candidate;
            }
        }

        return null;
    }

    public static void EnsureServiceMihomoAvailable(string? cliPath = null)
    {
        var source = FindAvailableMihomoPath(cliPath);
        if (string.IsNullOrWhiteSpace(source) || IsUsableMihomo(ServiceMihomoPath))
        {
            return;
        }

        var serviceBinDir = Path.GetDirectoryName(ServiceMihomoPath);
        if (string.IsNullOrWhiteSpace(serviceBinDir))
        {
            return;
        }

        Directory.CreateDirectory(serviceBinDir);
        if (!SamePath(source, ServiceMihomoPath))
        {
            File.Copy(source, ServiceMihomoPath, overwrite: true);
        }
    }

    public static void PurgeLegacyData()
    {
        try
        {
            PurgeLegacyDirectory(Environment.SpecialFolder.ApplicationData);
        }
        catch
        {
        }

        try
        {
            PurgeLegacyDirectory(Environment.SpecialFolder.CommonApplicationData);
        }
        catch
        {
        }
    }

    private static void PurgeLegacyDirectory(Environment.SpecialFolder folder)
    {
        var root = Environment.GetFolderPath(folder);
        var legacy = Path.Combine(root, LegacyProductFolderName);
        if (Directory.Exists(legacy))
        {
            Directory.Delete(legacy, recursive: true);
        }
    }

    private static IEnumerable<string> EnumerateMihomoCandidates(string? cliPath)
    {
        var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        foreach (var candidate in new[]
        {
            UserMihomoPath,
            ServiceMihomoPath,
            CombineIfPresent(Path.GetDirectoryName(cliPath), "bin", MihomoExecutableName),
            CombineIfPresent(Path.GetDirectoryName(cliPath), MihomoExecutableName),
            Path.Combine(AppDirectory, "bin", MihomoExecutableName),
            Path.Combine(AppDirectory, MihomoExecutableName),
        })
        {
            if (string.IsNullOrWhiteSpace(candidate))
            {
                continue;
            }

            if (seen.Add(candidate))
            {
                yield return candidate;
            }
        }
    }

    private static string? CombineIfPresent(string? basePath, params string[] parts)
    {
        if (string.IsNullOrWhiteSpace(basePath))
        {
            return null;
        }

        var combined = basePath;
        foreach (var part in parts)
        {
            combined = Path.Combine(combined, part);
        }
        return combined;
    }

    private static bool IsUsableMihomo(string path)
    {
        try
        {
            var info = new FileInfo(path);
            return info.Exists && info.Length > MinimumMihomoSizeBytes;
        }
        catch
        {
            return false;
        }
    }

    private static bool SamePath(string left, string right)
        => string.Equals(Path.GetFullPath(left), Path.GetFullPath(right), StringComparison.OrdinalIgnoreCase);
}
