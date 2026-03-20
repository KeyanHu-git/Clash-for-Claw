using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using OpenClawAdapter.Services;
using OpenClawAdapter.ViewModels;

namespace OpenClawAdapter.Views;

public partial class SettingsPage : Page
{
    private bool isLoaded;
    private bool isApplyingConfig;
    private bool isApplyingServiceMode;

    public MainViewModel ViewModel { get; } = AppState.ViewModel;
    private AdapterApiClient Api => AppState.Api;

    public SettingsPage()
    {
        InitializeComponent();
        DataContext = ViewModel;
    }

    private async void OnLoaded(object sender, RoutedEventArgs e)
    {
        if (isLoaded)
        {
            return;
        }

        isLoaded = true;
        await LoadConfigAsync();
    }

    private async Task LoadConfigAsync()
    {
        try
        {
            isApplyingConfig = true;
            var resp = await Api.GetConfigAsync();
            AppState.ApplyAdapterConfig(resp.Config);
            AppState.ApplyServiceModeSelection(SettingsStore.Current.ServiceModeEnabled);
        }
        catch (Exception ex)
        {
            ViewModel.ConnectionDetail = $"读取配置失败：{ex.Message}";
        }
        finally
        {
            isApplyingConfig = false;
        }
    }

    private async void OnServiceModeToggled(object sender, RoutedEventArgs e)
    {
        if (!isLoaded || isApplyingConfig || isApplyingServiceMode || sender is not ToggleSwitch toggle)
        {
            return;
        }

        var targetState = toggle.IsOn;
        if (targetState == SettingsStore.Current.ServiceModeEnabled)
        {
            return;
        }

        if (targetState)
        {
            var confirmed = await ConfirmEnableServiceModeAsync();
            if (!confirmed)
            {
                SetServiceModeToggle(SettingsStore.Current.ServiceModeEnabled);
                return;
            }
        }

        isApplyingServiceMode = true;
        try
        {
            var result = await AppState.ChangeServiceModeAsync(targetState);
            var actualState = SettingsStore.Current.ServiceModeEnabled;
            var succeeded = actualState == targetState;

            SetServiceModeToggle(actualState);
            await ShowServiceModeResultAsync(result, targetState, succeeded);

            if (targetState && succeeded)
            {
                AppState.RequestExit();
            }
        }
        finally
        {
            isApplyingServiceMode = false;
        }
    }

    private async void OnSubscriptionRefreshChanged(NumberBox sender, NumberBoxValueChangedEventArgs args)
    {
        if (isApplyingConfig || double.IsNaN(args.NewValue))
        {
            return;
        }

        ViewModel.SubscriptionRefreshHours = (int)Math.Round(args.NewValue);
        await UpdateSubscriptionIntervalsAsync();
    }

    private async void OnSubscriptionProbeChanged(NumberBox sender, NumberBoxValueChangedEventArgs args)
    {
        if (isApplyingConfig || double.IsNaN(args.NewValue))
        {
            return;
        }

        ViewModel.SubscriptionProbeMinutes = (int)Math.Round(args.NewValue);
        await UpdateSubscriptionIntervalsAsync();
    }

    private void OnSubscriptionColumnsChanged(NumberBox sender, NumberBoxValueChangedEventArgs args)
    {
        if (double.IsNaN(args.NewValue))
        {
            return;
        }

        ViewModel.SubscriptionColumns = (int)Math.Round(args.NewValue);
    }

    private async Task UpdateSubscriptionIntervalsAsync()
    {
        var refresh = ViewModel.SubscriptionRefreshHours;
        var probe = ViewModel.SubscriptionProbeMinutes;
        if (refresh <= 0 || probe <= 0)
        {
            return;
        }

        var payload = new Dictionary<string, object>
        {
            ["proxy"] = new Dictionary<string, object>
            {
                ["subscription_refresh_hours"] = refresh,
                ["subscription_probe_minutes"] = probe,
            },
        };

        try
        {
            await Api.SetConfigAsync(payload);
        }
        catch (Exception ex)
        {
            ViewModel.ConnectionDetail = $"订阅策略更新失败：{ex.Message}";
        }
    }

    private async void OnLocalPortLostFocus(object sender, RoutedEventArgs e)
    {
        if (isApplyingConfig)
        {
            return;
        }

        if (!int.TryParse(ViewModel.LocalPort, out var port) || port <= 0)
        {
            ViewModel.ConnectionDetail = "本地端口无效。";
            return;
        }

        var payload = new Dictionary<string, object>
        {
            ["proxy"] = new Dictionary<string, object>
            {
                ["local_port"] = port,
            },
        };

        try
        {
            await Api.SetConfigAsync(payload);
            if (!ViewModel.IsSubscriptionMode)
            {
                await Api.ReloadAsync();
            }
            await LoadConfigAsync();
        }
        catch (Exception ex)
        {
            ViewModel.ConnectionDetail = $"端口更新失败：{ex.Message}";
        }
    }

    private async Task<bool> ConfirmEnableServiceModeAsync()
    {
        if (XamlRoot is null)
        {
            return false;
        }

        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot,
            Title = "切换到完全静默后台？",
            PrimaryButtonText = "立即切换",
            CloseButtonText = "取消",
            DefaultButton = ContentDialogButton.Primary,
            Content = "启用后会注册后台服务，当前主窗口与托盘将退出。之后后台仍会继续运行，界面只在你手动打开时出现。",
        };

        var result = await dialog.ShowAsync();
        return result == ContentDialogResult.Primary;
    }

    private async Task ShowServiceModeResultAsync(ServiceModeResult result, bool enabling, bool succeeded)
    {
        if (XamlRoot is null)
        {
            return;
        }

        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot,
            Title = result.Title,
            Content = result.Message,
            CloseButtonText = enabling && succeeded ? "关闭前台" : "知道了",
        };

        await dialog.ShowAsync();
    }

    private void SetServiceModeToggle(bool enabled)
    {
        AppState.ApplyServiceModeSelection(enabled);
        if (ServiceModeSwitch is not null)
        {
            ServiceModeSwitch.IsOn = enabled;
        }
    }
}
