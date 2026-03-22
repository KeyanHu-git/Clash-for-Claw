using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using ClashForClaw.Models;
using ClashForClaw.Services;

namespace ClashForClaw.ViewModels;

public enum StatusLevel
{
    Unknown,
    Ok,
    Warning,
    Error,
}

public sealed class MainViewModel : INotifyPropertyChanged
{
    private string gatewayUrl = "http://127.0.0.1:18789";
    private string gatewayToken = string.Empty;
    private bool gatewayTokenConfigured;
    private string subscriptionUrl = string.Empty;
    private string localPort = "7890";
    private bool isSubscriptionMode = true;
    private string modeHint = "订阅优先，失败自动回退本地端口。";
    private int subscriptionRefreshHours = 6;
    private int subscriptionProbeMinutes = 60;
    private int subscriptionColumns = 2;

    private string connectionState = "未连接";
    private string connectionDetail = "等待配置";
    private StatusLevel connectionStatusLevel = StatusLevel.Unknown;

    private StatusLevel localStatusLevel = StatusLevel.Unknown;
    private string localStatusText = "未检测";
    private StatusLevel gatewayStatusLevel = StatusLevel.Unknown;
    private string gatewayStatusText = "未检测";
    private StatusLevel internetStatusLevel = StatusLevel.Unknown;
    private string internetStatusText = "未检测";

    private double trafficUsedGb;
    private double trafficTotalGb;
    private double subscriptionTrafficUsedGb;
    private double subscriptionTrafficTotalGb;
    private string subscriptionTrafficUnit = "GB";
    private DateTimeOffset? subscriptionTrafficUpdatedAt;
    private DateTimeOffset? trafficUpdatedAt;
    private DateTimeOffset? lastReloadAt;
    private string trafficUpRateText = "0 B/s";
    private string trafficDownRateText = "0 B/s";
    private long lastUploadBytes;
    private long lastDownloadBytes;
    private DateTimeOffset? lastTrafficSampleAt;
    private bool isModeSwitching;

    private bool autoStartEnabled = true;
    private bool silentOnBootEnabled;
    private bool closeToTrayEnabled = true;
    private bool serviceModeEnabled;
    private bool autoRunCliEnabled = true;
    private string cliPath = string.Empty;
    private string cliArgs = "--daemon";
    private bool debugLoggingEnabled;
    private string themeMode = "Dark";
    private string serviceModeMessage = string.Empty;
    private bool isServiceModeMessageOpen;
    private InfoBarSeverity serviceModeSeverity = InfoBarSeverity.Informational;

    public event PropertyChangedEventHandler? PropertyChanged;

    public MainViewModel()
        : this(new AppSettings())
    {
    }

    public MainViewModel(AppSettings settings)
    {
        ApplySettings(settings);
    }

    public void ApplySettings(AppSettings settings)
    {
        AutoStartEnabled = settings.AutoStartEnabled;
        SilentOnBootEnabled = settings.SilentOnBootEnabled;
        CloseToTrayEnabled = settings.CloseToTrayEnabled;
        AutoRunCliEnabled = settings.AutoRunCliEnabled;
        CliPath = string.IsNullOrWhiteSpace(settings.CliPath)
            ? string.Empty
            : settings.CliPath;
        CliArgs = settings.CliArgs is null ? string.Empty : settings.CliArgs;
        DebugLoggingEnabled = settings.DebugLoggingEnabled;
        ThemeMode = string.IsNullOrWhiteSpace(settings.ThemeMode)
            ? "Dark"
            : settings.ThemeMode;
        SubscriptionColumns = settings.SubscriptionColumns <= 0 ? 2 : settings.SubscriptionColumns;
    }

    public string GatewayUrl
    {
        get => gatewayUrl;
        set => SetField(ref gatewayUrl, value);
    }

    public string GatewayToken
    {
        get => gatewayToken;
        set
        {
            if (SetField(ref gatewayToken, value) && !string.IsNullOrWhiteSpace(value))
            {
                GatewayTokenConfigured = true;
            }
        }
    }

    public bool GatewayTokenConfigured
    {
        get => gatewayTokenConfigured;
        set
        {
            if (SetField(ref gatewayTokenConfigured, value))
            {
                OnPropertyChanged(nameof(GatewayTokenStatusText));
            }
        }
    }

