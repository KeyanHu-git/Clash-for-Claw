using System;
using System.IO;
using System.Text.Json;
using ClashForClaw;
using ClashForClaw.Models;

namespace ClashForClaw.Services;

public static class SettingsStore
{
    private const string BrokenSettingsSuffix = ".broken";

    public static AppSettings Current { get; private set; } = new();

    public static event EventHandler? Changed;

    public static string SettingsPath => Path.Combine(AppPaths.UserDataDirectory, "settings.json");

    public static void Load()
    {
        AppPaths.PurgeLegacyData();
        var loadedFromDisk = false;

        try
        {
            if (File.Exists(SettingsPath))
            {
                var json = File.ReadAllText(SettingsPath);
                var loaded = JsonSerializer.Deserialize(json, AppJsonContext.Default.AppSettings);
                if (loaded is not null)
                {
                    Current = loaded;
                    loadedFromDisk = true;
                }
            }
        }
        catch (Exception ex)
        {
            BackupBrokenSettings(ex);
            Current = new AppSettings();
        }

        EnsureDefaults(loadedFromDisk);
        Save();
    }

    public static void Update(Action<AppSettings> apply)
    {
        apply(Current);
        Save();
        Changed?.Invoke(null, EventArgs.Empty);
    }

    private static void EnsureDefaults(bool loadedFromDisk)
    {
        if (!loadedFromDisk)
        {
            var startupState = StartupManager.ReadAutoStartState();
            Current.AutoStartEnabled = startupState.Enabled;
            Current.SilentOnBootEnabled = startupState.Enabled && startupState.Silent;
        }

        if (string.IsNullOrWhiteSpace(Current.CliPath))
        {
            Current.CliPath = string.Empty;
        }

        if (IsLegacyCliPath(Current.CliPath))
        {
            Current.CliPath = string.Empty;
        }

        if (string.IsNullOrWhiteSpace(Current.CliArgs)
            || string.Equals(Current.CliArgs, "--mode adapter", StringComparison.OrdinalIgnoreCase))
        {
            Current.CliArgs = AppDefaults.DefaultCliArguments;
        }

        Current.LogDirectory = AppPaths.NormalizeLogDirectorySetting(Current.LogDirectory);

        var resolved = AppPaths.ResolveCliPath(Current.CliPath);
        if (!File.Exists(resolved))
        {
            var detected = AppPaths.FindDefaultCliPath();
            if (!string.IsNullOrWhiteSpace(detected))
            {
                Current.CliPath = detected;
            }
        }

        Current.ThemeMode = string.IsNullOrWhiteSpace(Current.ThemeMode)
            ? AppDefaults.DefaultThemeMode
            : Current.ThemeMode;

        if (Current.SubscriptionColumns <= 0)
        {
            Current.SubscriptionColumns = AppDefaults.DefaultSubscriptionColumns;
        }
        if (Current.SubscriptionColumns > 3)
        {
            Current.SubscriptionColumns = 3;
        }
    }

    private static bool IsLegacyCliPath(string? path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return false;
        }

        return path.Contains("OpenClaw-Adapter", StringComparison.OrdinalIgnoreCase)
            || path.Contains("OpenClawAdapter", StringComparison.OrdinalIgnoreCase)
            || path.Contains("openclaw-adapter", StringComparison.OrdinalIgnoreCase);
    }

    private static void Save()
    {
        try
        {
            var dir = Path.GetDirectoryName(SettingsPath);
            if (!string.IsNullOrWhiteSpace(dir))
            {
                Directory.CreateDirectory(dir);
            }

            var json = JsonSerializer.Serialize(Current, AppJsonIndentedContext.Default.AppSettings);
            var tempPath = SettingsPath + ".tmp";
            File.WriteAllText(tempPath, json);

            if (File.Exists(SettingsPath))
            {
                File.Replace(tempPath, SettingsPath, destinationBackupFileName: null);
            }
            else
            {
                File.Move(tempPath, SettingsPath);
            }
        }
        catch (Exception ex)
        {
            LogSettingsIssue("保存设置失败", ex);
        }
    }

    private static void BackupBrokenSettings(Exception ex)
    {
        try
        {
            if (!File.Exists(SettingsPath))
            {
                LogSettingsIssue("读取设置失败", ex);
                return;
            }

            var backupPath = SettingsPath + BrokenSettingsSuffix;
            File.Copy(SettingsPath, backupPath, overwrite: true);
            LogSettingsIssue($"读取设置失败，已备份到 {backupPath}", ex);
        }
        catch (Exception backupEx)
        {
            LogSettingsIssue("读取设置失败，且备份损坏设置文件时出错", backupEx);
            LogSettingsIssue("原始设置读取异常", ex);
        }
    }

    private static void LogSettingsIssue(string message, Exception ex)
    {
        try
        {
            var logPath = AppPaths.GetSettingsLogPath(Current.LogDirectory);
            var directory = Path.GetDirectoryName(logPath);
            if (!string.IsNullOrWhiteSpace(directory))
            {
                Directory.CreateDirectory(directory);
            }
            var entry = $"{DateTimeOffset.Now:u} {message}{Environment.NewLine}{ex}{Environment.NewLine}{Environment.NewLine}";
            File.AppendAllText(logPath, entry);
        }
        catch
        {
            // Avoid crashing when diagnostics cannot be written.
        }
    }
}


