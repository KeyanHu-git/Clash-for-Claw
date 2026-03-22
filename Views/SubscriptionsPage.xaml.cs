using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.CompilerServices;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media.Animation;
using Microsoft.UI.Xaml.Media;
using Windows.ApplicationModel.DataTransfer;
using Windows.Storage.Pickers;
using WinRT.Interop;
using ClashForClaw.Services;
using ClashForClaw.ViewModels;

namespace ClashForClaw.Views;

public partial class SubscriptionsPage : Page, INotifyPropertyChanged
{
    private static readonly Regex UrlRegex = new(@"https?://\S+", RegexOptions.IgnoreCase | RegexOptions.Compiled);

    private bool isLoaded;
    private bool isInitialAutoRefreshRunning;
    private bool isActionBusy;
    private bool isLayoutQueued;
    private double lastItemWidth;
    private int lastColumns = -1;
    private string? activeSubscriptionId;
    private const double SubscriptionItemHeight = 132;
    private double subscriptionColumnWidth = 220;
    private string statusMessage = string.Empty;
    private InfoBarSeverity statusSeverity = InfoBarSeverity.Informational;
    private bool isStatusOpen;

    public event PropertyChangedEventHandler? PropertyChanged;

    public MainViewModel ViewModel { get; } = AppState.ViewModel;
    public ObservableCollection<SubscriptionItem> Subscriptions { get; } = new();

    private AdapterApiClient Api => AppState.Api;

    public double SubscriptionColumnWidth
    {
        get => subscriptionColumnWidth;
        private set => SetField(ref subscriptionColumnWidth, value);
    }

    public string StatusMessage
    {
        get => statusMessage;
        private set => SetField(ref statusMessage, value);
    }

    public InfoBarSeverity StatusSeverity
    {
        get => statusSeverity;
        private set => SetField(ref statusSeverity, value);
    }

    public bool IsStatusOpen
    {
        get => isStatusOpen;
        set => SetField(ref isStatusOpen, value);
    }

    public SubscriptionsPage()
    {
        InitializeComponent();
        DataContext = this;
        ViewModel.PropertyChanged += (_, args) =>
        {
            if (args.PropertyName == nameof(MainViewModel.SubscriptionColumns))
            {
                QueueColumnLayout();
            }
        };
    }

    private async void OnLoaded(object sender, RoutedEventArgs e)
    {
        if (isLoaded)
        {
            return;
        }

        isLoaded = true;
        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        await LoadConfigAsync();
        await LoadSubscriptionsAsync();
        _ = AutoRefreshOnFirstLoadAsync();
        QueueColumnLayout();
    }

    private async Task LoadConfigAsync()
    {
        try
        {
            var resp = await Api.GetConfigAsync();
            AppState.ApplyAdapterConfig(resp.Config);
        }
        catch (Exception ex)
        {
            SetStatus($"读取配置失败：{ex.Message}", true);
        }
    }

    private async Task LoadSubscriptionsAsync()
    {
        try
        {
            var resp = await Api.GetSubscriptionsAsync();
            ApplySubscriptions(resp);

            SetStatus("已同步订阅列表。", false);
            QueueColumnLayout();
        }
        catch (Exception ex)
        {
            SetStatus($"读取订阅失败：{ex.Message}", true);
        }
    }

    private async Task AutoRefreshOnFirstLoadAsync()
    {
        if (isInitialAutoRefreshRunning)
        {
            return;
        }

        isInitialAutoRefreshRunning = true;
        try
        {
            SetStatus("正在自动刷新订阅…", false);
            await Api.RefreshAllSubscriptionsAsync();
            await LoadSubscriptionsAsync();
            SetStatus("已自动刷新订阅。", false);
        }
        catch (Exception ex)
        {
            SetStatus($"自动刷新失败：{ex.Message}", true);
        }
        finally
        {
            isInitialAutoRefreshRunning = false;
        }
    }

