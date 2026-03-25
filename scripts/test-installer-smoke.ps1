param(
    [Parameter(Mandatory = $true)][string]$SetupPath,
    [Parameter(Mandatory = $true)][string]$InstallDir
)

$ErrorActionPreference = "Stop"

$runKeyPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
$previousRunValue = (Get-ItemProperty $runKeyPath -ErrorAction SilentlyContinue).ClashForClaw

function Get-RunningServicePid {
    $output = sc.exe queryex ClashForClaw 2>$null
    if (-not $output) {
        return $null
    }

    $joined = $output -join "`n"
    if ($joined -notmatch 'STATE\s+: 4\s+RUNNING') {
        return $null
    }

    if ($joined -match 'PID\s+: (\d+)') {
        return [int]$Matches[1]
    }

    return $null
}

function Get-InstalledProduct {
    return Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*' -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -like 'Clash for Claw*' } |
        Select-Object -First 1
}

try {
    $servicePid = Get-RunningServicePid
    $installedProduct = Get-InstalledProduct
    if ($servicePid -or $installedProduct) {
        [pscustomobject]@{
            Skipped = $true
            Reason = if ($servicePid) { "service_or_existing_install_active" } else { "existing_install_active" }
            ServicePid = $servicePid
            InstalledDisplayName = $installedProduct.DisplayName
            InstalledLocation = $installedProduct.InstallLocation
            Message = "Installer smoke test is skipped because this machine already has a live Clash for Claw installation or service registration."
        } | ConvertTo-Json -Depth 4
        exit 0
    }

    Remove-ItemProperty -Path $runKeyPath -Name "ClashForClaw" -ErrorAction SilentlyContinue

    if (Test-Path $InstallDir) {
        Remove-Item $InstallDir -Recurse -Force
    }

    $installLogPath = Join-Path ([System.IO.Path]::GetTempPath()) ("ClashForClaw-install-" + [System.Guid]::NewGuid().ToString("N") + ".log")
    $installProcess = Start-Process -FilePath $SetupPath -ArgumentList "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/DIR=$InstallDir", "/TASKS=autostart", "/LOG=$installLogPath" -Wait -PassThru
    if ($installProcess.ExitCode -ne 0) {
        throw "INSTALL_EXIT_$($installProcess.ExitCode) LOG=$installLogPath"
    }

    $runValueAfterInstall = (Get-ItemProperty $runKeyPath -ErrorAction SilentlyContinue).ClashForClaw
    $uninstaller = Get-ChildItem $InstallDir -Filter "unins*.exe" -ErrorAction Stop | Select-Object -First 1 -ExpandProperty FullName

    $installState = [pscustomobject]@{
        ExitCode           = $installProcess.ExitCode
        ExeExists          = Test-Path (Join-Path $InstallDir "ClashForClaw.exe")
        ServiceExists      = Test-Path (Join-Path $InstallDir "ClashForClaw.Service.exe")
        MihomoExists       = Test-Path (Join-Path $InstallDir "bin\mihomo.exe")
        RunKeyAfterInstall = $runValueAfterInstall
        Uninstaller        = $uninstaller
        InstallLog         = $installLogPath
    }

    $uninstallLogPath = Join-Path ([System.IO.Path]::GetTempPath()) ("ClashForClaw-uninstall-" + [System.Guid]::NewGuid().ToString("N") + ".log")
    $uninstallProcess = Start-Process -FilePath $uninstaller -ArgumentList "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/LOG=$uninstallLogPath" -Wait -PassThru
    if ($uninstallProcess.ExitCode -ne 0) {
        throw "UNINSTALL_EXIT_$($uninstallProcess.ExitCode) LOG=$uninstallLogPath"
    }

    Start-Sleep -Seconds 2
    $runValueAfterUninstall = (Get-ItemProperty $runKeyPath -ErrorAction SilentlyContinue).ClashForClaw

    $uninstallState = [pscustomobject]@{
        ExitCode             = $uninstallProcess.ExitCode
        InstallDirRemoved    = -not (Test-Path $InstallDir)
        RunKeyAfterUninstall = $runValueAfterUninstall
        UninstallLog         = $uninstallLogPath
    }

    [pscustomobject]@{
        Install   = $installState
        Uninstall = $uninstallState
    } | ConvertTo-Json -Depth 4
}
finally {
    if ([string]::IsNullOrWhiteSpace($previousRunValue)) {
        Remove-ItemProperty -Path $runKeyPath -Name "ClashForClaw" -ErrorAction SilentlyContinue
    }
    else {
        Set-ItemProperty -Path $runKeyPath -Name "ClashForClaw" -Value $previousRunValue
    }
}
