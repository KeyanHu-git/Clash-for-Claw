param(
    [string]$OutputPath = "D:\OpenClawAdapter\docs\readme-hero.png",
    [string]$WindowTitle = "Clash for Claw",
    [int]$Padding = 92,
    [int]$TargetWidth = 1320,
    [int]$TargetHeight = 840
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

Add-Type @"
using System;
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
    public const uint SWP_NOSIZE = 0x0001;
    public const uint SWP_NOMOVE = 0x0002;
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

function Save-Capture {
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

$target = Get-TargetProcess -ExpectedTitle $WindowTitle
$handle = [IntPtr]$target.MainWindowHandle
[void][HeroCaptureNative]::ShowWindow($handle, [HeroCaptureNative]::SW_RESTORE)
Start-Sleep -Milliseconds 250

$originalBounds = Get-WindowRectangle -Handle $handle
$screen = [System.Windows.Forms.Screen]::FromRectangle($originalBounds)
$workingArea = $screen.WorkingArea

$captureWidth = $TargetWidth
$captureHeight = $TargetHeight
$windowX = $workingArea.X + [math]::Floor(($workingArea.Width - $captureWidth) / 2)
$windowY = $workingArea.Y + [math]::Floor(($workingArea.Height - $captureHeight) / 2)
$stageWidth = $captureWidth + ($Padding * 2)
$stageHeight = $captureHeight + ($Padding * 2)
$stageX = $windowX - $Padding
$stageY = $windowY - $Padding

$stageForm = New-Object System.Windows.Forms.Form
$stageForm.FormBorderStyle = [System.Windows.Forms.FormBorderStyle]::None
$stageForm.StartPosition = [System.Windows.Forms.FormStartPosition]::Manual
$stageForm.BackColor = [System.Drawing.Color]::White
$stageForm.ShowInTaskbar = $false
$stageForm.TopMost = $true
$stageForm.Bounds = [System.Drawing.Rectangle]::new($stageX, $stageY, $stageWidth, $stageHeight)

try {
    $stageForm.Show()
    $stageForm.Refresh()

    [void][HeroCaptureNative]::SetWindowPos(
        $handle,
        [HeroCaptureNative]::HWND_TOPMOST,
        $windowX,
        $windowY,
        $captureWidth,
        $captureHeight,
        [HeroCaptureNative]::SWP_SHOWWINDOW)
    [void][HeroCaptureNative]::SetForegroundWindow($handle)

    Start-Sleep -Milliseconds 900
    Save-Capture -Bounds ([System.Drawing.Rectangle]::new($stageX, $stageY, $stageWidth, $stageHeight)) -Path $OutputPath
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

    $stageForm.Close()
    $stageForm.Dispose()
}

Write-Output "Saved hero capture to $OutputPath"
