using System;
using System.Diagnostics;
using System.IO;
using System.Threading.Tasks;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Windows.Storage.Pickers;
using WinRT.Interop;
using ClashForClaw.Services;
using ClashForClaw.ViewModels;

namespace ClashForClaw.Views;

public partial class LogsPage : Page
{
    public MainViewModel ViewModel { get; } = AppState.ViewModel;

    public LogsPage()
    {
        InitializeComponent();
        DataContext = ViewModel;
        ApplyAdaptiveLayout();
    }

    private void OnPageSizeChanged(object sender, SizeChangedEventArgs e)
    {
        ApplyAdaptiveLayout(e.NewSize.Width);
    }

    private void ApplyAdaptiveLayout(double? widthOverride = null)
    {
        var width = widthOverride ?? ActualWidth;
        if (width >= 780)
        {
            LogsColumnB.Width = new GridLength(1, GridUnitType.Star);
            Grid.SetColumn(AdvancedCard, 1);
            Grid.SetRow(AdvancedCard, 0);
            return;
        }

        LogsColumnB.Width = new GridLength(0);
        Grid.SetColumn(AdvancedCard, 0);
        Grid.SetRow(AdvancedCard, 1);
    }

    private async void OnPickLogDirectoryClicked(object sender, RoutedEventArgs e)
    {
        if (App.MainWindow is null)
        {
            await ShowMessageAsync("无法打开目录选择器", "主窗口尚未准备完成，请稍后重试。");
            return;
        }

        var picker = new FolderPicker();
        picker.FileTypeFilter.Add("*");

        var hwnd = WindowNative.GetWindowHandle(App.MainWindow);
        InitializeWithWindow.Initialize(picker, hwnd);

        var folder = await picker.PickSingleFolderAsync();
        if (folder is null)
        {
            return;
        }

        var selectedPath = AppPaths.ResolveDesktopLogDirectory(folder.Path);
        if (!AppPaths.TryEnsureWritableDirectory(selectedPath, out var errorMessage))
        {
            await ShowMessageAsync("无法使用该目录", $"当前目录不可写入，请重新选择。\n\n{errorMessage}");
            return;
        }

        SettingsStore.Update(settings => settings.LogDirectory = AppPaths.NormalizeLogDirectorySetting(selectedPath));
    }

    private void OnResetLogDirectoryClicked(object sender, RoutedEventArgs e)
    {
        SettingsStore.Update(settings => settings.LogDirectory = string.Empty);
    }

    private async void OnOpenLogDirectoryClicked(object sender, RoutedEventArgs e)
    {
        await OpenDirectoryAsync(ViewModel.LogDirectoryPath);
    }

    private async void OnOpenServiceLogDirectoryClicked(object sender, RoutedEventArgs e)
    {
        await OpenDirectoryAsync(ViewModel.ServiceLogDirectory);
    }

    private async Task OpenDirectoryAsync(string directoryPath)
    {
        try
        {
            Directory.CreateDirectory(directoryPath);
            Process.Start(new ProcessStartInfo
            {
                FileName = directoryPath,
                UseShellExecute = true,
                Verb = "open",
            });
        }
        catch (Exception ex)
        {
            await ShowMessageAsync("无法打开目录", ex.Message);
        }
    }

    private async Task ShowMessageAsync(string title, string message)
    {
        var root = XamlRoot ?? (App.MainWindow?.Content as FrameworkElement)?.XamlRoot;
        if (root is null)
        {
            return;
        }

        var dialog = new ContentDialog
        {
            Title = title,
            Content = message,
            CloseButtonText = "知道了",
            XamlRoot = root,
        };

        await dialog.ShowAsync();
    }
}
