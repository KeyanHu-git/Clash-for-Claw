using Microsoft.UI;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using WinRT.Interop;

namespace OpenClawAdapter.Services;

public static class WindowManager
{
    public static void Hide(Window window)
    {
        var appWindow = GetAppWindow(window);
        if (appWindow is null)
        {
            return;
        }

        var hideMethod = appWindow.GetType().GetMethod("Hide");
        if (hideMethod is not null)
        {
            hideMethod.Invoke(appWindow, null);
            return;
        }

        var presenter = appWindow.Presenter;
        var minimize = presenter?.GetType().GetMethod("Minimize");
        minimize?.Invoke(presenter, null);
    }

    public static void Show(Window window)
    {
        var appWindow = GetAppWindow(window);
        if (appWindow is null)
        {
            return;
        }

        var showMethod = appWindow.GetType().GetMethod("Show");
        showMethod?.Invoke(appWindow, null);
        window.Activate();
    }

    public static AppWindow? GetAppWindow(Window window)
    {
        var hwnd = WindowNative.GetWindowHandle(window);
        var windowId = Win32Interop.GetWindowIdFromWindow(hwnd);
        return AppWindow.GetFromWindowId(windowId);
    }
}
