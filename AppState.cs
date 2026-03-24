using System;
using System.ComponentModel;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using ClashForClaw.Models;
using ClashForClaw.Services;
using ClashForClaw.ViewModels;

namespace ClashForClaw;

public static class AppState
{
    private const int AdapterPort = 13000;
    private static readonly TimeSpan ServiceModeBackendDrainTimeout = TimeSpan.FromSeconds(8);
    private static bool isApplyingSettings;
    private static bool isSwitchingToServiceMode;
    private static DispatcherQueue? dispatcherQueue;
    private static DispatcherQueueTimer? statusTimer;
    private static bool isStatusPolling;
    private static readonly SemaphoreSlim backendReadyGate = new(1, 1);

    public static MainViewModel ViewModel { get; private set; } = new(new AppSettings());
    public static AdapterApiClient Api { get; } = new();
    public static TrayIconManager Tray { get; } = new();
    public static CliRunner CliRunner { get; } = new();
    public static ServiceModeState ServiceMode { get; private set; } = new();
    public static bool IsServiceModeEnabled => (ServiceMode.IsServiceMode && ServiceMode.IsRunning) || ServiceMode.IsTaskFallback;
    public static bool IsWindowsServiceMode => ServiceMode.IsServiceMode && ServiceMode.IsRunning;
    public static bool AllowClose { get; set; }

    public static void Initialize()
    {
        if (dispatcherQueue is null)
        {
            dispatcherQueue = DispatcherQueue.GetForCurrentThread();
        }
        SettingsStore.Load();
        ViewModel = new MainViewModel(SettingsStore.Current);
        ViewModel.PropertyChanged += OnViewModelPropertyChanged;
        SettingsStore.Changed += (_, _) => ApplySettings();
        RefreshServiceModeState();
        ApplySettings();
        StartStatusPolling();
    }

    public static async Task<bool> EnsureBackendReadyAsync()
    {
        try
        {
            await Api.GetStatusAsync();
            return true;
        }
        catch
        {
            // Fall through and attempt a local bootstrap below.
        }

        if (IsServiceModeEnabled || isSwitchingToServiceMode)
        {
            return false;
        }

        await backendReadyGate.WaitAsync();
        try
        {
            try
            {
                await Api.GetStatusAsync();
                return true;
            }
            catch
            {
                // The service is still down; try the local CLI path once.
            }

            if (IsServiceModeEnabled || isSwitchingToServiceMode)
            {
                return false;
            }

            if (!CliRunner.TryStartOnDemand(SettingsStore.Current))
            {
                return false;
            }

            for (var attempt = 0; attempt < 12; attempt++)
            {
                await Task.Delay(350);
                try
                {
                    await Api.GetStatusAsync();
                    return true;
                }
                catch
                {
                    // Keep waiting for the loopback endpoint to accept requests.
                }
            }

            return false;
        }
        finally
        {
            backendReadyGate.Release();
        }
    }

    public static void ApplySettings()
    {
        isApplyingSettings = true;
        ViewModel.ApplySettings(SettingsStore.Current);
        isApplyingSettings = false;

        StartupManager.ApplyAutoStart(SettingsStore.Current, IsServiceModeEnabled);

        if (!isSwitchingToServiceMode)
        {
            if (!IsServiceModeEnabled)
            {
                CliRunner.EnsureRunning(SettingsStore.Current);
            }
            else
            {
                CliRunner.Stop();
            }
        }

        ThemeManager.ApplyTheme(App.MainWindow, SettingsStore.Current.ThemeMode);

        if (!IsServiceModeEnabled)
        {
            Tray.Show();
        }
        else
        {
            Tray.Hide();
        }
    }

    private static void StartStatusPolling()
    {
        if (dispatcherQueue is null)
        {
            return;
        }

        if (statusTimer is null)
        {
            statusTimer = dispatcherQueue.CreateTimer();
            statusTimer.Interval = TimeSpan.FromSeconds(2);
            statusTimer.Tick += async (_, _) => await PollStatusAsync();
        }

        statusTimer.Start();
    }

    private static async Task PollStatusAsync()
    {
        if (isStatusPolling)
        {
            return;
        }

        isStatusPolling = true;
        try
        {
            var resp = await Api.GetStatusAsync();
            ApplyStatus(resp);
        }
        catch
        {
        }
        finally
        {
            isStatusPolling = false;
        }
    }

