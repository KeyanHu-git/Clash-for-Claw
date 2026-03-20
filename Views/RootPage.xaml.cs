using System;
using System.Diagnostics;
using System.Runtime.InteropServices;
using Microsoft.Graphics.Canvas.UI.Xaml;
using Microsoft.UI;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Animation;
using OpenClawAdapter.ViewModels;
using WinRT.Interop;
using Windows.Foundation;
using Windows.Graphics;
using Windows.UI.ViewManagement;

namespace OpenClawAdapter.Views;

public partial class RootPage : Page
{
    private const double DefaultWidthRatio = 0.3;
    private const double DefaultHeightRatio = 0.3;
    private const double MinWidthRatio = 0.1875;
    private const double MinHeightRatio = 0.22;
    private const double NavCompactWidth = 72.0;
    private const double NavExpandedWidth = 186.0;
    private const double NavTransitionStartWidth = 520.0;
    private const double NavTransitionEndWidth = 980.0;
    private const double NavRevealStartWidth = 96.0;
    private const double NavRevealEndWidth = 156.0;
    private const double NavExpandedIconWidth = 34.0;
    private const double NavPaddingExpanded = 8.0;
    private const double NavContentInset = 20.0;
    private const double NavLabelGapExpanded = 10.0;
    private static readonly TimeSpan JellyfishFrameInterval = TimeSpan.FromSeconds(1.0 / 120.0);
    private static readonly Type DefaultPageType = typeof(MainPage);
    private static readonly Dictionary<string, Type> NavigationPages = new(StringComparer.Ordinal)
    {
        ["dashboard"] = typeof(MainPage),
        ["subscriptions"] = typeof(SubscriptionsPage),
        ["settings"] = typeof(SettingsPage),
        ["logs"] = typeof(LogsPage),
        ["guide"] = typeof(GuidePage),
    };

    private int minWindowWidth = 480;
    private int minWindowHeight = 320;
    private int defaultWindowWidth = 900;
    private int defaultWindowHeight = 520;
    private bool isInitialized;
    private bool isResizing;
    private bool isXamlRootHooked;
    private Storyboard? backgroundStoryboard;
    private readonly UISettings uiSettings = new();
    private readonly AccessibilitySettings accessibilitySettings = new();
    private bool jellyfishInitialized;
    private bool jellyfishRendering;
    private readonly Stopwatch jellyfishClock = new();
    private readonly Stopwatch renderClock = new();
    private TimeSpan lastRenderTime;
    private bool allowJellyfishMotion;
    private CanvasControl? jellyfishCanvas;
    private MathJellyfishRenderer? jellyfishRenderer;
    private readonly NavigationLayoutBinding[] navigationLayouts;
    public MainViewModel ViewModel { get; } = AppState.ViewModel;

    public RootPage()
    {
        InitializeComponent();
        DataContext = ViewModel;
        navigationLayouts =
        [
            new(NavGridDashboard, NavIconColumnDashboard, NavLabelColumnDashboard, NavLabelDashboard),
            new(NavGridSubscriptions, NavIconColumnSubscriptions, NavLabelColumnSubscriptions, NavLabelSubscriptions),
            new(NavGridSettings, NavIconColumnSettings, NavLabelColumnSettings, NavLabelSettings),
            new(NavGridLogs, NavIconColumnLogs, NavLabelColumnLogs, NavLabelLogs),
            new(NavGridGuide, NavIconColumnGuide, NavLabelColumnGuide, NavLabelGuide),
        ];
    }

    private void OnLoaded(object sender, RoutedEventArgs e)
    {
        if (isInitialized)
        {
            return;
        }

        isInitialized = true;
        ConfigureWindowChrome();
        ConfigureBackgroundMotion();
        EnsureInitialNavigation();

        if (!isXamlRootHooked && XamlRoot is not null)
        {
            isXamlRootHooked = true;
            XamlRoot.Changed += OnXamlRootChanged;
        }

        UpdateNavigationLayout();
    }

