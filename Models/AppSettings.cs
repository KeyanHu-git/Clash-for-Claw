namespace OpenClawAdapter.Models;

public sealed class AppSettings
{
    public bool AutoStartEnabled { get; set; } = true;
    public bool SilentOnBootEnabled { get; set; }
    public bool CloseToTrayEnabled { get; set; } = true;
    public bool ServiceModeEnabled { get; set; }
    public bool AutoRunCliEnabled { get; set; } = true;
    public string CliPath { get; set; } = "OpenClaw-Adapter.exe";
    public string CliArgs { get; set; } = "--daemon";
    public bool DebugLoggingEnabled { get; set; }
    public string ThemeMode { get; set; } = "Dark";
    public int SubscriptionColumns { get; set; } = 2;
}
