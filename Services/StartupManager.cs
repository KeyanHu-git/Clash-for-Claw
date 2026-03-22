using Microsoft.Win32;
using ClashForClaw.Models;

namespace ClashForClaw.Services;

public static class StartupManager
{
    private const string RunKeyPath = "Software\\Microsoft\\Windows\\CurrentVersion\\Run";
    private const string AppRunName = "ClashForClaw";
    private const string LegacyAppRunName = "OpenClawAdapter";

    public static void ApplyAutoStart(AppSettings settings, bool serviceModeEnabled)
    {
        try
        {
            using var key = Registry.CurrentUser.CreateSubKey(RunKeyPath, true);
            if (key is null)
            {
                return;
            }

            key.DeleteValue(LegacyAppRunName, false);

            if (settings.AutoStartEnabled && !serviceModeEnabled)
            {
                var silentArg = settings.SilentOnBootEnabled ? " --silent" : string.Empty;
                var value = $"\"{AppPaths.AppExecutablePath}\"{silentArg}";
                key.SetValue(AppRunName, value);
            }
            else
            {
                key.DeleteValue(AppRunName, false);
            }
        }
        catch
        {
            // Ignore registry failures; UI will reflect state but autostart might not apply.
        }
    }
}

