using System;
using System.Diagnostics;
using System.Runtime.InteropServices;
using Microsoft.Graphics.Canvas.UI.Xaml;
using Microsoft.UI;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Animation;
using Microsoft.UI.Xaml.Navigation;
using ClashForClaw.ViewModels;
using WinRT.Interop;
using Windows.Foundation;
using Windows.Graphics;
using Windows.UI;
using Windows.UI.ViewManagement;

namespace ClashForClaw.Views;

public partial class RootPage : Page
{
    private const double DefaultWidthRatio = 0.46;
    private const double DefaultHeightRatio = 0.56;
    private const double MinWidthRatio = 0.24;
    private const double MinHeightRatio = 0.3;
    private const double NavCompactWidth = 76.0;
    private const double NavExpandedWidth = 192.0;
    private const double NavExpandedBreakpointWidth = 780.0;
    private const double NavExpandedIconWidth = 34.0;
    private const double NavPaddingExpanded = 8.0;
    private const double NavContentInset = 20.0;
    private const double NavLabelGapExpanded = 10.0;
    private const int GWLP_WNDPROC = -4;
    private const uint WM_ENTERSIZEMOVE = 0x0231;
    private const uint WM_EXITSIZEMOVE = 0x0232;
    private static readonly TimeSpan JellyfishFrameInterval = TimeSpan.FromSeconds(1.0 / 48.0);
    private static readonly TimeSpan InteractionRestoreDelay = TimeSpan.FromMilliseconds(180);
    private static readonly TimeSpan NavigationSelectionAnimationInterval = TimeSpan.FromMilliseconds(40);
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
    private bool isWindowInteractionActive;
    private bool isWindowMoveSizeLoopActive;
    private CanvasControl? jellyfishCanvas;
    private MathJellyfishRenderer? jellyfishRenderer;
    private DispatcherQueueTimer? interactionRestoreTimer;
    private DispatcherQueueTimer? navigationSelectionAnimationTimer;
    private IntPtr hookedWindowHandle;
    private IntPtr originalWindowProc;
    private WindowProc? windowProc;
    private readonly Stopwatch navigationSelectionAnimationClock = new();
    private NavigationSelectionAnimationTargets? navigationSelectionAnimationTargets;
    private bool isNavigationSelectionAnimationRunning;
    private ListViewItem[] navigationItems = Array.Empty<ListViewItem>();
    private readonly NavigationLayoutBinding[] navigationLayouts;
    private readonly Dictionary<Type, ListViewItem> navigationItemsByPage;
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
        navigationItems =
        [
            NavItemDashboard,
            NavItemSubscriptions,
            NavItemSettings,
            NavItemLogs,
            NavItemGuide,
        ];
        navigationItemsByPage = new Dictionary<Type, ListViewItem>
        {
            [typeof(MainPage)] = NavItemDashboard,
            [typeof(SubscriptionsPage)] = NavItemSubscriptions,
            [typeof(SettingsPage)] = NavItemSettings,
            [typeof(LogsPage)] = NavItemLogs,
            [typeof(GuidePage)] = NavItemGuide,
        };
        ContentFrame.Navigated += OnContentFrameNavigated;
        Unloaded += OnUnloaded;
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
        ApplyNavigationSelectionVisuals(NavList.SelectedItem as ListViewItem);
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
            EnterInteractionVisualMode();
            EnforceMinSize(appWindow);
            UpdateUiScale(appWindow);
            UpdateNavigationLayout();
            ScheduleInteractionVisualRestore();
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
        if (RootScale is not null)
        {
            // Keep the shell in layout space so window resizing does not "pull" the app around.
            RootScale.ScaleX = 1;
            RootScale.ScaleY = 1;
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
        HookWindowInteractionMessages(hwnd);
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
        EnsureInteractionRestoreTimer();

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
        var showDynamicLayer = !highContrast && !isWindowInteractionActive;
        var allowMotion = uiSettings.AnimationsEnabled && showDynamicLayer;
        FluidCanvas.Visibility = showDynamicLayer ? Visibility.Visible : Visibility.Collapsed;
        JellyfishLayer.Visibility = showDynamicLayer ? Visibility.Visible : Visibility.Collapsed;

        if (backgroundStoryboard is not null)
        {
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

        UpdateNavigationSelectionAnimationState();
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

        ApplyNavigationSelectionVisuals(item);
        NavigateToNavigationItem(item);
    }

    private void OnContentFrameNavigated(object sender, NavigationEventArgs e)
    {
        if (e.SourcePageType is null)
        {
            return;
        }

        if (navigationItemsByPage.TryGetValue(e.SourcePageType, out var item)
            && !ReferenceEquals(NavList.SelectedItem, item))
        {
            NavList.SelectedItem = item;
        }

        ApplyNavigationSelectionVisuals(item);
    }

    private void OnShellRootSizeChanged(object sender, SizeChangedEventArgs e)
    {
        UpdateNavigationLayout();
    }

    private void OnUnloaded(object sender, RoutedEventArgs e)
    {
        interactionRestoreTimer?.Stop();
        StopNavigationSelectionAnimation();
        UnhookWindowInteractionMessages();
        StopJellyfish();
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

    private void ApplyNavigationSelectionVisuals(ListViewItem? activeItem)
    {
        var mutedForeground = ResolveBrush("TextMutedBrush", Color.FromArgb(255, 148, 163, 184));
        var selectedForeground = new SolidColorBrush(Colors.White);
        var transparent = new SolidColorBrush(Colors.Transparent);

        foreach (var item in navigationItems)
        {
            if (item is null || item.XamlRoot is null)
            {
                continue;
            }

            item.ApplyTemplate();
            var isActive = ReferenceEquals(item, activeItem);
            item.Background = transparent;
            item.BorderBrush = transparent;
            item.BorderThickness = new Thickness(0);
            item.Foreground = isActive ? selectedForeground : mutedForeground;

            var state = isActive ? "SelectedUnfocused" : "Unselected";
            VisualStateManager.GoToState(item, state, true);
            ApplySelectionChrome(item, isActive);
        }

        navigationSelectionAnimationTargets = activeItem is not null
            ? ResolveNavigationSelectionAnimationTargets(activeItem)
            : null;
        if (navigationSelectionAnimationTargets is not null)
        {
            navigationSelectionAnimationClock.Restart();
        }

        UpdateNavigationSelectionAnimationState();
    }

    private static void ApplySelectionChrome(ListViewItem item, bool isActive)
    {
        SetTemplateOpacity(item, "SelectionLayer", isActive ? 1 : 0);
        SetTemplateOpacity(item, "SelectionRailFlow", isActive ? 0.74 : 0);
        SetTemplateOpacity(item, "SelectionCue", 0);
        SetTemplateOpacity(item, "SelectionSheen", 0);
        if (!isActive)
        {
            SetTemplateOpacity(item, "SelectionAura", 0);
        }
    }

    private static NavigationSelectionAnimationTargets? ResolveNavigationSelectionAnimationTargets(ListViewItem item)
    {
        if (FindNamedElement(item, "SelectionRailHost") is not FrameworkElement railHost
            || FindNamedElement(item, "SelectionRailFlow") is not Border railFlow)
        {
            return null;
        }

        var railFlowTransform = railFlow.RenderTransform as CompositeTransform;
        if (railFlowTransform is null)
        {
            railFlowTransform = new CompositeTransform();
            railFlow.RenderTransform = railFlowTransform;
        }

        return new NavigationSelectionAnimationTargets(railHost, railFlow, railFlowTransform);
    }

    private void EnsureNavigationSelectionAnimationTimer()
    {
        if (navigationSelectionAnimationTimer is not null || DispatcherQueue is null)
        {
            return;
        }

        navigationSelectionAnimationTimer = DispatcherQueue.CreateTimer();
        navigationSelectionAnimationTimer.Interval = NavigationSelectionAnimationInterval;
        navigationSelectionAnimationTimer.IsRepeating = true;
        navigationSelectionAnimationTimer.Tick += (_, _) => ApplyNavigationSelectionAnimationFrame();
    }

    private void UpdateNavigationSelectionAnimationState()
    {
        EnsureNavigationSelectionAnimationTimer();
        if (navigationSelectionAnimationTargets is null)
        {
            StopNavigationSelectionAnimation();
            return;
        }

        if (!uiSettings.AnimationsEnabled || IsHighContrastEnabled() || isWindowInteractionActive)
        {
            StopNavigationSelectionAnimation();
            ApplyNavigationSelectionAnimationFrame(useStaticLayout: true);
            return;
        }

        if (!isNavigationSelectionAnimationRunning)
        {
            navigationSelectionAnimationClock.Restart();
            navigationSelectionAnimationTimer?.Start();
            isNavigationSelectionAnimationRunning = true;
        }

        ApplyNavigationSelectionAnimationFrame();
    }

    private void StopNavigationSelectionAnimation()
    {
        navigationSelectionAnimationTimer?.Stop();
        isNavigationSelectionAnimationRunning = false;
        navigationSelectionAnimationClock.Stop();
    }

    private void ApplyNavigationSelectionAnimationFrame(bool useStaticLayout = false)
    {
        if (navigationSelectionAnimationTargets is null)
        {
            return;
        }

        var railHostWidth = navigationSelectionAnimationTargets.RailHost.ActualWidth;
        var flowWidth = navigationSelectionAnimationTargets.RailFlow.ActualWidth > 0
            ? navigationSelectionAnimationTargets.RailFlow.ActualWidth
            : navigationSelectionAnimationTargets.RailFlow.Width;

        if (railHostWidth <= 0 || flowWidth <= 0)
        {
            navigationSelectionAnimationTargets.RailFlow.Opacity = 0.72;
            navigationSelectionAnimationTargets.RailFlowTransform.TranslateX = 0;
            return;
        }

        var travel = Math.Max(0, railHostWidth - flowWidth);
        if (useStaticLayout)
        {
            navigationSelectionAnimationTargets.RailFlow.Opacity = 0.72;
            navigationSelectionAnimationTargets.RailFlowTransform.TranslateX = travel * 0.18;
            return;
        }

        var time = navigationSelectionAnimationClock.Elapsed.TotalSeconds;
        var phase = (time * 0.72) % 1.0;
        navigationSelectionAnimationTargets.RailFlowTransform.TranslateX = travel * phase;
        navigationSelectionAnimationTargets.RailFlow.Opacity = 0.64 + (0.2 * (0.5 + (0.5 * Math.Sin(time * 6.4))));
    }

    private static void SetTemplateOpacity(Control control, string childName, double opacity)
    {
        if (FindNamedElement(control, childName) is UIElement element)
        {
            element.Opacity = opacity;
        }
    }

    private static FrameworkElement? FindNamedElement(DependencyObject root, string name)
    {
        if (root is FrameworkElement element && string.Equals(element.Name, name, StringComparison.Ordinal))
        {
            return element;
        }

        var childCount = VisualTreeHelper.GetChildrenCount(root);
        for (var index = 0; index < childCount; index++)
        {
            var match = FindNamedElement(VisualTreeHelper.GetChild(root, index), name);
            if (match is not null)
            {
                return match;
            }
        }

        return null;
    }

    private sealed class NavigationSelectionAnimationTargets(
        FrameworkElement railHost,
        Border railFlow,
        CompositeTransform railFlowTransform)
    {
        public FrameworkElement RailHost { get; } = railHost;
        public Border RailFlow { get; } = railFlow;
        public CompositeTransform RailFlowTransform { get; } = railFlowTransform;
    }

    private void EnsureInteractionRestoreTimer()
    {
        if (interactionRestoreTimer is not null || DispatcherQueue is null)
        {
            return;
        }

        interactionRestoreTimer = DispatcherQueue.CreateTimer();
        interactionRestoreTimer.Interval = InteractionRestoreDelay;
        interactionRestoreTimer.Tick += (_, _) =>
        {
            interactionRestoreTimer?.Stop();
            if (!isWindowInteractionActive || isWindowMoveSizeLoopActive)
            {
                return;
            }

            isWindowInteractionActive = false;
            UpdateBackgroundMotion();
        };
    }

    private void EnterInteractionVisualMode()
    {
        EnsureInteractionRestoreTimer();
        interactionRestoreTimer?.Stop();
        if (isWindowInteractionActive)
        {
            return;
        }

        isWindowInteractionActive = true;
        UpdateBackgroundMotion();
    }

    private void ScheduleInteractionVisualRestore()
    {
        EnsureInteractionRestoreTimer();
        interactionRestoreTimer?.Stop();
        interactionRestoreTimer?.Start();
    }

    private void HookWindowInteractionMessages(IntPtr hwnd)
    {
        if (hwnd == IntPtr.Zero || hookedWindowHandle == hwnd)
        {
            return;
        }

        UnhookWindowInteractionMessages();
        windowProc = WindowInteractionWndProc;
        var procPointer = Marshal.GetFunctionPointerForDelegate(windowProc);
        originalWindowProc = SetWindowLongPtrCompat(hwnd, GWLP_WNDPROC, procPointer);
        hookedWindowHandle = hwnd;
    }

    private void UnhookWindowInteractionMessages()
    {
        if (hookedWindowHandle == IntPtr.Zero || originalWindowProc == IntPtr.Zero)
        {
            return;
        }

        SetWindowLongPtrCompat(hookedWindowHandle, GWLP_WNDPROC, originalWindowProc);
        hookedWindowHandle = IntPtr.Zero;
        originalWindowProc = IntPtr.Zero;
        windowProc = null;
    }

    private IntPtr WindowInteractionWndProc(IntPtr hwnd, uint message, IntPtr wParam, IntPtr lParam)
    {
        switch (message)
        {
            case WM_ENTERSIZEMOVE:
                DispatcherQueue.TryEnqueue(() =>
                {
                    isWindowMoveSizeLoopActive = true;
                    EnterInteractionVisualMode();
                });
                break;
            case WM_EXITSIZEMOVE:
                DispatcherQueue.TryEnqueue(() =>
                {
                    isWindowMoveSizeLoopActive = false;
                    ScheduleInteractionVisualRestore();
                });
                break;
        }

        return CallWindowProc(originalWindowProc, hwnd, message, wParam, lParam);
    }

    private static IntPtr SetWindowLongPtrCompat(IntPtr hwnd, int index, IntPtr newProc)
    {
        return IntPtr.Size == 8
            ? SetWindowLongPtr(hwnd, index, newProc)
            : new IntPtr(SetWindowLong(hwnd, index, newProc.ToInt32()));
    }

    private SolidColorBrush ResolveBrush(string resourceKey, Color fallback)
    {
        if (Resources.TryGetValue(resourceKey, out var localValue) && localValue is SolidColorBrush localBrush)
        {
            return new SolidColorBrush(localBrush.Color);
        }

        if (Application.Current?.Resources.TryGetValue(resourceKey, out var appValue) == true
            && appValue is SolidColorBrush appBrush)
        {
            return new SolidColorBrush(appBrush.Color);
        }

        return new SolidColorBrush(fallback);
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

        var isExpanded = shellWidth >= NavExpandedBreakpointWidth;
        var navWidth = isExpanded ? NavExpandedWidth : NavCompactWidth;
        NavColumn.Width = new GridLength(navWidth);

        var listPadding = isExpanded ? NavPaddingExpanded : 0;
        NavList.Padding = new Thickness(listPadding, 10, listPadding, 8);

        var contentWidth = Math.Max(32, navWidth - NavContentInset - (listPadding * 2));
        var revealProgress = isExpanded ? 1.0 : 0.0;
        var iconColumnWidth = isExpanded ? NavExpandedIconWidth : contentWidth;
        var labelGap = isExpanded ? NavLabelGapExpanded : 0;
        var labelWidth = Math.Max(0, contentWidth - iconColumnWidth - labelGap);
        var labelOffset = isExpanded ? NavLabelGapExpanded : 0;

        foreach (var layout in navigationLayouts)
        {
            layout.Apply(contentWidth, iconColumnWidth, labelWidth, revealProgress, labelOffset);
        }
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

    private delegate IntPtr WindowProc(IntPtr hwnd, uint message, IntPtr wParam, IntPtr lParam);

    [DllImport("user32.dll", EntryPoint = "SetWindowLongPtrW", SetLastError = true)]
    private static extern IntPtr SetWindowLongPtr(IntPtr hWnd, int nIndex, IntPtr dwNewLong);

    [DllImport("user32.dll", EntryPoint = "SetWindowLongW", SetLastError = true)]
    private static extern int SetWindowLong(IntPtr hWnd, int nIndex, int dwNewLong);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern IntPtr CallWindowProc(IntPtr lpPrevWndFunc, IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam);
}

