using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Threading.Tasks;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using OpenClawAdapter;
using OpenClawAdapter.Services;
using OpenClawAdapter.ViewModels;

namespace OpenClawAdapter.Views;

public partial class MainPage : Page
{
    private bool isLoaded;
    private bool isUpdatingSystemProxySwitch;
    private bool isApplyingConfigToView;
    private bool isModeSwitching;

    public MainViewModel ViewModel { get; } = AppState.ViewModel;
    private AdapterApiClient Api => AppState.Api;

    public MainPage()
    {
        InitializeComponent();
        DataContext = ViewModel;
        ViewModel.PropertyChanged += OnViewModelPropertyChanged;
    }

    private async void OnLoaded(object sender, RoutedEventArgs e)
    {
        if (isLoaded)
        {
            return;
        }

        isLoaded = true;
        if (!await EnsureBackendReadyAsync("本地服务未启动。"))
        {
            return;
        }

        var configLoaded = await LoadConfigAsync();
        if (!configLoaded)
        {
            return;
        }

        try
        {
            await SyncStatusAsync(applyProbe: true);
        }
        catch (Exception ex)
        {
            ViewModel.ConnectionDetail = $"状态同步失败：{ex.Message}";
            ViewModel.ConnectionStatusLevel = StatusLevel.Warning;
        }
    }


    private async void OnReloadClicked(object sender, RoutedEventArgs e)
    {
        if (!await EnsureBackendReadyAsync("本地服务未启动。"))
        {
            return;
        }

        try
        {
            var resp = await Api.ReloadAsync();
            ApplyProbe(resp.Probe);
            ViewModel.ReloadConfig();
            await LoadConfigAsync();
            await SyncStatusAsync(applyProbe: false);
        }
        catch (Exception ex)
        {
            ViewModel.ConnectionDetail = $"重载失败：{ex.Message}";
            ViewModel.ConnectionStatusLevel = StatusLevel.Warning;
        }
    }

    private async void OnProbeClicked(object sender, RoutedEventArgs e)
    {
        if (!await EnsureBackendReadyAsync("本地服务未启动。"))
        {
            return;
        }

        try
        {
            await SyncStatusAsync(applyProbe: true);
        }
        catch (Exception ex)
        {
            ViewModel.ConnectionDetail = $"探测失败：{ex.Message}";
            ViewModel.ConnectionStatusLevel = StatusLevel.Warning;
        }
    }

    private async Task<bool> LoadConfigAsync()
    {
        try
        {
            var resp = await Api.GetConfigAsync();
            isApplyingConfigToView = true;
            AppState.ApplyAdapterConfig(resp.Config);
            ViewModel.GatewayTokenConfigured = resp.TokenPresent || !string.IsNullOrWhiteSpace(ViewModel.GatewayToken);
            isApplyingConfigToView = false;
            ViewModel.Connect();
            UpdateModeButtons();
            return true;
        }
        catch (Exception ex)
        {
            isApplyingConfigToView = false;
            ViewModel.ConnectionDetail = $"读取配置失败：{ex.Message}";
            ViewModel.ConnectionStatusLevel = StatusLevel.Warning;
            return false;
        }
    }

    private async Task ApplySubscriptionModeAsync()
    {
        if (isModeSwitching)
        {
            return;
        }

        isModeSwitching = true;
        try
        {
            if (!await EnsureBackendReadyAsync("本地服务未启动。"))
            {
                ViewModel.IsSubscriptionMode = false;
                return;
            }

            var resp = await Api.GetSubscriptionsAsync();
            var target = PickSubscription(resp);
            if (target is null || string.IsNullOrWhiteSpace(target.Id))
            {
                ViewModel.IsSubscriptionMode = false;
                ViewModel.ConnectionDetail = "没有可用订阅，无法切换。";
                return;
            }

            await Api.ActivateSubscriptionAsync(target.Id);
            var probe = await Api.ReloadAsync();
            ApplyProbe(probe.Probe);
            await LoadConfigAsync();
            await SyncStatusAsync(applyProbe: false);
        }
        catch (Exception ex)
        {
            ViewModel.ConnectionDetail = $"切换失败：{ex.Message}";
        }
        finally
        {
            isModeSwitching = false;
        }
    }

