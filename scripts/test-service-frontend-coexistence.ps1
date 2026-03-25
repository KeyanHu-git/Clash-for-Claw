param(
    [Parameter(Mandatory = $true)][string]$AppExePath,
    [string]$ServiceName = "ClashForClaw",
    [int]$ControlPort = 13000,
    [int]$ColdLaunchWaitSeconds = 5,
    [int]$RelaunchWaitSeconds = 3
)

$ErrorActionPreference = "Stop"

function Get-ServiceSnapshot {
    param([string]$Name)

    $raw = sc.exe queryex $Name 2>$null
    if (-not $raw) {
        return [pscustomobject]@{
            Name = $Name
            Registered = $false
            Running = $false
            ProcessId = 0
        }
    }

    $joined = $raw -join "`n"
    $running = $joined -match 'STATE\s+: 4\s+RUNNING'
    $servicePid = 0
    if ($joined -match 'PID\s+: (\d+)') {
        $servicePid = [int]$Matches[1]
    }

    [pscustomobject]@{
        Name = $Name
        Registered = $true
        Running = $running
        ProcessId = $servicePid
    }
}

function Get-FrontendSnapshot {
    return ,@(Get-Process ClashForClaw -ErrorAction SilentlyContinue |
        Select-Object Id, ProcessName, MainWindowTitle, Responding, StartTime)
}

function Get-PortOwners {
    param([int]$Port)

    return ,@(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -ExpandProperty OwningProcess -Unique)
}

function Wait-ForFrontendCount {
    param(
        [int]$ExpectedCount,
        [int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $front = Get-FrontendSnapshot
        if ($front.Count -eq $ExpectedCount) {
            return ,$front
        }
        Start-Sleep -Milliseconds 300
    }

    return ,(Get-FrontendSnapshot)
}

if (-not (Test-Path $AppExePath -PathType Leaf)) {
    throw "App executable not found: $AppExePath"
}

$service = Get-ServiceSnapshot -Name $ServiceName
if (-not $service.Registered) {
    throw "Windows service '$ServiceName' is not installed."
}
if (-not $service.Running -or $service.ProcessId -le 0) {
    throw "Windows service '$ServiceName' is not running."
}

Get-Process ClashForClaw -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2

$firstLauncher = Start-Process -FilePath $AppExePath -PassThru
$frontAfterColdLaunch = Wait-ForFrontendCount -ExpectedCount 1 -TimeoutSeconds $ColdLaunchWaitSeconds
$ownersAfterColdLaunch = Get-PortOwners -Port $ControlPort

if ($frontAfterColdLaunch.Count -ne 1) {
    throw "Expected exactly one frontend process after cold launch, got $($frontAfterColdLaunch.Count)."
}

if ($ownersAfterColdLaunch.Count -ne 1 -or $ownersAfterColdLaunch[0] -ne $service.ProcessId) {
    throw "Expected control port $ControlPort to remain owned by service PID $($service.ProcessId), got: $($ownersAfterColdLaunch -join ',')."
}

$steadyFrontendPid = $frontAfterColdLaunch[0].Id
$secondLauncher = Start-Process -FilePath $AppExePath -PassThru
$frontAfterRelaunch = Wait-ForFrontendCount -ExpectedCount 1 -TimeoutSeconds $RelaunchWaitSeconds
$ownersAfterRelaunch = Get-PortOwners -Port $ControlPort

if ($frontAfterRelaunch.Count -ne 1) {
    throw "Expected exactly one frontend process after relaunch, got $($frontAfterRelaunch.Count)."
}

if ($frontAfterRelaunch[0].Id -ne $steadyFrontendPid) {
    throw "Expected second launch to reuse the existing frontend PID $steadyFrontendPid, got $($frontAfterRelaunch[0].Id)."
}

if ($ownersAfterRelaunch.Count -ne 1 -or $ownersAfterRelaunch[0] -ne $service.ProcessId) {
    throw "Expected control port $ControlPort to remain owned by service PID $($service.ProcessId) after relaunch, got: $($ownersAfterRelaunch -join ',')."
}

[pscustomobject]@{
    Service = $service
    ControlPort = [pscustomobject]@{
        Port = $ControlPort
        OwnerAfterColdLaunch = $ownersAfterColdLaunch
        OwnerAfterRelaunch = $ownersAfterRelaunch
    }
    Launch = [pscustomobject]@{
        FirstLauncherPid = $firstLauncher.Id
        SecondLauncherPid = $secondLauncher.Id
        FrontendPid = $steadyFrontendPid
        ReusedFrontendPid = $frontAfterRelaunch[0].Id
    }
} | ConvertTo-Json -Depth 6
