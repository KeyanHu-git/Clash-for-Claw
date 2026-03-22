using System;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;

namespace ClashForClaw.Services;

public sealed class TrayIconManager : IDisposable
{
    private const uint WM_APP = 0x8000;
    private const uint WM_COMMAND = 0x0111;
    private const uint WM_NULL = 0x0000;
    private const uint WM_TRAYICON = WM_APP + 1;
    private const int WM_LBUTTONDBLCLK = 0x0203;
    private const int WM_RBUTTONUP = 0x0205;

    private const uint NIM_ADD = 0x00000000;
    private const uint NIM_MODIFY = 0x00000001;
    private const uint NIM_DELETE = 0x00000002;

    private const uint NIF_MESSAGE = 0x00000001;
    private const uint NIF_ICON = 0x00000002;
    private const uint NIF_TIP = 0x00000004;

    private const uint MF_GRAYED = 0x00000001;
    private const uint MF_STRING = 0x00000000;
    private const uint MF_SEPARATOR = 0x00000800;

    private const uint TPM_NONOTIFY = 0x0080;
    private const uint TPM_RETURNCMD = 0x0100;

    private const uint IMAGE_ICON = 1;
    private const uint LR_LOADFROMFILE = 0x00000010;
    private const uint LR_DEFAULTSIZE = 0x00000040;
    private const int IDI_APPLICATION = 32512;

    private const uint StatusMenuId = 1001;
    private const uint ShowMenuId = 1002;
    private const uint ExitMenuId = 1003;

    private Action? showCallback;
    private Action? exitCallback;
    private Func<string>? statusProvider;

    private bool isInitialized;
    private bool iconVisible;
    private string currentStatus = string.Empty;
    private string windowClassName = string.Empty;

    private ushort windowClassAtom;
    private IntPtr windowHandle;
    private IntPtr iconHandle;
    private bool destroyIconHandle;
    private WindowProc? windowProc;

    public void Initialize(Action showCallback, Action exitCallback, Func<string> statusProvider)
    {
        if (isInitialized)
        {
            return;
        }

        this.showCallback = showCallback;
        this.exitCallback = exitCallback;
        this.statusProvider = statusProvider;
        currentStatus = statusProvider()?.Trim() ?? string.Empty;

        windowProc = WndProc;
        windowClassName = "ClashForClawTray_" + Guid.NewGuid().ToString("N");
        RegisterWindowClass();
        CreateMessageWindow();
        LoadTrayIcon();
        AddOrUpdateTrayIcon(NIM_ADD);

        isInitialized = true;
        iconVisible = true;
    }

    public void UpdateStatus(string status)
    {
        currentStatus = status?.Trim() ?? string.Empty;
        if (!iconVisible)
        {
            return;
        }

        AddOrUpdateTrayIcon(NIM_MODIFY);
    }

    public void Show()
    {
        if (!isInitialized || iconVisible)
        {
            return;
        }

        AddOrUpdateTrayIcon(NIM_ADD);
        iconVisible = true;
    }

    public void Hide()
    {
        if (!iconVisible || windowHandle == IntPtr.Zero)
        {
            return;
        }

        var data = CreateNotifyIconData();
        Shell_NotifyIcon(NIM_DELETE, ref data);
        iconVisible = false;
    }

    public void Dispose()
    {
        Hide();

        if (windowHandle != IntPtr.Zero)
        {
            DestroyWindow(windowHandle);
            windowHandle = IntPtr.Zero;
        }

        if (destroyIconHandle && iconHandle != IntPtr.Zero)
        {
            DestroyIcon(iconHandle);
        }
        iconHandle = IntPtr.Zero;
        destroyIconHandle = false;

        if (windowClassAtom != 0)
        {
            UnregisterClass(windowClassName, GetModuleHandle(null));
            windowClassAtom = 0;
        }

        windowProc = null;
        showCallback = null;
        exitCallback = null;
        statusProvider = null;
        currentStatus = string.Empty;
        windowClassName = string.Empty;
        isInitialized = false;
    }