    private void ConfigureWindowChrome()
    {
        var window = App.MainWindow;
        if (window is null)
        {
            return;
        }

        window.ExtendsContentIntoTitleBar = true;
        window.SetTitleBar(AppTitleBar);

        var hwnd = WindowNative.GetWindowHandle(window);
        var windowId = Win32Interop.GetWindowIdFromWindow(hwnd);
        var appWindow = AppWindow.GetFromWindowId(windowId);
        var titleBar = appWindow.TitleBar;

        titleBar.ButtonBackgroundColor = Colors.Transparent;
        titleBar.ButtonInactiveBackgroundColor = Colors.Transparent;

        UpdateWindowMetrics(appWindow);
        UpdateUiScale(appWindow);

        var targetSize = new SizeInt32(defaultWindowWidth, defaultWindowHeight);
        if (appWindow.Size.Width <= 0 || appWindow.Size.Height <= 0)
        {
            appWindow.Resize(targetSize);
        }
        else if (appWindow.Size.Width > targetSize.Width || appWindow.Size.Height > targetSize.Height)
        {
            appWindow.Resize(targetSize);
        }

        window.SizeChanged += (_, _) =>
        {
            EnforceMinSize(appWindow);
            UpdateUiScale(appWindow);
            UpdateNavigationLayout();
        };
        EnforceMinSize(appWindow);
        DispatcherQueue.TryEnqueue(() =>
        {
            UpdateUiScale(appWindow);
            UpdateNavigationLayout();
        });
        UpdateTitleBarLayout(titleBar);
    }

    private void UpdateWindowMetrics(AppWindow appWindow)
    {
        var displayArea = DisplayArea.GetFromWindowId(appWindow.Id, DisplayAreaFallback.Primary);
        if (displayArea.WorkArea.Width > 0 && displayArea.WorkArea.Height > 0)
        {
            defaultWindowWidth = Math.Max(1, (int)Math.Round(displayArea.WorkArea.Width * DefaultWidthRatio));
            minWindowWidth = Math.Max(360, (int)Math.Round(displayArea.WorkArea.Width * MinWidthRatio));
            defaultWindowHeight = Math.Max(1, (int)Math.Round(displayArea.WorkArea.Height * DefaultHeightRatio));
            minWindowHeight = Math.Max(280, (int)Math.Round(displayArea.WorkArea.Height * MinHeightRatio));
        }
    }

    private void UpdateUiScale(AppWindow appWindow)
    {
        var displayArea = DisplayArea.GetFromWindowId(appWindow.Id, DisplayAreaFallback.Primary);
        if (displayArea.WorkArea.Width <= 0)
        {
            return;
        }

        var xamlRoot = XamlRoot;
        if (xamlRoot is null)
        {
            return;
        }

        var raster = xamlRoot.RasterizationScale;
        if (raster <= 0)
        {
            raster = 1;
        }

        var logicalWidth = displayArea.WorkArea.Width / raster;
        var scale = logicalWidth / 1800.0;
        scale = Math.Clamp(scale, 0.75, 1.0);

        if (RootScale is not null)
        {
            RootScale.ScaleX = scale;
            RootScale.ScaleY = scale;
        }
    }

    private void OnXamlRootChanged(XamlRoot sender, XamlRootChangedEventArgs args)
    {
        var window = App.MainWindow;
        if (window is null)
        {
            return;
        }

        var hwnd = WindowNative.GetWindowHandle(window);
        var windowId = Win32Interop.GetWindowIdFromWindow(hwnd);
        var appWindow = AppWindow.GetFromWindowId(windowId);
        UpdateUiScale(appWindow);
        UpdateNavigationLayout();
    }

    private void EnforceMinSize(AppWindow appWindow)
    {
        if (isResizing)
        {
            return;
        }

        var size = appWindow.Size;
        var targetWidth = size.Width < minWindowWidth ? minWindowWidth : size.Width;
        var targetHeight = size.Height < minWindowHeight ? minWindowHeight : size.Height;
        if (targetWidth != size.Width || targetHeight != size.Height)
        {
            isResizing = true;
            appWindow.Resize(new SizeInt32(targetWidth, targetHeight));
            isResizing = false;
        }
    }