    private void ApplySubscriptions(AdapterSubscriptionsResponse resp)
    {
        var map = new Dictionary<string, SubscriptionItem>(StringComparer.OrdinalIgnoreCase);
        foreach (var item in Subscriptions)
        {
            if (!string.IsNullOrWhiteSpace(item.Id))
            {
                map[item.Id] = item;
            }
        }

        var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        foreach (var sub in resp.Subscriptions)
        {
            var id = string.IsNullOrWhiteSpace(sub.Id) ? string.Empty : sub.Id;
            if (string.IsNullOrWhiteSpace(id))
            {
                continue;
            }
            seen.Add(id);
            if (map.TryGetValue(id, out var existing))
            {
                existing.Update(
                    string.IsNullOrWhiteSpace(sub.Name) ? string.Empty : sub.Name,
                    string.IsNullOrWhiteSpace(sub.Source) ? string.Empty : sub.Source,
                    string.IsNullOrWhiteSpace(sub.Url) ? string.Empty : sub.Url,
                    string.IsNullOrWhiteSpace(sub.FilePath) ? string.Empty : sub.FilePath,
                    string.IsNullOrWhiteSpace(sub.State) ? "unknown" : sub.State,
                    sub.LastSuccessAt,
                    sub.UsageUsed,
                    sub.UsageLimit,
                    string.IsNullOrWhiteSpace(sub.UsageUnit) ? "GB" : sub.UsageUnit,
                    sub.ExpireAt,
                    sub.UpdatedAt);
            }
            else
            {
                Subscriptions.Add(new SubscriptionItem(
                    id,
                    string.IsNullOrWhiteSpace(sub.Name) ? string.Empty : sub.Name,
                    string.IsNullOrWhiteSpace(sub.Source) ? string.Empty : sub.Source,
                    string.IsNullOrWhiteSpace(sub.Url) ? string.Empty : sub.Url,
                    string.IsNullOrWhiteSpace(sub.FilePath) ? string.Empty : sub.FilePath,
                    string.IsNullOrWhiteSpace(sub.State) ? "unknown" : sub.State,
                    sub.LastSuccessAt,
                    sub.UsageUsed,
                    sub.UsageLimit,
                    string.IsNullOrWhiteSpace(sub.UsageUnit) ? "GB" : sub.UsageUnit,
                    sub.ExpireAt,
                    sub.UpdatedAt));
            }
        }

        for (var i = Subscriptions.Count - 1; i >= 0; i--)
        {
            if (!seen.Contains(Subscriptions[i].Id))
            {
                Subscriptions.RemoveAt(i);
            }
        }

        activeSubscriptionId = resp.ActiveId;
        UpdateActiveSelection(activeSubscriptionId);
    }

    private async void OnDownloadClicked(object sender, RoutedEventArgs e)
    {
        if (isActionBusy)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        isActionBusy = true;
        try
        {
        var raw = UrlInput.Text;
        if (string.IsNullOrWhiteSpace(raw))
        {
            SetStatus("请输入订阅 URL。", true);
            return;
        }

        var inputs = ParseSubscriptionInput(raw);
        if (inputs.Count == 0)
        {
            SetStatus("未识别到有效 URL。", true);
            return;
        }

        var success = 0;
        var failed = 0;
        string? lastError = null;

        foreach (var entry in inputs)
        {
            try
            {
                await Api.CreateSubscriptionAsync(entry.Url, entry.Name);
                success++;
            }
            catch (Exception ex)
            {
                failed++;
                lastError = ex.Message;
            }
        }

        UrlInput.Text = string.Empty;
        await LoadSubscriptionsAsync();

        if (failed == 0)
        {
            SetStatus($"已添加 {success} 条订阅。", false);
        }
        else
        {
            var detail = string.IsNullOrWhiteSpace(lastError) ? string.Empty : $"（{lastError}）";
            SetStatus($"成功 {success} 条，失败 {failed} 条{detail}", true);
        }
        }
        finally
        {
            isActionBusy = false;
        }
    }

    private async void OnRefreshAllClicked(object sender, RoutedEventArgs e)
    {
        if (isActionBusy)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        isActionBusy = true;
        try
        {
            await Api.RefreshAllSubscriptionsAsync();
            await LoadSubscriptionsAsync();
        }
        catch (Exception ex)
        {
            SetStatus($"刷新失败：{ex.Message}", true);
        }
        finally
        {
            isActionBusy = false;
        }
    }