    private async Task ApplyLocalModeAsync()
    {
        if (isModeSwitching)
        {
            return;
        }

        isModeSwitching = true;
        if (!int.TryParse(ViewModel.LocalPort, out var port) || port <= 0)
        {
            ViewModel.ConnectionDetail = "本地端口无效。";
            ViewModel.IsSubscriptionMode = true;
            isModeSwitching = false;
            return;
        }

        var payload = new Dictionary<string, object>
        {
            ["proxy"] = new Dictionary<string, object>
            {
                ["mode"] = "local_port",
                ["local_port"] = port,
            },
        };

        try
        {
            if (!await EnsureBackendReadyAsync("本地服务未启动。"))
            {
                ViewModel.IsSubscriptionMode = true;
                return;
            }

            await Api.SetConfigAsync(payload);
            var resp = await Api.ReloadAsync();
            ApplyProbe(resp.Probe);
            await LoadConfigAsync();
            await SyncStatusAsync(applyProbe: false);
        }
        catch (Exception ex)
        {
            ViewModel.ConnectionDetail = $"切换失败：{ex.Message}";
        }
        finally
        {
            isModeSwitching = false;
        }
    }

    private async Task SyncStatusAsync(bool applyProbe)
    {
        var status = await Api.GetStatusAsync();
        if (applyProbe)
        {
            ApplyProbe(status.Probe);
        }

        var enabled = status.SystemProxy?.Enabled ?? false;
        if (SystemProxySwitch is not null)
        {
            isUpdatingSystemProxySwitch = true;
            SystemProxySwitch.IsOn = enabled;
            isUpdatingSystemProxySwitch = false;
        }
    }

    private async void OnSystemProxyToggled(object sender, RoutedEventArgs e)
    {
        if (isUpdatingSystemProxySwitch || sender is not ToggleSwitch toggle)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync("本地服务未启动。"))
        {
            isUpdatingSystemProxySwitch = true;
            toggle.IsOn = false;
            isUpdatingSystemProxySwitch = false;
            return;
        }