    private void UpdateTitleBarLayout(AppWindowTitleBar titleBar)
    {
        var left = titleBar.LeftInset + 6;
        var right = titleBar.RightInset + 6;
        AppTitleBar.Margin = new Thickness(left, 3, right, 2);
    }

    private void ConfigureBackgroundMotion()
    {
        backgroundStoryboard = (Storyboard)Resources["FluidMotionStoryboard"];
        ConfigureJellyfishLayer();

        if (OperatingSystem.IsWindowsVersionAtLeast(10, 0, 19041))
        {
            uiSettings.AnimationsEnabledChanged += (_, _) => DispatcherQueue.TryEnqueue(UpdateBackgroundMotion);
        }

        try
        {
            accessibilitySettings.HighContrastChanged += (_, _) => DispatcherQueue.TryEnqueue(UpdateBackgroundMotion);
        }
        catch (COMException)
        {
            // Some systems throw when subscribing; fall back to polling only.
        }

        UpdateBackgroundMotion();
    }

    private void UpdateBackgroundMotion()
    {
        var highContrast = IsHighContrastEnabled();
        var allowMotion = uiSettings.AnimationsEnabled && !highContrast;
        var showLayer = !highContrast;
        FluidCanvas.Visibility = showLayer ? Visibility.Visible : Visibility.Collapsed;
        JellyfishLayer.Visibility = showLayer ? Visibility.Visible : Visibility.Collapsed;

        if (backgroundStoryboard is null)
        {
            return;
        }

        if (allowMotion)
        {
            backgroundStoryboard.Begin();
            StartJellyfish();
        }
        else
        {
            backgroundStoryboard.Stop();
            StopJellyfish();
            jellyfishCanvas?.Invalidate();
        }
    }

    private void ConfigureJellyfishLayer()
    {
        if (jellyfishInitialized)
        {
            return;
        }

        jellyfishInitialized = true;

        jellyfishCanvas = new CanvasControl
        {
            IsHitTestVisible = false,
            HorizontalAlignment = HorizontalAlignment.Stretch,
            VerticalAlignment = VerticalAlignment.Stretch,
        };
        jellyfishCanvas.Draw += OnJellyfishDraw;
        jellyfishCanvas.SizeChanged += (_, _) => jellyfishCanvas.Invalidate();
        JellyfishLayer.Children.Clear();
        JellyfishLayer.Children.Add(jellyfishCanvas);

        jellyfishRenderer = new MathJellyfishRenderer();
        ActualThemeChanged += (_, _) =>
        {
            jellyfishRenderer?.UpdatePalette();
            jellyfishCanvas?.Invalidate();
        };
    }

    private void StartJellyfish()
    {
        allowJellyfishMotion = true;
        if (!jellyfishRendering)
        {
            CompositionTarget.Rendering += OnJellyfishRendering;
            jellyfishRendering = true;
        }
        jellyfishClock.Restart();
        renderClock.Restart();
        lastRenderTime = TimeSpan.Zero;
        jellyfishCanvas?.Invalidate();
    }

    private void StopJellyfish()
    {
        allowJellyfishMotion = false;
        if (jellyfishRendering)
        {
            CompositionTarget.Rendering -= OnJellyfishRendering;
            jellyfishRendering = false;
        }
        jellyfishClock.Stop();
        renderClock.Stop();
    }

    private void OnJellyfishRendering(object? sender, object e)
    {
        if (!allowJellyfishMotion)
        {
            return;
        }

        var elapsed = renderClock.Elapsed;
        if (elapsed - lastRenderTime < JellyfishFrameInterval)
        {
            return;
        }

        lastRenderTime = elapsed;
        jellyfishCanvas?.Invalidate();
    }

