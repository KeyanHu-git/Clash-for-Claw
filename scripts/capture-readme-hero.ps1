param(
    [string]$OutputPath = "D:\OpenClawAdapter\artifacts\readme-window-raw.png",
    [string]$WindowTitle = "Clash for Claw",
    [int]$TargetWidth = 1320,
    [int]$TargetHeight = 840
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Windows.Forms

Add-Type @"
using System;
using System.Drawing;
using System.Runtime.InteropServices;

public static class HeroCaptureNative
{
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT
    {
        public int Left;
        public int Top;
        public int Right;
        public int Bottom;
    }

    [DllImport("user32.dll")]
    public static extern bool SetForegroundWindow(IntPtr hWnd);

    [DllImport("user32.dll")]
    public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);

    [DllImport("user32.dll", SetLastError = true)]
    public static extern bool SetWindowPos(
        IntPtr hWnd,
        IntPtr hWndInsertAfter,
        int X,
        int Y,
        int cx,
        int cy,
        uint uFlags);

    [DllImport("user32.dll", SetLastError = true)]
    public static extern bool GetWindowRect(IntPtr hWnd, out RECT lpRect);

    [DllImport("user32.dll")]
    public static extern bool PrintWindow(IntPtr hwnd, IntPtr hdcBlt, uint nFlags);

    [DllImport("dwmapi.dll")]
    public static extern int DwmGetWindowAttribute(
        IntPtr hwnd,
        int dwAttribute,
        out RECT pvAttribute,
        int cbAttribute);

    public static readonly IntPtr HWND_TOPMOST = new IntPtr(-1);
    public static readonly IntPtr HWND_NOTOPMOST = new IntPtr(-2);
    public const int SW_RESTORE = 9;
    public const int DWMWA_EXTENDED_FRAME_BOUNDS = 9;
    public const uint PW_RENDERFULLCONTENT = 0x00000002;
    public const uint SWP_SHOWWINDOW = 0x0040;
}
"@

function Get-TargetProcess {
    param([string]$ExpectedTitle)

    $deadline = (Get-Date).AddSeconds(20)
    while ((Get-Date) -lt $deadline) {
        $process = Get-Process | Where-Object {
            $_.MainWindowHandle -ne 0 -and $_.MainWindowTitle -like "*$ExpectedTitle*"
        } | Select-Object -First 1

        if ($process) {
            return $process
        }

        Start-Sleep -Milliseconds 300
    }

    throw "Could not find a visible window titled '$ExpectedTitle'."
}

function Get-WindowRectangle {
    param([IntPtr]$Handle)

    $rect = New-Object HeroCaptureNative+RECT
    $rectSize = 16
    $result = [HeroCaptureNative]::DwmGetWindowAttribute(
        $Handle,
        [HeroCaptureNative]::DWMWA_EXTENDED_FRAME_BOUNDS,
        [ref]$rect,
        $rectSize)

    if ($result -ne 0) {
        [void][HeroCaptureNative]::GetWindowRect($Handle, [ref]$rect)
    }

    return [System.Drawing.Rectangle]::FromLTRB($rect.Left, $rect.Top, $rect.Right, $rect.Bottom)
}

function Save-ScreenFallback {
    param(
        [System.Drawing.Rectangle]$Bounds,
        [string]$Path
    )

    $bitmap = New-Object System.Drawing.Bitmap $Bounds.Width, $Bounds.Height
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    try {
        $graphics.CopyFromScreen($Bounds.Location, [System.Drawing.Point]::Empty, $Bounds.Size)
        $bitmap.Save($Path, [System.Drawing.Imaging.ImageFormat]::Png)
    }
    finally {
        $graphics.Dispose()
        $bitmap.Dispose()
    }
}

function Save-WindowCapture {
    param(
        [IntPtr]$Handle,
        [System.Drawing.Rectangle]$Bounds,
        [string]$Path
    )

    $bitmap = New-Object System.Drawing.Bitmap $Bounds.Width, $Bounds.Height, ([System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    $graphics.Clear([System.Drawing.Color]::Transparent)
    $hdc = $graphics.GetHdc()

    try {
        $rendered = [HeroCaptureNative]::PrintWindow($Handle, $hdc, [HeroCaptureNative]::PW_RENDERFULLCONTENT)
    }
    finally {
        $graphics.ReleaseHdc($hdc)
        $graphics.Dispose()
    }

    try {
        if ($rendered) {
            $bitmap.Save($Path, [System.Drawing.Imaging.ImageFormat]::Png)
            return
        }
    }
    finally {
        $bitmap.Dispose()
    }

    Save-ScreenFallback -Bounds $Bounds -Path $Path
}

$target = Get-TargetProcess -ExpectedTitle $WindowTitle
$handle = [IntPtr]$target.MainWindowHandle
[void][HeroCaptureNative]::ShowWindow($handle, [HeroCaptureNative]::SW_RESTORE)
Start-Sleep -Milliseconds 250

$originalBounds = Get-WindowRectangle -Handle $handle
$screen = [System.Windows.Forms.Screen]::FromRectangle($originalBounds)
$workingArea = $screen.WorkingArea

$windowX = $workingArea.X + [math]::Floor(($workingArea.Width - $TargetWidth) / 2)
$windowY = $workingArea.Y + [math]::Floor(($workingArea.Height - $TargetHeight) / 2)

try {
    [void][HeroCaptureNative]::SetWindowPos(
        $handle,
        [HeroCaptureNative]::HWND_TOPMOST,
        $windowX,
        $windowY,
        $TargetWidth,
        $TargetHeight,
        [HeroCaptureNative]::SWP_SHOWWINDOW)
    [void][HeroCaptureNative]::SetForegroundWindow($handle)

    Start-Sleep -Milliseconds 1000

    $captureBounds = Get-WindowRectangle -Handle $handle
    $parent = Split-Path -Parent $OutputPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Path $parent -Force | Out-Null
    }

    Save-WindowCapture -Handle $handle -Bounds $captureBounds -Path $OutputPath
}
finally {
    [void][HeroCaptureNative]::SetWindowPos(
        $handle,
        [HeroCaptureNative]::HWND_NOTOPMOST,
        $originalBounds.X,
        $originalBounds.Y,
        $originalBounds.Width,
        $originalBounds.Height,
        [HeroCaptureNative]::SWP_SHOWWINDOW)
}

Write-Output "Saved raw window capture to $OutputPath"
