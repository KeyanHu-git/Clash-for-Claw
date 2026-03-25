param(
    [Parameter(Mandatory = $true)][string]$AppExePath,
    [int]$StartupWaitSeconds = 6,
    [int]$RelaunchWaitSeconds = 4,
    [int]$CloseWaitSeconds = 12
)

$ErrorActionPreference = "Stop"

function Get-StateSnapshot {
    param([int]$Port = 13000)

    [pscustomobject]@{
        Front = @(Get-Process ClashForClaw -ErrorAction SilentlyContinue).Count
        Back = @(Get-Process ClashForClaw.Service -ErrorAction SilentlyContinue).Count
        PortOwners = @(
            Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
                Select-Object -ExpandProperty OwningProcess
        )
    }
}

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

function Assert-State {
    param(
        [string]$Label,
        [pscustomobject]$State,
        [bool]$ExpectFront,
        [bool]$ExpectPortOwner
    )

    if ($ExpectFront -and $State.Front -ne 1) {
        throw "$Label expected 1 frontend process, got $($State.Front)."
    }

    if (-not $ExpectFront -and $State.Front -ne 0) {
        throw "$Label expected 0 frontend processes, got $($State.Front)."
    }

    if ($ExpectPortOwner -and $State.PortOwners.Count -ne 1) {
        throw "$Label expected exactly 1 listening owner on port 13000, got $($State.PortOwners.Count)."
    }

    if (-not $ExpectPortOwner -and $State.PortOwners.Count -ne 0) {
        throw "$Label expected port 13000 to be released, got owners: $($State.PortOwners -join ',')."
    }
}

$settingsPath = Join-Path $env:APPDATA "ClashForClaw\settings.json"
$backupPath = Join-Path $env:TEMP ("clashforclaw-settings-backup-" + [Guid]::NewGuid().ToString("N") + ".json")

if (-not (Test-Path $AppExePath -PathType Leaf)) {
    throw "App executable not found: $AppExePath"
}

if (Test-Path $settingsPath) {
    Copy-Item $settingsPath $backupPath -Force
}

try {
    $servicePid = Get-RunningServicePid
    if ($servicePid) {
        [pscustomobject]@{
            Skipped = $true
            Reason = "windows_service_mode_active"
            ServicePid = $servicePid
            Message = "Desktop daemon lifecycle smoke test is skipped because the Windows service already owns the shared control port 13000."
        } | ConvertTo-Json -Depth 4
        exit 0
    }

    $settings = if (Test-Path $settingsPath) {
        Get-Content $settingsPath -Raw | ConvertFrom-Json
    }
    else {
        [pscustomobject]@{}
    }

    $settings | Add-Member -NotePropertyName CloseToTrayEnabled -NotePropertyValue $false -Force
    $settings | ConvertTo-Json -Depth 8 | Set-Content $settingsPath -Encoding UTF8

    Get-Process ClashForClaw,ClashForClaw.Service -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2

    Start-Process -FilePath $AppExePath | Out-Null
    Start-Sleep -Seconds $StartupWaitSeconds
    $before = Get-StateSnapshot
    Assert-State -Label "before" -State $before -ExpectFront $true -ExpectPortOwner $true

    Start-Process -FilePath $AppExePath | Out-Null
    Start-Sleep -Seconds $RelaunchWaitSeconds
    $afterRelaunch = Get-StateSnapshot
    Assert-State -Label "after_relaunch" -State $afterRelaunch -ExpectFront $true -ExpectPortOwner $true

    if (($afterRelaunch.PortOwners | Select-Object -Unique).Count -ne 1) {
        throw "after_relaunch expected a single stable port owner, got: $($afterRelaunch.PortOwners -join ',')."
    }

    $main = Get-Process ClashForClaw -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $main) {
        throw "unable to locate frontend process for graceful close."
    }

    $null = $main.CloseMainWindow()
    Start-Sleep -Seconds $CloseWaitSeconds
    $afterClose = Get-StateSnapshot
    Assert-State -Label "after_close" -State $afterClose -ExpectFront $false -ExpectPortOwner $false

    [pscustomobject]@{
        Before = $before
        AfterRelaunch = $afterRelaunch
        AfterClose = $afterClose
    } | ConvertTo-Json -Depth 6
}
finally {
    if (Test-Path $backupPath) {
        Copy-Item $backupPath $settingsPath -Force
        Remove-Item $backupPath -Force
    }

    Get-Process ClashForClaw,ClashForClaw.Service -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}
