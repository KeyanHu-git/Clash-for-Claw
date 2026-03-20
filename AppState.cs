using System;
using System.ComponentModel;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using OpenClawAdapter.Models;
using OpenClawAdapter.Services;
using OpenClawAdapter.ViewModels;

namespace OpenClawAdapter;

public static class AppState
{
    private static bool isApplyingSettings;
    private static DispatcherQueue? dispatcherQueue;
    private static DispatcherQueueTimer? statusTimer;
    private static bool isStatusPolling;
    private static readonly SemaphoreSlim backendReadyGate = new(1, 1);

    public static MainViewModel ViewModel { get; private set; } = new(new AppSettings());
    public static AdapterApiClient Api { get; } = new();
    public static TrayIconManager Tray { get; } = new();
    public static CliRunner CliRunner { get; } = new();
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

        if (SettingsStore.Current.ServiceModeEnabled)
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

        StartupManager.ApplyAutoStart(SettingsStore.Current);

        if (!SettingsStore.Current.ServiceModeEnabled)
        {
            CliRunner.EnsureRunning(SettingsStore.Current);
        }
        else
        {
            CliRunner.Stop();
        }

        ThemeManager.ApplyTheme(App.MainWindow, SettingsStore.Current.ThemeMode);

        if (!SettingsStore.Current.ServiceModeEnabled)
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
        if (resp.Billing is null)
        {
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
        ViewModel.UpdateTrafficRates(resp.Billing.Upload, resp.Billing.Download, DateTimeOffset.Now);
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
        if (SettingsStore.Current.ServiceModeEnabled)
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
            CliRunner.Stop();
            var enableResult = await Task.Run(() => ServiceModeManager.Enable(SettingsStore.Current));
            ApplyServiceModeResult(enableResult, true);

            if (enableResult.ServiceStarted || enableResult.FallbackScheduled)
            {
                SettingsStore.Update(settings => settings.ServiceModeEnabled = true);
            }
            else
            {
                CliRunner.EnsureRunning(SettingsStore.Current);
            }

            return enableResult;
        }

        var disableResult = await Task.Run(() => ServiceModeManager.Disable(SettingsStore.Current));
        ApplyServiceModeResult(disableResult, false);

        if (string.Equals(disableResult.Title, "关闭服务模式失败", StringComparison.Ordinal))
        {
            return disableResult;
        }

        SettingsStore.Update(settings => settings.ServiceModeEnabled = false);
        CliRunner.EnsureRunning(SettingsStore.Current);
        return disableResult;
    }

    public static void ApplyServiceModeSelection(bool enabled)
    {
        isApplyingSettings = true;
        ViewModel.ServiceModeEnabled = enabled;
        isApplyingSettings = false;
    }

    private static void ApplyServiceModeResult(ServiceModeResult result, bool enabled)
    {
        ViewModel.ServiceModeMessage = result.Message;
        ViewModel.IsServiceModeMessageOpen = true;
        ViewModel.ServiceModeSeverity = result.ServiceStarted
            ? InfoBarSeverity.Success
            : result.FallbackScheduled
                ? InfoBarSeverity.Warning
                : enabled
                    ? InfoBarSeverity.Error
                    : InfoBarSeverity.Informational;
    }

}
