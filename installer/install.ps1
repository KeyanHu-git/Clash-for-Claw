param(
    [switch]$Quiet
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.IO.Compression.FileSystem

function Stop-AppProcess {
    "ClashForClaw", "OpenClawAdapter" | ForEach-Object {
        Get-Process -Name $_ -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    }
}

function New-Shortcut {
    param(
        [Parameter(Mandatory = $true)][string]$ShortcutPath,
        [Parameter(Mandatory = $true)][string]$TargetPath,
        [Parameter(Mandatory = $true)][string]$WorkingDirectory,
        [Parameter(Mandatory = $true)][string]$IconPath
    )

    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($ShortcutPath)
    $shortcut.TargetPath = $TargetPath
    $shortcut.WorkingDirectory = $WorkingDirectory
    $shortcut.IconLocation = $IconPath
    $shortcut.Save()
}

$packageInfoPath = Join-Path $PSScriptRoot "package-info.json"
if (-not (Test-Path $packageInfoPath)) {
    throw "Missing package-info.json."
}

$package = Get-Content $packageInfoPath -Raw | ConvertFrom-Json
$payloadZip = Join-Path $PSScriptRoot "payload.zip"
if (-not (Test-Path $payloadZip)) {
    throw "Missing payload.zip."
}

$appName = [string]$package.AppName
$publisher = [string]$package.Publisher
$version = [string]$package.Version
$installRoot = Join-Path $env:LOCALAPPDATA "Programs\\$appName"
$extractRoot = Join-Path $env:TEMP ("ClashForClawInstall_" + [guid]::NewGuid().ToString("N"))
$payloadRoot = Join-Path $extractRoot "payload"
$appExe = Join-Path $installRoot "ClashForClaw.exe"
$iconPath = Join-Path $installRoot "Assets\\ClashForClaw.ico"
$startMenuDir = Join-Path $env:APPDATA "Microsoft\\Windows\\Start Menu\\Programs"
$shortcutPath = Join-Path $startMenuDir "$appName.lnk"
$uninstallKey = "HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\ClashForClaw"

try {
    Stop-AppProcess

    if (Test-Path $extractRoot) {
        Remove-Item $extractRoot -Recurse -Force
    }

    New-Item -ItemType Directory -Path $payloadRoot -Force | Out-Null
    Expand-Archive -Path $payloadZip -DestinationPath $payloadRoot -Force

    New-Item -ItemType Directory -Path $installRoot -Force | Out-Null
    $null = robocopy $payloadRoot $installRoot /MIR /NFL /NDL /NJH /NJS /NP
    if ($LASTEXITCODE -gt 7) {
        throw "robocopy failed with exit code $LASTEXITCODE."
    }

    Copy-Item (Join-Path $PSScriptRoot "uninstall.ps1") (Join-Path $installRoot "uninstall.ps1") -Force
    Copy-Item (Join-Path $PSScriptRoot "uninstall.cmd") (Join-Path $installRoot "uninstall.cmd") -Force
    Copy-Item $packageInfoPath (Join-Path $installRoot "package-info.json") -Force

    New-Item -ItemType Directory -Path $startMenuDir -Force | Out-Null
    New-Shortcut -ShortcutPath $shortcutPath -TargetPath $appExe -WorkingDirectory $installRoot -IconPath $iconPath

    New-Item -Path $uninstallKey -Force | Out-Null
    Set-ItemProperty -Path $uninstallKey -Name "DisplayName" -Value $appName
    Set-ItemProperty -Path $uninstallKey -Name "DisplayVersion" -Value $version
    Set-ItemProperty -Path $uninstallKey -Name "Publisher" -Value $publisher
    Set-ItemProperty -Path $uninstallKey -Name "InstallLocation" -Value $installRoot
    Set-ItemProperty -Path $uninstallKey -Name "DisplayIcon" -Value $appExe
    Set-ItemProperty -Path $uninstallKey -Name "UninstallString" -Value ('"{0}"' -f (Join-Path $installRoot "uninstall.cmd"))
    Set-ItemProperty -Path $uninstallKey -Name "QuietUninstallString" -Value ('"{0}" /quiet' -f (Join-Path $installRoot "uninstall.cmd"))
    Set-ItemProperty -Path $uninstallKey -Name "NoModify" -Value 1 -Type DWord
    Set-ItemProperty -Path $uninstallKey -Name "NoRepair" -Value 1 -Type DWord

    if (-not $Quiet) {
        Start-Process -FilePath $appExe -WorkingDirectory $installRoot | Out-Null
    }
}
finally {
    if (Test-Path $extractRoot) {
        Remove-Item $extractRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