    private static void ApplyStatus(AdapterStatusResponse resp)
    {
        ApplyProxyHealth(resp);

        if (resp.Billing is null)
        {
            ViewModel.TrafficUsedGb = 0;
            ViewModel.TrafficTotalGb = 0;
            return;
        }

        ViewModel.UpdateTrafficRates(resp.Billing.Upload, resp.Billing.Download, DateTimeOffset.Now);
        if (resp.Billing.Limit <= 0)
        {
            ViewModel.TrafficUsedGb = 0;
            ViewModel.TrafficTotalGb = 0;
            return;
        }

        ViewModel.TrafficUsedGb = resp.Billing.Used;
        ViewModel.TrafficTotalGb = resp.Billing.Limit;
        if (!string.IsNullOrWhiteSpace(resp.Billing.UpdatedAt)
            && DateTimeOffset.TryParse(resp.Billing.UpdatedAt, out var updated))
        {
            ViewModel.SetTrafficUpdated(updated);
        }
        else
        {
            ViewModel.SetTrafficUpdated(DateTimeOffset.Now);
        }
    }

    private static void ApplyProxyHealth(AdapterStatusResponse resp)
    {
        var proxy = resp.Proxy;
        if (proxy is null)
        {
            return;
        }

        if (!string.IsNullOrWhiteSpace(proxy.MihomoError) || proxy.Fallback)
        {
            ViewModel.ConnectionState = proxy.Fallback ? "已回退" : "异常";
            ViewModel.ConnectionStatusLevel = StatusLevel.Warning;
            ViewModel.ConnectionDetail = DescribeProxyIssue(proxy);

            if (string.Equals(proxy.MihomoError, "mihomo_binary_not_found", StringComparison.OrdinalIgnoreCase))
            {
                ViewModel.LocalStatusLevel = StatusLevel.Warning;
                ViewModel.LocalStatusText = "缺少 mihomo";
            }
            return;
        }

        if (string.Equals(proxy.Mode, "subscription_url", StringComparison.OrdinalIgnoreCase) && proxy.MihomoActive)
        {
            ViewModel.ConnectionState = resp.Ok ? "已连接" : "需关注";
            ViewModel.ConnectionStatusLevel = resp.Ok ? StatusLevel.Ok : StatusLevel.Warning;
            return;
        }

        if (string.Equals(proxy.EffectiveMode, "local_port", StringComparison.OrdinalIgnoreCase))
        {
            ViewModel.ConnectionState = "已连接";
            ViewModel.ConnectionStatusLevel = StatusLevel.Ok;
        }
    }

    private static string DescribeProxyIssue(AdapterProxyStatus proxy)
    {
        if (string.Equals(proxy.MihomoError, "mihomo_binary_not_found", StringComparison.OrdinalIgnoreCase))
        {
            return "未找到 mihomo 运行时。程序会优先尝试自动下载；如果当前网络无法访问 GitHub，请将 mihomo.exe 放到应用目录或数据目录的 bin 中后重试。";
        }

        if (proxy.Fallback)
        {
            return "订阅模式未能接管，当前已回退到本地端口。";
        }

        return string.IsNullOrWhiteSpace(proxy.MihomoError)
            ? "本地代理运行异常。"
            : proxy.MihomoError;
    }

    public static void ApplyAdapterConfig(AdapterConfig? cfg)
    {
        if (cfg?.Gateway?.Url is not null)
        {
            ViewModel.GatewayUrl = cfg.Gateway.Url;
        }

        if (cfg?.Proxy is null)
        {
            return;
        }

        ViewModel.IsSubscriptionMode = string.Equals(cfg.Proxy.Mode, "subscription_url", StringComparison.OrdinalIgnoreCase);
        if (cfg.Proxy.LocalPort > 0)
        {
            ViewModel.LocalPort = cfg.Proxy.LocalPort.ToString();
        }
        ViewModel.SubscriptionUrl = string.IsNullOrWhiteSpace(cfg.Proxy.SubscriptionUrl)
            ? string.Empty
            : cfg.Proxy.SubscriptionUrl;
        if (cfg.Proxy.SubscriptionRefreshHours > 0)
        {
            ViewModel.SubscriptionRefreshHours = cfg.Proxy.SubscriptionRefreshHours;
        }
        if (cfg.Proxy.SubscriptionProbeMinutes > 0)
        {
            ViewModel.SubscriptionProbeMinutes = cfg.Proxy.SubscriptionProbeMinutes;
        }
    }

    public static void InitializeTray(Window window)
    {
        if (IsServiceModeEnabled)
        {
            return;
        }

        Tray.Initialize(() => ShowWindow(window), RequestExit, () => ViewModel.ConnectionState);
        Tray.Show();
    }

    public static void RequestExit()
    {
        AllowClose = true;
        Tray.Dispose();
        Application.Current.Exit();
    }

    public static void ShowWindow(Window window)
    {
        WindowManager.Show(window);
    }

    public static void HideToTray(Window window)
    {
        WindowManager.Hide(window);
    }