    private void RegisterWindowClass()
    {
        var proc = windowProc ?? throw new InvalidOperationException("Tray window proc is unavailable.");
        var wc = new WNDCLASSEX
        {
            cbSize = (uint)Marshal.SizeOf<WNDCLASSEX>(),
            lpfnWndProc = Marshal.GetFunctionPointerForDelegate(proc),
            hInstance = GetModuleHandle(null),
            lpszClassName = windowClassName,
        };

        windowClassAtom = RegisterClassEx(ref wc);
        if (windowClassAtom == 0)
        {
            throw new InvalidOperationException("Failed to register tray window class.");
        }
    }

    private void CreateMessageWindow()
    {
        windowHandle = CreateWindowEx(
            0,
            windowClassName,
            "ClashForClawTray",
            0,
            0,
            0,
            0,
            0,
            IntPtr.Zero,
            IntPtr.Zero,
            GetModuleHandle(null),
            IntPtr.Zero);

        if (windowHandle == IntPtr.Zero)
        {
            throw new InvalidOperationException("Failed to create tray message window.");
        }
    }

    private void LoadTrayIcon()
    {
        destroyIconHandle = false;
        iconHandle = IntPtr.Zero;

        try
        {
            var appDir = AppContext.BaseDirectory;
            var candidates = new[]
            {
                Path.Combine(appDir, "Assets", "ClashForClaw.ico"),
                Environment.ProcessPath ?? Process.GetCurrentProcess().MainModule?.FileName ?? string.Empty,
            };

            foreach (var candidate in candidates)
            {
                if (string.IsNullOrWhiteSpace(candidate) || !File.Exists(candidate))
                {
                    continue;
                }

                var loaded = LoadImage(IntPtr.Zero, candidate, IMAGE_ICON, 0, 0, LR_LOADFROMFILE | LR_DEFAULTSIZE);
                if (loaded != IntPtr.Zero)
                {
                    iconHandle = loaded;
                    destroyIconHandle = true;
                    return;
                }
            }
        }
        catch
        {
            // Ignore icon-loading failures and fall back to the stock icon below.
        }

        iconHandle = LoadIcon(IntPtr.Zero, (IntPtr)IDI_APPLICATION);
    }

    private void AddOrUpdateTrayIcon(uint action)
    {
        if (windowHandle == IntPtr.Zero)
        {
            return;
        }

        var data = CreateNotifyIconData();
        if (!Shell_NotifyIcon(action, ref data) && action == NIM_MODIFY)
        {
            Shell_NotifyIcon(NIM_ADD, ref data);
        }
    }

    private NOTIFYICONDATA CreateNotifyIconData()
    {
        return new NOTIFYICONDATA
        {
            cbSize = (uint)Marshal.SizeOf<NOTIFYICONDATA>(),
            hWnd = windowHandle,
            uID = 1,
            uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP,
            uCallbackMessage = WM_TRAYICON,
            hIcon = iconHandle,
            szTip = BuildTooltip(),
        };
    }

    private string BuildTooltip()
    {
        var status = currentStatus;
        if (string.IsNullOrWhiteSpace(status) && statusProvider is not null)
        {
            status = statusProvider()?.Trim() ?? string.Empty;
        }

        var tooltip = string.IsNullOrWhiteSpace(status)
            ? "Clash for Claw"
            : $"Clash for Claw - {status}";

        return tooltip.Length <= 127 ? tooltip : tooltip[..127];
    }

    private IntPtr WndProc(IntPtr hwnd, uint message, UIntPtr wParam, IntPtr lParam)
    {
        switch (message)
        {
            case WM_TRAYICON:
                return HandleTrayMessage(unchecked((uint)lParam.ToInt64()));
            case WM_COMMAND:
                HandleMenuCommand(unchecked((uint)wParam.ToUInt64()) & 0xFFFF);
                return IntPtr.Zero;
            default:
                return DefWindowProc(hwnd, message, wParam, lParam);
        }
    }

    private IntPtr HandleTrayMessage(uint message)
    {
        switch ((int)message)
        {
            case WM_LBUTTONDBLCLK:
                showCallback?.Invoke();
                break;
            case WM_RBUTTONUP:
                ShowContextMenu();
                break;
        }

        return IntPtr.Zero;
    }