        try
        {
            if (toggle.IsOn)
            {
                await Api.EnableSystemProxyAsync();
                ViewModel.ConnectionDetail = "系统代理已开启。";
            }
            else
            {
                await Api.DisableSystemProxyAsync();
                ViewModel.ConnectionDetail = "系统代理已关闭。";
            }
        }
        catch (Exception ex)
        {
            ViewModel.ConnectionDetail = $"系统代理切换失败：{ex.Message}";
            ViewModel.ConnectionStatusLevel = StatusLevel.Warning;
        }
        finally
        {
            try
            {
                await SyncStatusAsync(applyProbe: false);
            }
            catch
            {
            }
        }
    }

    private static AdapterSubscription? PickSubscription(AdapterSubscriptionsResponse resp)
    {
        if (!string.IsNullOrWhiteSpace(resp.ActiveId))
        {
            foreach (var sub in resp.Subscriptions)
            {
                if (!string.IsNullOrWhiteSpace(sub.Id)
                    && string.Equals(sub.Id, resp.ActiveId, StringComparison.OrdinalIgnoreCase)
                    && !string.IsNullOrWhiteSpace(sub.Url))
                {
                    return sub;
                }
            }
        }

        foreach (var sub in resp.Subscriptions)
        {
            if (!string.IsNullOrWhiteSpace(sub.Url))
            {
                return sub;
            }
        }

        return null;
    }

    private void ApplyProbe(AdapterProbe? probe)
    {
        if (probe is null)
        {
            return;
        }

        ViewModel.LocalStatusLevel = StatusLevel.Ok;
        ViewModel.LocalStatusText = "可用";
        ViewModel.GatewayStatusLevel = probe.GatewayOk ? StatusLevel.Ok : StatusLevel.Error;
        ViewModel.GatewayStatusText = probe.GatewayOk ? "可达" : "不可达";
        ViewModel.InternetStatusLevel = probe.InternetOk ? StatusLevel.Ok : StatusLevel.Warning;
        ViewModel.InternetStatusText = probe.InternetOk ? "可用" : "异常";
    }

    private void OnGatewayUrlLostFocus(object sender, RoutedEventArgs e)
    {
        var raw = ViewModel.GatewayUrl?.Trim();
        if (string.IsNullOrWhiteSpace(raw))
        {
            return;
        }

        if (!Uri.TryCreate(raw, UriKind.Absolute, out var uri))
        {
            return;
        }

        if (TryExtractToken(uri, out var token, out var sanitizedUrl))
        {
            ViewModel.GatewayToken = token;
            if (TokenBox.Password != token)
            {
                TokenBox.Password = token;
            }

            if (!string.IsNullOrWhiteSpace(sanitizedUrl))
            {
                ViewModel.GatewayUrl = sanitizedUrl;
            }
        }
    }

    private void OnTokenPasswordChanged(object sender, RoutedEventArgs e)
    {
        ViewModel.GatewayToken = TokenBox.Password;
        ViewModel.GatewayTokenConfigured = !string.IsNullOrWhiteSpace(TokenBox.Password);
    }

    private static bool TryExtractToken(Uri uri, out string token, out string? sanitizedUrl)
    {
        token = string.Empty;
        sanitizedUrl = null;

        var query = uri.Query;
        if (string.IsNullOrWhiteSpace(query))
        {
            return false;
        }

        var kept = new List<string>();
        var parts = query.TrimStart('?').Split('&', StringSplitOptions.RemoveEmptyEntries);
        foreach (var part in parts)
        {
            var pieces = part.Split('=', 2);
            var key = Uri.UnescapeDataString(pieces[0]);
            var value = pieces.Length > 1 ? Uri.UnescapeDataString(pieces[1]) : string.Empty;

            if (key.Equals("token", StringComparison.OrdinalIgnoreCase)
                || key.Equals("access_token", StringComparison.OrdinalIgnoreCase))
            {
                if (!string.IsNullOrWhiteSpace(value))
                {
                    token = value;
                }
                continue;
            }

            kept.Add(part);
        }

        if (string.IsNullOrWhiteSpace(token))
        {
            return false;
        }

        var baseUrl = uri.GetLeftPart(UriPartial.Path);
        var rebuiltQuery = kept.Count > 0 ? "?" + string.Join("&", kept) : string.Empty;
        sanitizedUrl = baseUrl + rebuiltQuery + uri.Fragment;
        return true;
    }

    private void OnSubscriptionModeClicked(object sender, RoutedEventArgs e)
        => _ = HandleModeSwitchAsync(useSubscription: true);

    private void OnLocalModeClicked(object sender, RoutedEventArgs e)
        => _ = HandleModeSwitchAsync(useSubscription: false);

    private async Task HandleModeSwitchAsync(bool useSubscription)
    {
        if (!isLoaded || isApplyingConfigToView || isModeSwitching)
        {
            return;
        }

        if (useSubscription ? ViewModel.IsSubscriptionMode : ViewModel.IsLocalMode)
        {
            return;
        }

        if (useSubscription)
        {
            await ApplySubscriptionModeAsync();
            return;
        }

        await ApplyLocalModeAsync();
    }

    private void OnViewModelPropertyChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(MainViewModel.IsSubscriptionMode))
        {
            DispatcherQueue?.TryEnqueue(UpdateModeButtons);
        }
    }

    private void UpdateModeButtons()
    {
        var selected = Application.Current.Resources["OverviewSelectedButton"] as Style;
        var secondary = Application.Current.Resources["OverviewSecondaryButton"] as Style;
        if (selected is null || secondary is null || SubscriptionModeButton is null || LocalModeButton is null)
        {
            return;
        }

        SubscriptionModeButton.Style = ViewModel.IsSubscriptionMode ? selected : secondary;
        LocalModeButton.Style = ViewModel.IsSubscriptionMode ? secondary : selected;
    }

    private async Task<bool> EnsureBackendReadyAsync(string detail)
    {
        if (await AppState.EnsureBackendReadyAsync())
        {
            return true;
        }

        ViewModel.ConnectionState = "未连接";
        ViewModel.ConnectionStatusLevel = StatusLevel.Warning;
        ViewModel.ConnectionDetail = detail;
        ViewModel.LocalStatusLevel = StatusLevel.Warning;
        ViewModel.LocalStatusText = "未启动";
        ViewModel.GatewayStatusLevel = StatusLevel.Unknown;
        ViewModel.GatewayStatusText = "未检测";
        ViewModel.InternetStatusLevel = StatusLevel.Unknown;
        ViewModel.InternetStatusText = "未检测";
        return false;
    }
}