    private static void OnViewModelPropertyChanged(object? sender, PropertyChangedEventArgs args)
    {
        if (isApplyingSettings)
        {
            return;
        }

        switch (args.PropertyName)
        {
            case nameof(MainViewModel.AutoStartEnabled):
                SettingsStore.Update(settings => settings.AutoStartEnabled = ViewModel.AutoStartEnabled);
                break;
            case nameof(MainViewModel.SilentOnBootEnabled):
                SettingsStore.Update(settings => settings.SilentOnBootEnabled = ViewModel.SilentOnBootEnabled);
                break;
            case nameof(MainViewModel.CloseToTrayEnabled):
                SettingsStore.Update(settings => settings.CloseToTrayEnabled = ViewModel.CloseToTrayEnabled);
                break;
            case nameof(MainViewModel.AutoRunCliEnabled):
                SettingsStore.Update(settings => settings.AutoRunCliEnabled = ViewModel.AutoRunCliEnabled);
                break;
            case nameof(MainViewModel.CliPath):
                SettingsStore.Update(settings => settings.CliPath = ViewModel.CliPath);
                break;
            case nameof(MainViewModel.CliArgs):
                SettingsStore.Update(settings => settings.CliArgs = ViewModel.CliArgs);
                break;
            case nameof(MainViewModel.DebugLoggingEnabled):
                SettingsStore.Update(settings => settings.DebugLoggingEnabled = ViewModel.DebugLoggingEnabled);
                break;
            case nameof(MainViewModel.ThemeMode):
                SettingsStore.Update(settings => settings.ThemeMode = ViewModel.ThemeMode);
                break;
            case nameof(MainViewModel.SubscriptionColumns):
                SettingsStore.Update(settings => settings.SubscriptionColumns = ViewModel.SubscriptionColumns);
                break;
            case nameof(MainViewModel.ConnectionState):
                Tray.UpdateStatus(ViewModel.ConnectionState);
                break;
        }
    }

    public static async Task<ServiceModeResult> ChangeServiceModeAsync(bool enabled)
    {
        if (enabled)
        {
            isSwitchingToServiceMode = true;
            try
            {
                await TryRequestDesktopBackendShutdownAsync(ServiceModeBackendDrainTimeout);
                _ = await CliRunner.StopManagedBackendAsync(SettingsStore.Current, AdapterPort, ServiceModeBackendDrainTimeout);

                var enableResult = await Task.Run(() => ServiceModeManager.Enable(SettingsStore.Current));
                RefreshServiceModeState();
                ApplySettings();

                if (!IsServiceModeEnabled && !enableResult.SuppressDesktopFallback)
                {
                    if (await IsBackendReachableAsync())
                    {
                        enableResult = ServiceModeManager.WithDesktopFallback(enableResult);
                    }
                    else
                    {
                        var desktopFallbackStarted = CliRunner.EnsureRunning(SettingsStore.Current, force: true);
                        if (desktopFallbackStarted)
                        {
                            enableResult = ServiceModeManager.WithDesktopFallback(enableResult);
                        }
                    }
                }

                ApplyServiceModeResult(enableResult, true);
                return enableResult;
            }
            finally
            {
                isSwitchingToServiceMode = false;
            }
        }

        var disableResult = await Task.Run(() => ServiceModeManager.Disable(SettingsStore.Current));
        RefreshServiceModeState();
        ApplySettings();
        ApplyServiceModeResult(disableResult, false);

        if (disableResult.Failed)
        {
            return disableResult;
        }
        return disableResult;
    }

    public static ServiceModeState RefreshServiceModeState()
    {
        ServiceMode = ServiceModeManager.Query(SettingsStore.Current);
        ApplyServiceModeSelection(ServiceMode.IsServiceMode && ServiceMode.IsRunning, ServiceMode.IsTaskFallback);
        return ServiceMode;
    }

    public static void ApplyServiceModeSelection(bool enabled, bool taskFallbackActive = false)
    {
        isApplyingSettings = true;
        ViewModel.ServiceModeEnabled = enabled;
        ViewModel.ServiceTaskFallbackActive = taskFallbackActive;
        isApplyingSettings = false;
    }

    private static void ApplyServiceModeResult(ServiceModeResult result, bool enabled)
    {
        ViewModel.ServiceModeMessage = result.Message;
        ViewModel.IsServiceModeMessageOpen = true;
        ViewModel.ServiceModeSeverity = result.ServiceStarted
            ? InfoBarSeverity.Success
            : result.FallbackScheduled || result.DesktopFallbackStarted
                ? InfoBarSeverity.Warning
                    : enabled
                        ? InfoBarSeverity.Error
                        : InfoBarSeverity.Informational;
    }

    private static async Task<bool> IsBackendReachableAsync()
    {
        try
        {
            await Api.GetStatusAsync();
            return true;
        }
        catch
        {
            return false;
        }
    }

    private static async Task TryRequestDesktopBackendShutdownAsync(TimeSpan timeout)
    {
        try
        {
            await Api.ShutdownDesktopBackendAsync();
        }
        catch
        {
            return;
        }

        var deadline = DateTimeOffset.UtcNow + timeout;
        while (DateTimeOffset.UtcNow < deadline)
        {
            if (!await IsBackendReachableAsync())
            {
                return;
            }

            await Task.Delay(150);
        }
    }

}

