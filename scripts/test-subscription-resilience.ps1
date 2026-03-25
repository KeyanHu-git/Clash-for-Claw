param(
    [Parameter(Mandatory = $true)][string]$ServiceExePath,
    [Parameter(Mandatory = $true)][string]$SourceConfigPath,
    [string]$BaseDir = "D:\OpenClawAdapter\artifacts\subscription-test-runtime",
    [int]$HttpPort = 13001,
    [int]$MixedPort = 7895,
    [int]$HttpProxyPort = 7896,
    [int]$SocksPort = 7897,
    [int]$ControllerPort = 9095,
    [int]$WarmupSeconds = 20,
    [int]$SampleCount = 6,
    [int]$SampleIntervalSeconds = 5,
    [int]$RequestTimeoutSeconds = 10
)

$ErrorActionPreference = "Stop"

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

if (-not (Test-Path $ServiceExePath -PathType Leaf)) {
    throw "Service executable not found: $ServiceExePath"
}

if (-not (Test-Path $SourceConfigPath -PathType Leaf)) {
    throw "Source config not found: $SourceConfigPath"
}

if (Test-Path $BaseDir) {
    Remove-Item $BaseDir -Recurse -Force
}

New-Item -ItemType Directory -Path $BaseDir -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $BaseDir "logs") -Force | Out-Null

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
    InitialActiveSubscription = $cfg.proxy.active_subscription_id
    Samples = @()
    Recovery = $null
}

$process = $null
try {
    $process = Start-Process -FilePath $ServiceExePath -ArgumentList "--daemon --base-dir `"$BaseDir`"" -PassThru -WindowStyle Hidden
    $null = Wait-ForStatus -BaseUrl $baseUrl -TimeoutSeconds $WarmupSeconds

    Start-Sleep -Seconds 2
    $cfgAfterStart = Get-Json $configPath
    $result.StartedActiveSubscription = $cfgAfterStart.proxy.active_subscription_id

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
        Start-Sleep -Seconds $SampleIntervalSeconds
    }

    $mihomoPid = Get-PortOwner -Ports @($MixedPort, $ControllerPort)
    if (-not $mihomoPid) {
        throw "Failed to locate isolated mihomo process on ports $MixedPort/$ControllerPort"
    }

    Stop-Process -Id $mihomoPid -Force
    $deadline = (Get-Date).AddSeconds(25)
    $recovered = $false
    $recoveryStatus = $null
    while ((Get-Date) -lt $deadline) {
        try {
            $recoveryStatus = Invoke-WebRequest -UseBasicParsing "$baseUrl/status" -TimeoutSec $RequestTimeoutSeconds | Select-Object -ExpandProperty Content | ConvertFrom-Json
            if ($recoveryStatus.proxy.mihomo_active -and $recoveryStatus.proxy.effective_mode -eq "subscription_url") {
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

    $sampleFailures = @($result.Samples | Where-Object { -not $_.Ok -or -not $_.GatewayOk -or -not $_.InternetOk -or $_.Error })
    $result.SampleFailureCount = $sampleFailures.Count

    if ($sampleFailures.Count -gt 0) {
        throw "Observed $($sampleFailures.Count) unhealthy or timed-out subscription samples during soak."
    }

    if (-not $recovered) {
        throw "Isolated subscription runtime did not recover after mihomo kill."
    }

    $result | ConvertTo-Json -Depth 8
}
finally {
    if ($process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }
}