    public string SubscriptionUrl
    {
        get => subscriptionUrl;
        set
        {
            if (SetField(ref subscriptionUrl, value))
            {
                OnPropertyChanged(nameof(IsSubscriptionAvailable));
            }
        }
    }

    public string LocalPort
    {
        get => localPort;
        set
        {
            if (SetField(ref localPort, value))
            {
                OnPropertyChanged(nameof(ModeSwitchMessage));
            }
        }
    }

    public bool IsSubscriptionMode
    {
        get => isSubscriptionMode;
        set
        {
            if (SetField(ref isSubscriptionMode, value))
            {
                OnPropertyChanged(nameof(IsLocalMode));
                OnPropertyChanged(nameof(ModeSummary));
                OnPropertyChanged(nameof(ModeStatusBrush));
                OnPropertyChanged(nameof(ModeSwitchMessage));
            }
        }
    }

    public bool IsLocalMode
    {
        get => !IsSubscriptionMode;
        set
        {
            if (value)
            {
                IsSubscriptionMode = false;
            }
        }
    }

    public bool IsSubscriptionAvailable => !string.IsNullOrWhiteSpace(SubscriptionUrl);
    public string ModeSummary => IsSubscriptionMode ? "订阅模式" : "本地端口";
    public string GatewayTokenStatusText => GatewayTokenConfigured ? "已配置" : "未配置";

    public Brush ModeStatusBrush => IsSubscriptionMode
        ? GetAccentBrush()
        : GetBrush(StatusLevel.Warning);

    public string ModeHint
    {
        get => modeHint;
        set => SetField(ref modeHint, value);
    }

    public int SubscriptionRefreshHours
    {
        get => subscriptionRefreshHours;
        set => SetField(ref subscriptionRefreshHours, value);
    }

    public int SubscriptionProbeMinutes
    {
        get => subscriptionProbeMinutes;
        set => SetField(ref subscriptionProbeMinutes, value);
    }

    public int SubscriptionColumns
    {
        get => subscriptionColumns;
        set => SetField(ref subscriptionColumns, value);
    }

    public string ConnectionState
    {
        get => connectionState;
        set => SetField(ref connectionState, value);
    }

    public string ConnectionDetail
    {
        get => connectionDetail;
        set => SetField(ref connectionDetail, value);
    }

    public StatusLevel ConnectionStatusLevel
    {
        get => connectionStatusLevel;
        set
        {
            if (SetField(ref connectionStatusLevel, value))
            {
                OnPropertyChanged(nameof(ConnectionStatusBrush));
            }
        }
    }

    public Brush ConnectionStatusBrush => GetBrush(ConnectionStatusLevel);

    public StatusLevel LocalStatusLevel
    {
        get => localStatusLevel;
        set
        {
            if (SetField(ref localStatusLevel, value))
            {
                OnPropertyChanged(nameof(LocalStatusBrush));
            }
        }
    }

    public string LocalStatusText
    {
        get => localStatusText;
        set
        {
            if (SetField(ref localStatusText, value))
            {
                OnPropertyChanged(nameof(ConnectivitySummary));
            }
        }
    }

    public Brush LocalStatusBrush => GetBrush(LocalStatusLevel);

    public StatusLevel GatewayStatusLevel
    {
        get => gatewayStatusLevel;
        set
        {
            if (SetField(ref gatewayStatusLevel, value))
            {
                OnPropertyChanged(nameof(GatewayStatusBrush));
            }
        }
    }

    public string GatewayStatusText
    {
        get => gatewayStatusText;
        set
        {
            if (SetField(ref gatewayStatusText, value))
            {
                OnPropertyChanged(nameof(ConnectivitySummary));
            }
        }
    }

    public Brush GatewayStatusBrush => GetBrush(GatewayStatusLevel);

    public StatusLevel InternetStatusLevel
    {
        get => internetStatusLevel;
        set
        {
            if (SetField(ref internetStatusLevel, value))
            {
                OnPropertyChanged(nameof(InternetStatusBrush));
            }
        }
    }

    public string InternetStatusText
    {
        get => internetStatusText;
        set
        {
            if (SetField(ref internetStatusText, value))
            {
                OnPropertyChanged(nameof(ConnectivitySummary));
            }
        }
    }

    public Brush InternetStatusBrush => GetBrush(InternetStatusLevel);

    public double TrafficUsedGb
    {
        get => trafficUsedGb;
        set
        {
            if (SetField(ref trafficUsedGb, value))
            {
                RaiseTrafficChanged();
            }
        }
    }

