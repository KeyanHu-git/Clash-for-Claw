param(
    [Parameter(Mandatory = $true)][string]$ServiceExePath,
    [Parameter(Mandatory = $true)][string]$SourceConfigPath,
    [string]$BaseDir = "D:\OpenClawAdapter\artifacts\subscription-test-runtime",
    [string]$MihomoPath = "",
    [int]$HttpPort = 13001,
    [int]$MixedPort = 7895,
    [int]$HttpProxyPort = 7896,
    [int]$SocksPort = 7897,
    [int]$ControllerPort = 9095,
    [int]$WarmupSeconds = 20,
    [int]$SampleCount = 6,
    [int]$SampleIntervalSeconds = 5,
    [int]$RequestTimeoutSeconds = 10,
    [switch]$SkipRecovery
)

$ErrorActionPreference = "Stop"

function Resolve-MihomoSeedPath {
    param(
        [string]$ExplicitPath,
        [string]$ServiceExecutablePath
    )

    $candidates = @()
    if (-not [string]::IsNullOrWhiteSpace($ExplicitPath)) {
        $candidates += $ExplicitPath
    }

    $candidates += @(
        (Join-Path $env:ProgramData "ClashForClaw\bin\mihomo.exe"),
        (Join-Path (Split-Path -Parent $ServiceExecutablePath) "bin\mihomo.exe"),
        (Join-Path (Split-Path -Parent $ServiceExecutablePath) "mihomo.exe")
    )

    foreach ($candidate in $candidates) {
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            continue
        }

        if (Test-Path $candidate -PathType Leaf) {
            $item = Get-Item $candidate
            if ($item.Length -gt 1MB) {
                return (Resolve-Path $candidate).Path
            }
        }
    }

    return ""
}

function Wait-ForStatus {
    param(
        [string]$BaseUrl,
        [int]$TimeoutSeconds = 30
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -UseBasicParsing "$BaseUrl/status" -TimeoutSec $RequestTimeoutSeconds
            return ($resp.Content | ConvertFrom-Json)
        }
        catch {
            Start-Sleep -Seconds 1
        }
    }

    throw "Timed out waiting for $BaseUrl/status"
}

function Wait-ForHealthySubscription {
    param(
        [string]$BaseUrl,
        [int]$TimeoutSeconds = 45
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $lastStatus = $null
    while ((Get-Date) -lt $deadline) {
        try {
            $lastStatus = Invoke-WebRequest -UseBasicParsing "$BaseUrl/status" -TimeoutSec $RequestTimeoutSeconds |
                Select-Object -ExpandProperty Content |
                ConvertFrom-Json

            if ($lastStatus.ok `
                -and $lastStatus.probe.gateway_ok `
                -and $lastStatus.probe.internet_ok `
                -and $lastStatus.proxy.mihomo_active `
                -and $lastStatus.proxy.effective_mode -eq "subscription_url") {
                return $lastStatus
            }
        }
        catch {
        }

        Start-Sleep -Seconds 2
    }

    if ($lastStatus) {
        throw ("Timed out waiting for healthy subscription mode. Last status: " + ($lastStatus | ConvertTo-Json -Depth 6 -Compress))
    }

    throw "Timed out waiting for healthy subscription mode."
}

function Get-Json {
    param([string]$Path)
    return (Get-Content $Path -Raw -Encoding UTF8 | ConvertFrom-Json)
}

function Save-Json {
    param(
        [string]$Path,
        [object]$Value
    )

    $json = $Value | ConvertTo-Json -Depth 12
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $json, $utf8NoBom)
}

function Get-PortOwner {
    param([int[]]$Ports)

    foreach ($port in $Ports) {
        $owner = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue |
            Select-Object -First 1 -ExpandProperty OwningProcess
        if ($owner) {
            return $owner
        }
    }

    return $null
}

function Stop-PortOwners {
    param([int[]]$Ports)

    $ownerIds = @()
    foreach ($port in $Ports) {
        $ownerIds += @(Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue |
            Select-Object -ExpandProperty OwningProcess -Unique)
    }

    foreach ($ownerId in ($ownerIds | Where-Object { $_ -and $_ -gt 0 } | Sort-Object -Unique)) {
        try {
            Stop-Process -Id $ownerId -Force -ErrorAction Stop
        }
        catch {
        }
    }
}