    private async void OnImportClicked(object sender, RoutedEventArgs e)
    {
        if (isActionBusy)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        isActionBusy = true;
        try
        {
            var picker = new FileOpenPicker();
            picker.FileTypeFilter.Add(".yaml");
            picker.FileTypeFilter.Add(".yml");

            var hwnd = WindowNative.GetWindowHandle(App.MainWindow);
            InitializeWithWindow.Initialize(picker, hwnd);

            var file = await picker.PickSingleFileAsync();
            if (file is null)
            {
                return;
            }

            await Api.ImportSubscriptionAsync(file.Path);
            await LoadSubscriptionsAsync();
        }
        catch (Exception ex)
        {
            SetStatus($"导入失败：{ex.Message}", true);
        }
        finally
        {
            isActionBusy = false;
        }
    }

    private async void OnItemClicked(object sender, ItemClickEventArgs e)
    {
        if (isActionBusy)
        {
            return;
        }

        if (e.ClickedItem is not SubscriptionItem item)
        {
            return;
        }

        if (item.IsActive)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        if (!item.HasUrl)
        {
            UpdateActiveSelection(activeSubscriptionId);
            SetStatus("本地文件订阅不可切换。", true);
            return;
        }

        var targetId = item.Id;
        var previousId = activeSubscriptionId;
        isActionBusy = true;

        try
        {
            await Api.ActivateSubscriptionAsync(targetId);
            activeSubscriptionId = targetId;
            UpdateActiveSelection(targetId);
            SetStatus("已切换订阅。", false);
            await LoadConfigAsync();
        }
        catch (Exception ex)
        {
            UpdateActiveSelection(previousId);
            SetStatus($"切换失败：{ex.Message}", true);
        }
        finally
        {
            isActionBusy = false;
        }
    }

    private async void OnItemQuickRefresh(object sender, RoutedEventArgs e)
    {
        if (sender is not Button button || button.DataContext is not SubscriptionItem sub)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        try
        {
            await Api.RefreshSubscriptionAsync(sub.Id);
            await LoadSubscriptionsAsync();
        }
        catch (Exception ex)
        {
            SetStatus($"刷新失败：{ex.Message}", true);
        }
    }

    private async void OnItemRefresh(object sender, RoutedEventArgs e)
    {
        if (sender is not MenuFlyoutItem item || item.CommandParameter is not SubscriptionItem sub)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        try
        {
            await Api.RefreshSubscriptionAsync(sub.Id);
            await LoadSubscriptionsAsync();
        }
        catch (Exception ex)
        {
            SetStatus($"刷新失败：{ex.Message}", true);
        }
    }

    private async void OnItemRename(object sender, RoutedEventArgs e)
    {
        if (sender is not MenuFlyoutItem item || item.CommandParameter is not SubscriptionItem sub)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot,
            Title = "重命名",
            PrimaryButtonText = "确定",
            CloseButtonText = "取消",
            Content = new TextBox { Text = sub.DisplayName },
        };

        var result = await dialog.ShowAsync();
        if (result != ContentDialogResult.Primary)
        {
            return;
        }

        var name = (dialog.Content as TextBox)?.Text?.Trim();
        if (string.IsNullOrWhiteSpace(name))
        {
            return;
        }

