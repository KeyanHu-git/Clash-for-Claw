using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;

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
    public const string LogDirectoryEnvVar = "CLASH_FOR_CLAW_LOG_DIR";

    public static string ServiceBaseDirectory => ServiceBaseDirectoryValue.Value;

    public static string UserDataDirectory => UserDataDirectoryValue.Value;

    public static string DefaultDesktopLogDirectory
        => Path.Combine(UserDataDirectory, "logs");

    public static string DefaultServiceLogDirectory
        => Path.Combine(ServiceBaseDirectory, "logs");

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
        => Path.Combine(DefaultServiceLogDirectory, "service.log");

    public static string ResolveDesktopLogDirectory(string? configuredDirectory = null)
    {
        if (string.IsNullOrWhiteSpace(configuredDirectory))
        {
            return DefaultDesktopLogDirectory;
        }

        try
        {
            var expanded = Environment.ExpandEnvironmentVariables(configuredDirectory.Trim());
            var fullPath = Path.GetFullPath(expanded);
            return string.IsNullOrWhiteSpace(fullPath)
                ? DefaultDesktopLogDirectory
                : fullPath;
        }
        catch
        {
            return DefaultDesktopLogDirectory;
        }
    }

    public static string NormalizeLogDirectorySetting(string? configuredDirectory)
    {
        var resolved = ResolveDesktopLogDirectory(configuredDirectory);
        return SamePath(resolved, DefaultDesktopLogDirectory)
            ? string.Empty
            : resolved;
    }

    public static string GetDesktopServiceLogPath(string? configuredDirectory = null)
        => Path.Combine(ResolveDesktopLogDirectory(configuredDirectory), "service.log");

    public static string GetSettingsLogPath(string? configuredDirectory = null)
        => Path.Combine(ResolveDesktopLogDirectory(configuredDirectory), "settings.log");

    public static string GetCrashLogPath(string? configuredDirectory = null)
        => Path.Combine(ResolveDesktopLogDirectory(configuredDirectory), "crash.log");

    public static bool IsDefaultDesktopLogDirectory(string? configuredDirectory = null)
        => string.IsNullOrWhiteSpace(NormalizeLogDirectorySetting(configuredDirectory));

    public static bool TryEnsureWritableDirectory(string directoryPath, out string errorMessage)
    {
        errorMessage = string.Empty;

        try
        {
            Directory.CreateDirectory(directoryPath);
            var probePath = Path.Combine(directoryPath, $".clashforclaw-write-test-{Guid.NewGuid():N}.tmp");
            using var stream = new FileStream(probePath, FileMode.CreateNew, FileAccess.Write, FileShare.None, 1, FileOptions.DeleteOnClose);
            stream.WriteByte(0);
            return true;
        }
        catch (Exception ex)
        {
            errorMessage = ex.Message;
            return false;
        }
    }

    public static string ResolveCliPath(string path)
    {
        var candidate = string.IsNullOrWhiteSpace(path)
            ? Path.Combine(AppDirectory, BackendExecutableName)
            : Path.IsPathRooted(path)
                ? path
                : Path.Combine(AppDirectory, path);

        if (File.Exists(candidate))
        {
            return candidate;
        }

        var detected = FindDefaultCliPath();
        if (!string.IsNullOrWhiteSpace(detected))
        {
            return detected;
        }

        return candidate;
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
                Path.Combine(current.FullName, "backend", "ClashForClaw.Service", "artifacts", "tmp", BackendExecutableName),
                Path.Combine(current.FullName, "artifacts", "release", BackendExecutableName),
                Path.Combine(current.FullName, "artifacts", "fi-ohm-runtime", BackendExecutableName),
                Path.Combine(current.FullName, "artifacts", "reliability-runtime", BackendExecutableName),
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
        foreach (var candidate in EnumerateMihomoCandidateValues(cliPath))
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

    private static IEnumerable<string?> EnumerateMihomoCandidateValues(string? cliPath)
    {
        yield return UserMihomoPath;
        yield return ServiceMihomoPath;
        yield return CombineIfPresent(Path.GetDirectoryName(cliPath), "bin", MihomoExecutableName);
        yield return CombineIfPresent(Path.GetDirectoryName(cliPath), MihomoExecutableName);
        yield return Path.Combine(AppDirectory, "bin", MihomoExecutableName);
        yield return Path.Combine(AppDirectory, MihomoExecutableName);

        var current = new DirectoryInfo(AppDirectory);
        for (var i = 0; i < 8 && current is not null; i++)
        {
            yield return ResolveBundledMihomoAsset(current.FullName);
            current = current.Parent;
        }
    }

    private static string? ResolveBundledMihomoAsset(string root)
    {
        var arch = RuntimeInformation.ProcessArchitecture switch
        {
            Architecture.X64 => "amd64",
            Architecture.Arm64 => "arm64",
            _ => string.Empty,
        };

        if (string.IsNullOrWhiteSpace(arch))
        {
            return null;
        }

        return Path.Combine(root, "backend", "ClashForClaw.Service", "internal", "mihomo", "assets", "windows", arch, MihomoExecutableName);
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