function Initialize-BaseDir {
    param(
        [string]$RequestedPath,
        [int[]]$Ports
    )

    if (-not (Test-Path $RequestedPath)) {
        New-Item -ItemType Directory -Path $RequestedPath -Force | Out-Null
        return $RequestedPath
    }

    Stop-PortOwners -Ports $Ports

    for ($attempt = 0; $attempt -lt 6; $attempt++) {
        try {
            Remove-Item $RequestedPath -Recurse -Force -ErrorAction Stop
            New-Item -ItemType Directory -Path $RequestedPath -Force | Out-Null
            return $RequestedPath
        }
        catch {
            if ($attempt -eq 0) {
                Stop-PortOwners -Ports $Ports
            }

            if ($attempt -eq 5) {
                $fallbackPath = "$RequestedPath-" + [Guid]::NewGuid().ToString("N")
                New-Item -ItemType Directory -Path $fallbackPath -Force | Out-Null
                return $fallbackPath
            }

            Start-Sleep -Milliseconds 400
        }
    }
}

if (-not (Test-Path $ServiceExePath -PathType Leaf)) {
    throw "Service executable not found: $ServiceExePath"
}

if (-not (Test-Path $SourceConfigPath -PathType Leaf)) {
    throw "Source config not found: $SourceConfigPath"
}

$BaseDir = Initialize-BaseDir -RequestedPath $BaseDir -Ports @($HttpPort, $MixedPort, $HttpProxyPort, $SocksPort, $ControllerPort)
New-Item -ItemType Directory -Path (Join-Path $BaseDir "logs") -Force | Out-Null

$seedMihomoPath = Resolve-MihomoSeedPath -ExplicitPath $MihomoPath -ServiceExecutablePath $ServiceExePath
if (-not [string]::IsNullOrWhiteSpace($seedMihomoPath)) {
    $binDir = Join-Path $BaseDir "bin"
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
    Copy-Item $seedMihomoPath (Join-Path $binDir "mihomo.exe") -Force
}

$configPath = Join-Path $BaseDir "config.json"
Copy-Item $SourceConfigPath $configPath -Force

$cfg = Get-Json $configPath
$cfg.http.port = $HttpPort
$cfg.proxy.local_port = $MixedPort
$cfg.proxy.mihomo.mixed_port = $MixedPort
$cfg.proxy.mihomo.http_port = $HttpProxyPort
$cfg.proxy.mihomo.socks_port = $SocksPort
$cfg.proxy.mihomo.controller_port = $ControllerPort
$cfg.system_proxy.enabled = $false
Save-Json $configPath $cfg

$baseUrl = "http://127.0.0.1:$HttpPort"
$result = [ordered]@{
    BaseDir = $BaseDir
    BaseUrl = $baseUrl
    SeededMihomoPath = $seedMihomoPath
    InitialActiveSubscription = $cfg.proxy.active_subscription_id
    Samples = @()
    Recovery = $null
}
$resultPath = Join-Path $BaseDir "result.json"

