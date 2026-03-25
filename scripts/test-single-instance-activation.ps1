param(
    [Parameter(Mandatory = $true)][string]$AppExePath,
    [int]$StartupWaitSeconds = 5,
    [int]$RelaunchWaitSeconds = 4,
    [int]$SettleWaitSeconds = 8,
    [switch]$KeepRunning
)

$ErrorActionPreference = "Stop"

function Wait-ForNoProcess {
    param(
        [string]$ProcessName,
        [int]$TimeoutSeconds = 8
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        if (@(Get-Process $ProcessName -ErrorAction SilentlyContinue).Count -eq 0) {
            return
        }

        Start-Sleep -Milliseconds 200
    }

    throw "Timed out waiting for process '$ProcessName' to exit."
}

if (-not (Test-Path $AppExePath -PathType Leaf)) {
    throw "App executable not found: $AppExePath"
}

Get-Process ClashForClaw -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Wait-ForNoProcess -ProcessName "ClashForClaw"

$first = $null
$second = $null
try {
    $first = Start-Process -FilePath $AppExePath -PassThru
    Start-Sleep -Seconds $StartupWaitSeconds

    $firstProcess = Get-Process -Id $first.Id -ErrorAction SilentlyContinue
    if ($null -eq $firstProcess) {
        throw "Primary instance exited before the startup wait completed."
    }

    $second = Start-Process -FilePath $AppExePath -PassThru
    Start-Sleep -Seconds $RelaunchWaitSeconds

    $all = @()
    $deadline = (Get-Date).AddSeconds($SettleWaitSeconds)
    while ((Get-Date) -lt $deadline) {
        $all = @(Get-Process ClashForClaw -ErrorAction SilentlyContinue)
        if ($all.Count -eq 1 -and $all[0].Responding) {
            break
        }

        Start-Sleep -Milliseconds 250
    }

    $all = @(Get-Process ClashForClaw -ErrorAction SilentlyContinue)
    if ($all.Count -ne 1) {
        throw "Expected exactly one ClashForClaw process after relaunch handoff, got $($all.Count)."
    }

    $survivor = $all[0]
    if (-not $survivor.Responding) {
        throw "The surviving ClashForClaw instance is not responding after relaunch handoff."
    }

    [pscustomobject]@{
        FirstPid = $first.Id
        SecondPid = $second.Id
        SurvivorPid = $survivor.Id
        SurvivorWindowTitle = $survivor.MainWindowTitle
        SurvivorResponding = $survivor.Responding
        LiveCount = $all.Count
        Result = "single_instance_handoff_ok"
    } | ConvertTo-Json -Depth 4
}
finally {
    if (-not $KeepRunning) {
        Get-Process ClashForClaw -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
        Wait-ForNoProcess -ProcessName "ClashForClaw"
    }
}