    private void OnJellyfishDraw(CanvasControl sender, CanvasDrawEventArgs args)
    {
        if (jellyfishRenderer is null)
        {
            return;
        }

        var bounds = new Size(sender.ActualWidth, sender.ActualHeight);
        var timeSeconds = allowJellyfishMotion ? jellyfishClock.Elapsed.TotalSeconds : 0;

        jellyfishRenderer.Draw(args.DrawingSession, bounds, timeSeconds);
    }

    private bool IsHighContrastEnabled()
    {
        try
        {
            return accessibilitySettings.HighContrast;
        }
        catch (COMException)
        {
            return false;
        }
    }

    private void OnNavSelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (NavList.SelectedItem is not ListViewItem item)
        {
            return;
        }

        NavigateToNavigationItem(item);
    }

    private void OnShellRootSizeChanged(object sender, SizeChangedEventArgs e)
    {
        UpdateNavigationLayout();
    }

    private void EnsureInitialNavigation()
    {
        if (NavList.SelectedItem is not ListViewItem)
        {
            NavList.SelectedIndex = 0;
            return;
        }

        NavigateToNavigationItem((ListViewItem)NavList.SelectedItem);
    }

    private void NavigateToNavigationItem(ListViewItem item)
    {
        if (ContentFrame is null || item.Tag is not string tag)
        {
            return;
        }

        var targetPage = ResolvePageType(tag);
        if (ContentFrame.CurrentSourcePageType == targetPage)
        {
            return;
        }

        ContentFrame.Navigate(targetPage);
    }

    private static Type ResolvePageType(string tag)
    {
        return NavigationPages.TryGetValue(tag, out var pageType) ? pageType : DefaultPageType;
    }

    private void UpdateNavigationLayout()
    {
        if (ShellRoot is null || NavColumn is null || NavList is null)
        {
            return;
        }

        var shellWidth = ShellRoot.ActualWidth;
        if (shellWidth <= 0)
        {
            return;
        }

        var navProgress = SmoothStep(NavTransitionStartWidth, NavTransitionEndWidth, shellWidth);
        var navWidth = Lerp(NavCompactWidth, NavExpandedWidth, navProgress);
        NavColumn.Width = new GridLength(navWidth);

        var listPadding = Lerp(0, NavPaddingExpanded, navProgress);
        NavList.Padding = new Thickness(listPadding, 10, listPadding, 8);

        // Keep icon centering and label reveal on the same continuous curve so resize never "snaps".
        var contentWidth = Math.Max(32, navWidth - NavContentInset - (listPadding * 2));
        var revealProgress = SmoothStep(NavRevealStartWidth, NavRevealEndWidth, navWidth);
        var iconColumnWidth = Lerp(contentWidth, NavExpandedIconWidth, revealProgress);
        var labelGap = Lerp(0, NavLabelGapExpanded, revealProgress);
        var labelWidth = Math.Max(0, contentWidth - iconColumnWidth - labelGap);
        var labelOffset = Lerp(0, NavLabelGapExpanded, revealProgress);

        foreach (var layout in navigationLayouts)
        {
            layout.Apply(contentWidth, iconColumnWidth, labelWidth, revealProgress, labelOffset);
        }
    }

    private static double Lerp(double start, double end, double progress)
    {
        return start + ((end - start) * progress);
    }

    private static double SmoothStep(double start, double end, double value)
    {
        if (value <= start)
        {
            return 0;
        }

        if (value >= end)
        {
            return 1;
        }

        var progress = (value - start) / (end - start);
        return progress * progress * (3 - (2 * progress));
    }

    private sealed class NavigationLayoutBinding(
        Grid container,
        ColumnDefinition iconColumn,
        ColumnDefinition labelColumn,
        TextBlock label)
    {
        public void Apply(double contentWidth, double iconWidth, double labelWidth, double labelOpacity, double labelOffset)
        {
            container.Width = contentWidth;
            iconColumn.Width = new GridLength(iconWidth);
            labelColumn.Width = new GridLength(labelWidth);
            label.Opacity = labelOpacity;
            label.Margin = new Thickness(labelOffset, 0, 0, 0);
        }
    }
}