$process = $null
try {
    $process = Start-Process -FilePath $ServiceExePath -ArgumentList "--daemon --base-dir `"$BaseDir`"" -PassThru -WindowStyle Hidden
    $null = Wait-ForStatus -BaseUrl $baseUrl -TimeoutSeconds $WarmupSeconds
    $healthyStatus = Wait-ForHealthySubscription -BaseUrl $baseUrl -TimeoutSeconds ($WarmupSeconds + 25)

    Start-Sleep -Seconds 2
    $cfgAfterStart = Get-Json $configPath
    $result.StartedActiveSubscription = $cfgAfterStart.proxy.active_subscription_id
    $result.InitialHealth = [pscustomobject]@{
        Ok = $healthyStatus.ok
        GatewayOk = $healthyStatus.probe.gateway_ok
        InternetOk = $healthyStatus.probe.internet_ok
        MihomoActive = $healthyStatus.proxy.mihomo_active
        EffectiveMode = $healthyStatus.proxy.effective_mode
    }

    for ($i = 0; $i -lt $SampleCount; $i++) {
        $sample = [ordered]@{
            Index = $i
            Error = ""
            Ok = $false
            GatewayOk = $false
            InternetOk = $false
            GatewayLatencyMs = 0
            InternetLatencyMs = 0
            Mode = ""
            EffectiveMode = ""
            MihomoActive = $false
            Fallback = $false
        }
        try {
            $status = Invoke-WebRequest -UseBasicParsing "$baseUrl/status" -TimeoutSec $RequestTimeoutSeconds | Select-Object -ExpandProperty Content | ConvertFrom-Json
            $sample.Ok = $status.ok
            $sample.GatewayOk = $status.probe.gateway_ok
            $sample.InternetOk = $status.probe.internet_ok
            $sample.GatewayLatencyMs = $status.probe.gateway_latency_ms
            $sample.InternetLatencyMs = $status.probe.internet_latency_ms
            $sample.Mode = $status.proxy.mode
            $sample.EffectiveMode = $status.proxy.effective_mode
            $sample.MihomoActive = $status.proxy.mihomo_active
            $sample.Fallback = $status.proxy.fallback
        }
        catch {
            $sample.Error = $_.Exception.Message
        }
        $result.Samples += [pscustomobject]$sample
        Save-Json -Path $resultPath -Value $result
        Start-Sleep -Seconds $SampleIntervalSeconds
    }

    $sampleFailures = @($result.Samples | Where-Object { -not $_.Ok -or -not $_.GatewayOk -or -not $_.InternetOk -or $_.Error })
    $result.SampleFailureCount = $sampleFailures.Count
    Save-Json -Path $resultPath -Value $result

    if ($sampleFailures.Count -gt 0) {
        throw "Observed $($sampleFailures.Count) unhealthy or timed-out subscription samples during soak."
    }

    if ($SkipRecovery) {
        $result.Recovery = [pscustomobject]@{
            Skipped = $true
        }
        Save-Json -Path $resultPath -Value $result
        $result | ConvertTo-Json -Depth 8
        return
    }

    $mihomoPid = Get-PortOwner -Ports @($MixedPort, $ControllerPort)
    if (-not $mihomoPid) {
        throw "Failed to locate isolated mihomo process on ports $MixedPort/$ControllerPort"
    }

    Stop-Process -Id $mihomoPid -Force -ErrorAction SilentlyContinue
    $deadline = (Get-Date).AddSeconds(25)
    $recovered = $false
    $recoveryStatus = $null
    while ((Get-Date) -lt $deadline) {
        try {
            $recoveryStatus = Invoke-WebRequest -UseBasicParsing "$baseUrl/status" -TimeoutSec $RequestTimeoutSeconds | Select-Object -ExpandProperty Content | ConvertFrom-Json
            if ($recoveryStatus.ok `
                -and $recoveryStatus.probe.gateway_ok `
                -and $recoveryStatus.probe.internet_ok `
                -and $recoveryStatus.proxy.mihomo_active `
                -and $recoveryStatus.proxy.effective_mode -eq "subscription_url") {
                $recovered = $true
                break
            }
        }
        catch {
        }
        Start-Sleep -Seconds 2
    }

    $result.Recovery = [pscustomobject]@{
        KilledMihomoPid = $mihomoPid
        Recovered = $recovered
        FinalOk = if ($recoveryStatus) { $recoveryStatus.ok } else { $false }
        FinalGatewayOk = if ($recoveryStatus) { $recoveryStatus.probe.gateway_ok } else { $false }
        FinalInternetOk = if ($recoveryStatus) { $recoveryStatus.probe.internet_ok } else { $false }
        FinalMihomoActive = if ($recoveryStatus) { $recoveryStatus.proxy.mihomo_active } else { $false }
        FinalEffectiveMode = if ($recoveryStatus) { $recoveryStatus.proxy.effective_mode } else { "" }
    }
    Save-Json -Path $resultPath -Value $result

    if (-not $recovered) {
        throw "Isolated subscription runtime did not recover after mihomo kill."
    }

    Save-Json -Path $resultPath -Value $result
    $result | ConvertTo-Json -Depth 8
}
finally {
    if ($result.Samples.Count -gt 0 -or $result.Recovery) {
        Save-Json -Path $resultPath -Value $result
    }

    if ($process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }
}
