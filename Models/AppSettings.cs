namespace ClashForClaw.Models;

public sealed class AppSettings
{
    public bool AutoStartEnabled { get; set; }
    public bool SilentOnBootEnabled { get; set; }
    public bool CloseToTrayEnabled { get; set; } = true;
    public bool AutoRunCliEnabled { get; set; } = true;
    public string CliPath { get; set; } = "ClashForClaw.Service.exe";
    public string CliArgs { get; set; } = "--daemon";
    public string LogDirectory { get; set; } = string.Empty;
    public bool DebugLoggingEnabled { get; set; }
    public string ThemeMode { get; set; } = "Dark";
    public int SubscriptionColumns { get; set; } = 2;
}

