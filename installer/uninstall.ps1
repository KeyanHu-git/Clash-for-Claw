param(
    [switch]$Quiet
)

$ErrorActionPreference = "Stop"

$packageInfoPath = Join-Path $PSScriptRoot "package-info.json"
$appName = "Clash for Claw"
if (Test-Path $packageInfoPath) {
    try {
        $package = Get-Content $packageInfoPath -Raw | ConvertFrom-Json
        if (-not [string]::IsNullOrWhiteSpace($package.AppName)) {
            $appName = [string]$package.AppName
        }
    }
    catch {
    }
}

$installRoot = $PSScriptRoot
$startMenuPath = Join-Path $env:APPDATA "Microsoft\\Windows\\Start Menu\\Programs\\$appName.lnk"
$uninstallKey = "HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\ClashForClaw"

Get-Process -Name "OpenClawAdapter" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

if (Test-Path $startMenuPath) {
    Remove-Item $startMenuPath -Force -ErrorAction SilentlyContinue
}

if (Test-Path $uninstallKey) {
    Remove-Item $uninstallKey -Recurse -Force -ErrorAction SilentlyContinue
}

$cleanupScript = Join-Path $env:TEMP "ClashForClaw-Uninstall.cmd"
$cleanupContent = @(
    "@echo off",
    "ping 127.0.0.1 -n 2 >nul",
    ('rmdir /s /q "{0}"' -f $installRoot),
    ('del /q "{0}"' -f $cleanupScript)
)
Set-Content -Path $cleanupScript -Value $cleanupContent -Encoding ASCII
Start-Process -FilePath "cmd.exe" -ArgumentList "/c `"$cleanupScript`"" -WindowStyle Hidden | Out-Null

if (-not $Quiet) {
    Write-Output "$appName has been removed."
}
