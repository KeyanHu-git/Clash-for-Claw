using Microsoft.Win32;
using OpenClawAdapter.Models;

namespace OpenClawAdapter.Services;

public static class StartupManager
{
    private const string RunKeyPath = "Software\\Microsoft\\Windows\\CurrentVersion\\Run";
    private const string AppRunName = "OpenClawAdapter";

    public static void ApplyAutoStart(AppSettings settings)
    {
        try
        {
            using var key = Registry.CurrentUser.CreateSubKey(RunKeyPath, true);
            if (key is null)
            {
                return;
            }

            if (settings.AutoStartEnabled && !settings.ServiceModeEnabled)
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