    public double TrafficTotalGb
    {
        get => trafficTotalGb;
        set
        {
            if (SetField(ref trafficTotalGb, value))
            {
                RaiseTrafficChanged();
            }
        }
    }

    public string TrafficSummary => EffectiveTrafficTotal > 0
        ? $"{EffectiveTrafficUsed:0.0} / {EffectiveTrafficTotal:0.0} {EffectiveTrafficUnit}"
        : "等待同步";
    public double TrafficUsageValue => EffectiveTrafficTotal > 0 ? Math.Max(0, EffectiveTrafficUsed) : 0;
    public double TrafficUsageMaximum => EffectiveTrafficTotal > 0 ? EffectiveTrafficTotal : 1;
    public string TrafficUsagePercentText => EffectiveTrafficTotal > 0
        ? $"{Math.Min(100, EffectiveTrafficUsed / EffectiveTrafficTotal * 100):0}% 已使用"
        : "等待同步订阅用量";
    public string TrafficRemainingText => EffectiveTrafficTotal > 0
        ? $"剩余 {Math.Max(0, EffectiveTrafficTotal - EffectiveTrafficUsed):0.0} {EffectiveTrafficUnit}"
        : "总量未知";

    public string ShellResidencyText => ServiceModeEnabled
        ? "单机回环 · 完全静默后台"
        : "单机回环 · 桌面后台";

    public string ConnectivitySummary => $"本地 {LocalStatusText} · 网关 {GatewayStatusText} · 互联网 {InternetStatusText}";

    public string TrafficUpRateText
    {
        get => trafficUpRateText;
        private set => SetField(ref trafficUpRateText, value);
    }

    public string TrafficDownRateText
    {
        get => trafficDownRateText;
        private set => SetField(ref trafficDownRateText, value);
    }

    public string TrafficUpdatedAtText => EffectiveTrafficUpdatedAt is null
        ? "未刷新"
        : $"更新于 {EffectiveTrafficUpdatedAt:HH:mm:ss}";

    public string LastReloadText => lastReloadAt is null
        ? "未重载"
        : $"{lastReloadAt:yyyy-MM-dd HH:mm}";

    public string LogFilePath => AppPaths.ServiceLogPath;

    public bool DesktopResidencyOptionsEnabled => !ServiceModeEnabled;

    public string DesktopResidencySummary => ServiceModeEnabled
        ? "服务模式已接管后台驻留，下面这些桌面行为已暂时失效。"
        : "日常推荐使用桌面后台：开机自启、静默启动、关闭最小化到托盘。";

    public string BackgroundProfileTitle => ServiceModeEnabled
        ? "完全静默后台"
        : "桌面后台";

    public string BackgroundProfileDetail => ServiceModeEnabled
        ? "当前将由 Windows 后台服务接管，启用后会关闭主窗口与托盘。"
        : "当前由桌面进程常驻，适合日常可视化管理与快速排障。";

    public bool IsModeSwitching
    {
        get => isModeSwitching;
        set
        {
            if (SetField(ref isModeSwitching, value))
            {
                OnPropertyChanged(nameof(CanSwitchModeButtons));
                OnPropertyChanged(nameof(ModeSwitchMessage));
            }
        }
    }

    public bool CanSwitchModeButtons => !IsModeSwitching;

    public string ModeSwitchMessage => !IsModeSwitching
        ? string.Empty
        : IsSubscriptionMode
            ? "正在切换到订阅模式..."
            : $"正在切换到本地端口 {LocalPort}...";

    public string ServiceModeHint => ServiceModeEnabled
        ? "当前已切换到服务模式。手动打开界面时，这个前台窗口只负责配置，不参与常驻。"
        : "需要完全无界面、无托盘时再启用服务模式；启用后会尝试注册 Windows 服务，失败时自动回退计划任务。";

    public string ServiceModeAccountText => "LocalService（低权限）";

    public string ServiceModeDataPath => AppPaths.ServiceBaseDirectory;

    public bool AutoStartEnabled
    {
        get => autoStartEnabled;
        set => SetField(ref autoStartEnabled, value);
    }

    public bool SilentOnBootEnabled
    {
        get => silentOnBootEnabled;
        set => SetField(ref silentOnBootEnabled, value);
    }

