using System;
using System.IO;
using System.Text.Json;
using ClashForClaw.Models;

namespace ClashForClaw.Services;

public static class SettingsStore
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        WriteIndented = true,
    };

    public static AppSettings Current { get; private set; } = new();

    public static event EventHandler? Changed;

    public static string SettingsPath => Path.Combine(AppPaths.UserDataDirectory, "settings.json");

    public static void Load()
    {
        AppPaths.PurgeLegacyData();

        try
        {
            if (File.Exists(SettingsPath))
            {
                var json = File.ReadAllText(SettingsPath);
                var loaded = JsonSerializer.Deserialize<AppSettings>(json);
                if (loaded is not null)
                {
                    Current = loaded;
                }
            }
        }
        catch
        {
            Current = new AppSettings();
        }

        EnsureDefaults();
        Save();
    }

    public static void Update(Action<AppSettings> apply)
    {
        apply(Current);
        Save();
        Changed?.Invoke(null, EventArgs.Empty);
    }

    private static void EnsureDefaults()
    {
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
            Current.CliArgs = "--daemon";
        }

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
            ? "Dark"
            : Current.ThemeMode;

        if (Current.SubscriptionColumns <= 0)
        {
            Current.SubscriptionColumns = 2;
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
        var dir = Path.GetDirectoryName(SettingsPath);
        if (!string.IsNullOrWhiteSpace(dir))
        {
            Directory.CreateDirectory(dir);
        }

        var json = JsonSerializer.Serialize(Current, JsonOptions);
        File.WriteAllText(SettingsPath, json);
    }
}