        try
        {
            await Api.RenameSubscriptionAsync(sub.Id, name);
            await LoadSubscriptionsAsync();
        }
        catch (Exception ex)
        {
            SetStatus($"重命名失败：{ex.Message}", true);
        }
    }

    private async void OnItemCopyUrl(object sender, RoutedEventArgs e)
    {
        if (sender is not MenuFlyoutItem item || item.CommandParameter is not SubscriptionItem sub)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        try
        {
            var url = await Api.CopySubscriptionUrlAsync(sub.Id);
            var data = new DataPackage();
            data.SetText(url);
            Clipboard.SetContent(data);
            SetStatus("已复制 URL。", false);
        }
        catch (Exception ex)
        {
            SetStatus($"复制失败：{ex.Message}", true);
        }
    }

    private void OnItemOpenFile(object sender, RoutedEventArgs e)
    {
        if (sender is not MenuFlyoutItem item || item.CommandParameter is not SubscriptionItem sub)
        {
            return;
        }
        if (string.IsNullOrWhiteSpace(sub.FilePath))
        {
            return;
        }
        try
        {
            var args = $"/select,\"{sub.FilePath}\"";
            Process.Start(new ProcessStartInfo("explorer.exe", args)
            {
                UseShellExecute = true,
            });
        }
        catch (Exception ex)
        {
            SetStatus($"打开失败：{ex.Message}", true);
        }
    }

    private async void OnItemDelete(object sender, RoutedEventArgs e)
    {
        if (sender is not MenuFlyoutItem item || item.CommandParameter is not SubscriptionItem sub)
        {
            return;
        }

        if (!await EnsureBackendReadyAsync())
        {
            return;
        }

        try
        {
            await Api.DeleteSubscriptionAsync(sub.Id);
            await LoadSubscriptionsAsync();
            await LoadConfigAsync();
        }
        catch (Exception ex)
        {
            SetStatus($"删除失败：{ex.Message}", true);
        }
    }

    private void OnSubscriptionListSizeChanged(object sender, SizeChangedEventArgs e)
        => QueueColumnLayout();

    private void QueueColumnLayout()
    {
        if (isLayoutQueued)
        {
            return;
        }

        isLayoutQueued = true;
        DispatcherQueue?.TryEnqueue(() =>
        {
            isLayoutQueued = false;
            UpdateColumnLayout();
        });
    }

    private void UpdateActiveSelection(string? activeId)
    {
        foreach (var item in Subscriptions)
        {
            var isActive = !string.IsNullOrWhiteSpace(activeId)
                && string.Equals(item.Id, activeId, StringComparison.OrdinalIgnoreCase);
            item.IsActive = isActive;
        }
    }

    private void UpdateColumnLayout()
    {
        if (SubscriptionList is null)
        {
            return;
        }

        var width = SubscriptionList.ActualWidth;
        if (width <= 0)
        {
            return;
        }

        var requestedColumns = Math.Clamp(ViewModel.SubscriptionColumns, 1, 3);
        var gap = 14.0;
        var minWidth = 180.0;
        var columns = requestedColumns;
        var available = Math.Max(0, width - gap * (columns - 1));

        while (columns > 1 && available / columns < minWidth)
        {
            columns--;
            available = Math.Max(0, width - gap * (columns - 1));
        }

        var columnWidth = Math.Max(80, available / Math.Max(1, columns));

        if (columns == lastColumns && Math.Abs(columnWidth - lastItemWidth) < 0.5)
        {
            return;
        }

        lastColumns = columns;
        lastItemWidth = columnWidth;
        SubscriptionColumnWidth = columnWidth;
    }

    private void OnItemPointerPressed(object sender, PointerRoutedEventArgs e)
    {
        if (sender is not Grid grid)
        {
            return;
        }
        AnimateIndicatorPressed(grid);
    }

    private void OnItemPointerReleased(object sender, PointerRoutedEventArgs e)
    {
        if (sender is not Grid grid)
        {
            return;
        }
        AnimateIndicator(grid, 1.0, 260);
    }

    private void OnItemPointerCanceled(object sender, PointerRoutedEventArgs e)
        => OnItemPointerReleased(sender, e);

    private void AnimateIndicatorPressed(Grid container)
    {
        if (container.FindName("IndicatorPressScale") is not ScaleTransform transform)
        {
            return;
        }

        var storyboard = new Storyboard();
        var keyframes = new DoubleAnimationUsingKeyFrames
        {
            EnableDependentAnimation = true,
        };
        keyframes.KeyFrames.Add(new EasingDoubleKeyFrame
        {
            KeyTime = KeyTime.FromTimeSpan(TimeSpan.FromMilliseconds(50)),
            Value = 0.35,
            EasingFunction = new SineEase { EasingMode = EasingMode.EaseOut },
        });
        keyframes.KeyFrames.Add(new EasingDoubleKeyFrame
        {
            KeyTime = KeyTime.FromTimeSpan(TimeSpan.FromMilliseconds(140)),
            Value = 0.2,
            EasingFunction = new SineEase { EasingMode = EasingMode.EaseOut },
        });
        Storyboard.SetTarget(keyframes, transform);
        Storyboard.SetTargetProperty(keyframes, "ScaleY");
        storyboard.Children.Add(keyframes);
        storyboard.Begin();
    }

    private void AnimateIndicator(Grid container, double to, int durationMs)
    {
        if (container.FindName("IndicatorPressScale") is not ScaleTransform transform)
        {
            return;
        }

        var animation = new DoubleAnimation
        {
            To = to,
            Duration = TimeSpan.FromMilliseconds(durationMs),
            EasingFunction = new SineEase { EasingMode = EasingMode.EaseOut },
            EnableDependentAnimation = true,
        };
        Storyboard.SetTarget(animation, transform);
        Storyboard.SetTargetProperty(animation, "ScaleY");
        var storyboard = new Storyboard();
        storyboard.Children.Add(animation);
        storyboard.Begin();
    }

    private static List<SubscriptionInput> ParseSubscriptionInput(string raw)
    {
        var results = new List<SubscriptionInput>();
        if (string.IsNullOrWhiteSpace(raw))
        {
            return results;
        }

        string? currentGroup = null;
        var groupIndex = 0;
        var defaultIndex = 0;

        var lines = raw.Split(new[] { "\r\n", "\n", "\r" }, StringSplitOptions.RemoveEmptyEntries);
        foreach (var line in lines)
        {
            var trimmed = line.Trim();
            if (string.IsNullOrWhiteSpace(trimmed))
            {
                continue;
            }

            var matches = UrlRegex.Matches(trimmed);
            if (matches.Count == 0)
            {
                if (IsGroupLabel(trimmed))
                {
                    currentGroup = trimmed;
                    groupIndex = 0;
                }
                continue;
            }

            foreach (Match match in matches)
            {
                var url = TrimUrl(match.Value);
                if (string.IsNullOrWhiteSpace(url))
                {
                    continue;
                }
                if (!Uri.TryCreate(url, UriKind.Absolute, out var uri)
                    || (!string.Equals(uri.Scheme, "http", StringComparison.OrdinalIgnoreCase)
                        && !string.Equals(uri.Scheme, "https", StringComparison.OrdinalIgnoreCase)))
                {
                    continue;
                }

                string name;
                if (!string.IsNullOrWhiteSpace(currentGroup))
                {
                    groupIndex++;
                    name = $"{currentGroup}-{groupIndex}";
                }
                else
                {
                    defaultIndex++;
                    name = $"订阅 {defaultIndex}";
                }

                results.Add(new SubscriptionInput(url, name));
            }
        }

        return results;
    }

    private static bool IsGroupLabel(string line)
    {
        return Regex.IsMatch(line, @"^(订阅|Subscription)\s*\d+", RegexOptions.IgnoreCase);
    }

    private static string TrimUrl(string raw)
    {
        if (string.IsNullOrWhiteSpace(raw))
        {
            return string.Empty;
        }

        var trimmed = raw.Trim();
        return trimmed.TrimEnd(')', ']', '}', ',', ';', '）', '】', '，', '。', '、', '；', '”', '"');
    }

    private void SetStatus(string message, bool isError)
    {
        StatusMessage = isError ? LocalizeError(message) : message;
        StatusSeverity = isError ? InfoBarSeverity.Error : InfoBarSeverity.Informational;
        IsStatusOpen = !string.IsNullOrWhiteSpace(StatusMessage);
    }

    private static string LocalizeError(string message)
    {
        if (string.IsNullOrWhiteSpace(message))
        {
            return "操作失败。";
        }

        return message switch
        {
            "bad_request" => "请求参数无效。",
            "unauthorized" => "未授权，请刷新后重试。",
            "forbidden" => "仅允许本机访问。",
            "mihomo_binary_not_found" => "未找到 mihomo 运行时。程序会先尝试自动下载；如下载不可达，请手动放到应用目录或数据目录的 bin 中。",
            "subscription_url_required" => "订阅 URL 为空。",
            "subscription_not_found" => "订阅不存在。",
            "subscription_url_missing" => "订阅 URL 缺失。",
            "subscription_userinfo_missing" => "订阅流量信息缺失。",
            "file_path_required" => "需要选择配置文件。",
            _ => message,
        };
    }

    private bool SetField<T>(ref T field, T value, [CallerMemberName] string? name = null)
    {
        if (EqualityComparer<T>.Default.Equals(field, value))
        {
            return false;
        }

        field = value;
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
        return true;
    }

    private async Task<bool> EnsureBackendReadyAsync()
    {
        if (await AppState.EnsureBackendReadyAsync())
        {
            return true;
        }

        SetStatus("后台未启动。", true);
        return false;
    }

    private sealed record SubscriptionInput(string Url, string Name);
}