    public bool CloseToTrayEnabled
    {
        get => closeToTrayEnabled;
        set => SetField(ref closeToTrayEnabled, value);
    }

    public bool ServiceModeEnabled
    {
        get => serviceModeEnabled;
        set
        {
            if (SetField(ref serviceModeEnabled, value))
            {
                OnPropertyChanged(nameof(DesktopResidencyOptionsEnabled));
                OnPropertyChanged(nameof(DesktopResidencySummary));
                OnPropertyChanged(nameof(BackgroundProfileTitle));
                OnPropertyChanged(nameof(BackgroundProfileDetail));
                OnPropertyChanged(nameof(ServiceModeHint));
                OnPropertyChanged(nameof(ShellResidencyText));
            }
        }
    }

    public bool AutoRunCliEnabled
    {
        get => autoRunCliEnabled;
        set => SetField(ref autoRunCliEnabled, value);
    }

    public string CliPath
    {
        get => cliPath;
        set => SetField(ref cliPath, value);
    }

    public string CliArgs
    {
        get => cliArgs;
        set => SetField(ref cliArgs, value);
    }

    public bool DebugLoggingEnabled
    {
        get => debugLoggingEnabled;
        set => SetField(ref debugLoggingEnabled, value);
    }

    public string ThemeMode
    {
        get => themeMode;
        set => SetField(ref themeMode, value);
    }

    public string ServiceModeMessage
    {
        get => serviceModeMessage;
        set => SetField(ref serviceModeMessage, value);
    }

    public bool IsServiceModeMessageOpen
    {
        get => isServiceModeMessageOpen;
        set => SetField(ref isServiceModeMessageOpen, value);
    }

    public InfoBarSeverity ServiceModeSeverity
    {
        get => serviceModeSeverity;
        set => SetField(ref serviceModeSeverity, value);
    }

    public void EnsureSubscriptionFallback()
    {
        if (string.IsNullOrWhiteSpace(SubscriptionUrl))
        {
            if (IsSubscriptionMode)
            {
                IsSubscriptionMode = false;
                ModeHint = "订阅缺失，已自动回退本地端口。";
            }
        }
        else
        {
            ModeHint = "订阅优先，失败自动回退本地端口。";
        }
    }

    public void Connect()
    {
        EnsureSubscriptionFallback();

        if (string.IsNullOrWhiteSpace(GatewayUrl))
        {
            ConnectionState = "未连接";
            ConnectionStatusLevel = StatusLevel.Warning;
            ConnectionDetail = "请填写网关地址";
            SetLocalStatus(StatusLevel.Unknown, "未检测");
            SetGatewayStatus(StatusLevel.Warning, "未配置");
            SetInternetStatus(StatusLevel.Unknown, "未检测");
            return;
        }

        ConnectionState = "已连接";
        ConnectionStatusLevel = StatusLevel.Ok;
        ConnectionDetail = IsSubscriptionMode
            ? "订阅模式已连接"
            : "本地端口已接入";
        SetLocalStatus(StatusLevel.Ok, "可用");
    }

    public void ReloadConfig()
    {
        lastReloadAt = DateTimeOffset.Now;
        OnPropertyChanged(nameof(LastReloadText));
        ConnectionDetail = $"配置已重载 ({lastReloadAt:HH:mm:ss})";
    }

    public void UpdateTrafficRates(long uploadBytes, long downloadBytes, DateTimeOffset sampleAt)
    {
        if (lastTrafficSampleAt is null)
        {
            lastTrafficSampleAt = sampleAt;
            lastUploadBytes = uploadBytes;
            lastDownloadBytes = downloadBytes;
            TrafficUpRateText = "0 B/s";
            TrafficDownRateText = "0 B/s";
            return;
        }

        var deltaSeconds = (sampleAt - lastTrafficSampleAt.Value).TotalSeconds;
        if (deltaSeconds <= 0.2)
        {
            return;
        }

        var upRate = (uploadBytes - lastUploadBytes) / deltaSeconds;
        var downRate = (downloadBytes - lastDownloadBytes) / deltaSeconds;
        lastTrafficSampleAt = sampleAt;
        lastUploadBytes = uploadBytes;
        lastDownloadBytes = downloadBytes;
        TrafficUpRateText = FormatRate(upRate);
        TrafficDownRateText = FormatRate(downRate);
    }

