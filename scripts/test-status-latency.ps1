param(
    [Parameter(Mandatory = $true)][string]$ServiceExePath,
    [Parameter(Mandatory = $true)][string]$SourceConfigPath,
    [string]$BaseDir = "D:\OpenClawAdapter\artifacts\status-latency-runtime",
    [int]$HttpPort = 13031,
    [int]$SampleCount = 8,
    [int]$SampleIntervalSeconds = 2,
    [int]$RequestTimeoutSeconds = 10,
    [int]$WarmupSeconds = 20
)

$ErrorActionPreference = "Stop"

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

function Wait-ForHealth {
    param(
        [string]$BaseUrl,
        [int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            Invoke-RestMethod -Uri "$BaseUrl/health" -Method Get -TimeoutSec 2 | Out-Null
            return
        }
        catch {
            Start-Sleep -Milliseconds 250
        }
    }

    throw "Timed out waiting for $BaseUrl/health"
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

$BaseDir = Initialize-BaseDir -RequestedPath $BaseDir -Ports @($HttpPort)

$configPath = Join-Path $BaseDir "config.json"
Copy-Item $SourceConfigPath $configPath -Force

$cfg = Get-Json $configPath
$cfg.http.port = $HttpPort
$cfg.proxy.mode = "local_port"
$cfg.proxy.subscription_url = ""
$cfg.proxy.subscriptions = @()
$cfg.proxy.active_subscription_id = ""
$cfg.system_proxy.enabled = $false
Save-Json $configPath $cfg

$baseUrl = "http://127.0.0.1:$HttpPort"
$process = $null

try {
    $process = Start-Process -FilePath $ServiceExePath -ArgumentList "--daemon --base-dir `"$BaseDir`"" -PassThru -WindowStyle Hidden
    Wait-ForHealth -BaseUrl $baseUrl -TimeoutSeconds $WarmupSeconds

    $samples = @()
    for ($i = 0; $i -lt $SampleCount; $i++) {
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        try {
            $status = Invoke-RestMethod -Uri "$baseUrl/status" -Method Get -TimeoutSec $RequestTimeoutSeconds
            $sw.Stop()
            $samples += [pscustomobject]@{
                Index = $i
                ElapsedMs = $sw.ElapsedMilliseconds
                Ok = $status.ok
                GatewayOk = $status.probe.gateway_ok
                InternetOk = $status.probe.internet_ok
                EffectiveMode = $status.proxy.effective_mode
            }
        }
        catch {
            $sw.Stop()
            $samples += [pscustomobject]@{
                Index = $i
                ElapsedMs = $sw.ElapsedMilliseconds
                Error = $_.Exception.Message
            }
        }

        Start-Sleep -Seconds $SampleIntervalSeconds
    }

    [pscustomobject]@{
        BaseDir = $BaseDir
        BaseUrl = $baseUrl
        SampleCount = $SampleCount
        Samples = $samples
    } | ConvertTo-Json -Depth 6
}
finally {
    if ($process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }
}