    private void ShowContextMenu()
    {
        var menu = CreatePopupMenu();
        if (menu == IntPtr.Zero)
        {
            return;
        }

        try
        {
            AppendMenu(menu, MF_STRING | MF_GRAYED, StatusMenuId, string.IsNullOrWhiteSpace(currentStatus) ? "后台驻留中" : currentStatus);
            AppendMenu(menu, MF_SEPARATOR, 0, null);
            AppendMenu(menu, MF_STRING, ShowMenuId, "显示主界面");
            AppendMenu(menu, MF_STRING, ExitMenuId, "退出");

            if (!GetCursorPos(out var point))
            {
                return;
            }

            SetForegroundWindow(windowHandle);
            var command = TrackPopupMenuEx(menu, TPM_RETURNCMD | TPM_NONOTIFY, point.X, point.Y, windowHandle, IntPtr.Zero);
            PostMessage(windowHandle, WM_NULL, UIntPtr.Zero, IntPtr.Zero);
            if (command != 0)
            {
                HandleMenuCommand(command);
            }
        }
        finally
        {
            DestroyMenu(menu);
        }
    }

    private void HandleMenuCommand(uint command)
    {
        switch (command)
        {
            case ShowMenuId:
                showCallback?.Invoke();
                break;
            case ExitMenuId:
                exitCallback?.Invoke();
                break;
        }
    }

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct WNDCLASSEX
    {
        public uint cbSize;
        public uint style;
        public IntPtr lpfnWndProc;
        public int cbClsExtra;
        public int cbWndExtra;
        public IntPtr hInstance;
        public IntPtr hIcon;
        public IntPtr hCursor;
        public IntPtr hbrBackground;
        [MarshalAs(UnmanagedType.LPWStr)] public string? lpszMenuName;
        [MarshalAs(UnmanagedType.LPWStr)] public string lpszClassName;
        public IntPtr hIconSm;
    }

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct NOTIFYICONDATA
    {
        public uint cbSize;
        public IntPtr hWnd;
        public uint uID;
        public uint uFlags;
        public uint uCallbackMessage;
        public IntPtr hIcon;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 128)] public string szTip;
        public uint dwState;
        public uint dwStateMask;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 256)] public string szInfo;
        public uint uTimeoutOrVersion;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 64)] public string szInfoTitle;
        public uint dwInfoFlags;
        public Guid guidItem;
        public IntPtr hBalloonIcon;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct POINT
    {
        public int X;
        public int Y;
    }

    [UnmanagedFunctionPointer(CallingConvention.Winapi)]
    private delegate IntPtr WindowProc(IntPtr hwnd, uint message, UIntPtr wParam, IntPtr lParam);

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern IntPtr GetModuleHandle(string? moduleName);

    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern ushort RegisterClassEx([In] ref WNDCLASSEX lpwcx);

    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool UnregisterClass(string lpClassName, IntPtr hInstance);

    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern IntPtr CreateWindowEx(
        uint dwExStyle,
        string lpClassName,
        string lpWindowName,
        uint dwStyle,
        int x,
        int y,
        int nWidth,
        int nHeight,
        IntPtr hWndParent,
        IntPtr hMenu,
        IntPtr hInstance,
        IntPtr lpParam);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool DestroyWindow(IntPtr hWnd);

    [DllImport("user32.dll")]
    private static extern IntPtr DefWindowProc(IntPtr hWnd, uint msg, UIntPtr wParam, IntPtr lParam);

    [DllImport("shell32.dll", CharSet = CharSet.Unicode)]
    private static extern bool Shell_NotifyIcon(uint dwMessage, [In] ref NOTIFYICONDATA lpData);

    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern IntPtr LoadImage(IntPtr hInst, string name, uint type, int cx, int cy, uint fuLoad);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern IntPtr LoadIcon(IntPtr hInstance, IntPtr lpIconName);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool DestroyIcon(IntPtr hIcon);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern IntPtr CreatePopupMenu();

    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern bool AppendMenu(IntPtr hMenu, uint uFlags, uint uIDNewItem, string? lpNewItem);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool DestroyMenu(IntPtr hMenu);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern uint TrackPopupMenuEx(IntPtr hmenu, uint fuFlags, int x, int y, IntPtr hwnd, IntPtr lptpm);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool GetCursorPos(out POINT lpPoint);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool SetForegroundWindow(IntPtr hWnd);

    [DllImport("user32.dll", SetLastError = true)]
    private static extern bool PostMessage(IntPtr hWnd, uint msg, UIntPtr wParam, IntPtr lParam);
}