    public void SetTrafficUpdated(DateTimeOffset time)
    {
        trafficUpdatedAt = time;
        OnPropertyChanged(nameof(TrafficUpdatedAtText));
    }

    public void SetSubscriptionTrafficSnapshot(double used, double total, string? unit, DateTimeOffset? updatedAt)
    {
        subscriptionTrafficUsedGb = Math.Max(0, used);
        subscriptionTrafficTotalGb = Math.Max(0, total);
        subscriptionTrafficUnit = string.IsNullOrWhiteSpace(unit) ? "GB" : unit;
        subscriptionTrafficUpdatedAt = updatedAt;
        RaiseTrafficChanged();
    }

    public void ClearSubscriptionTrafficSnapshot()
    {
        subscriptionTrafficUsedGb = 0;
        subscriptionTrafficTotalGb = 0;
        subscriptionTrafficUnit = "GB";
        subscriptionTrafficUpdatedAt = null;
        RaiseTrafficChanged();
    }

    private void SetLocalStatus(StatusLevel level, string text)
    {
        LocalStatusLevel = level;
        LocalStatusText = text;
    }

    private void SetGatewayStatus(StatusLevel level, string text)
    {
        GatewayStatusLevel = level;
        GatewayStatusText = text;
    }

    private void SetInternetStatus(StatusLevel level, string text)
    {
        InternetStatusLevel = level;
        InternetStatusText = text;
    }

    private static Brush GetBrush(StatusLevel level)
    {
        var key = level switch
        {
            StatusLevel.Ok => "StatusOkBrush",
            StatusLevel.Warning => "StatusWarnBrush",
            StatusLevel.Error => "StatusBadBrush",
            _ => "StatusUnknownBrush",
        };

        if (Application.Current?.Resources.TryGetValue(key, out var value) is true && value is SolidColorBrush brush)
        {
            return brush;
        }

        return level switch
        {
            StatusLevel.Ok => new SolidColorBrush(Colors.Green),
            StatusLevel.Warning => new SolidColorBrush(Colors.Orange),
            StatusLevel.Error => new SolidColorBrush(Colors.Red),
            _ => new SolidColorBrush(Colors.Gray),
        };
    }

    private static Brush GetAccentBrush()
    {
        if (Application.Current?.Resources.TryGetValue("AccentBrush", out var value) is true && value is SolidColorBrush brush)
        {
            return brush;
        }
        return new SolidColorBrush(Colors.CadetBlue);
    }

    private static string FormatRate(double bytesPerSecond)
    {
        if (bytesPerSecond <= 0)
        {
            return "0 B/s";
        }
        var abs = Math.Abs(bytesPerSecond);
        if (abs < 1024)
        {
            return $"{abs:0} B/s";
        }
        if (abs < 1024 * 1024)
        {
            return $"{abs / 1024:0.0} KB/s";
        }
        if (abs < 1024 * 1024 * 1024)
        {
            return $"{abs / (1024 * 1024):0.0} MB/s";
        }
        return $"{abs / (1024 * 1024 * 1024):0.0} GB/s";
    }

    private double EffectiveTrafficUsed => trafficTotalGb > 0 ? trafficUsedGb : subscriptionTrafficUsedGb;

    private double EffectiveTrafficTotal => trafficTotalGb > 0 ? trafficTotalGb : subscriptionTrafficTotalGb;

    private string EffectiveTrafficUnit => trafficTotalGb > 0 ? "GB" : subscriptionTrafficUnit;

    private DateTimeOffset? EffectiveTrafficUpdatedAt => trafficTotalGb > 0 ? trafficUpdatedAt : subscriptionTrafficUpdatedAt;

    private void RaiseTrafficChanged()
    {
        OnPropertyChanged(nameof(TrafficSummary));
        OnPropertyChanged(nameof(TrafficUsageValue));
        OnPropertyChanged(nameof(TrafficUsageMaximum));
        OnPropertyChanged(nameof(TrafficUsagePercentText));
        OnPropertyChanged(nameof(TrafficRemainingText));
        OnPropertyChanged(nameof(TrafficUpdatedAtText));
    }

    private bool SetField<T>(ref T field, T value, [CallerMemberName] string? name = null)
    {
        if (EqualityComparer<T>.Default.Equals(field, value))
        {
            return false;
        }

        field = value;
        OnPropertyChanged(name);
        return true;
    }

    private void OnPropertyChanged([CallerMemberName] string? name = null)
        => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
}

