param(
    [Parameter(Mandatory = $true)][string]$ServiceExePath,
    [int]$HttpPort = 13021,
    [int]$WarmupSeconds = 20,
    [switch]$KeepTemp
)

$ErrorActionPreference = "Stop"

function Wait-ForConfig {
    param(
        [string]$BaseUrl,
        [int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -UseBasicParsing "$BaseUrl/config" -TimeoutSec 3
            return ($resp.Content | ConvertFrom-Json)
        }
        catch {
            Start-Sleep -Seconds 1
        }
    }

    throw "Timed out waiting for $BaseUrl/config"
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

if (-not (Test-Path $ServiceExePath -PathType Leaf)) {
    throw "Service executable not found: $ServiceExePath"
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("cfc-shared-config-" + [Guid]::NewGuid().ToString("N"))
$appDataRoot = Join-Path $tempRoot "AppData\Roaming"
$programDataRoot = Join-Path $tempRoot "ProgramData"
$sourceBase = Join-Path $appDataRoot "ClashForClaw"
$targetBase = Join-Path $programDataRoot "ClashForClaw"
$sourceRuntimeSubscriptions = Join-Path $sourceBase "runtime\subscriptions"
$targetRuntimeSubscriptions = Join-Path $targetBase "runtime\subscriptions"

$originalAppData = $env:APPDATA
$originalProgramData = $env:ProgramData
$process = $null

try {
    New-Item -ItemType Directory -Force -Path $sourceRuntimeSubscriptions | Out-Null
    New-Item -ItemType Directory -Force -Path $targetBase | Out-Null

    $sourceSubscriptionPath = Join-Path $sourceRuntimeSubscriptions "provider.yaml"
    Set-Content -Path $sourceSubscriptionPath -Value "proxies: []" -Encoding UTF8

    $sourceConfig = [ordered]@{
        version = 1
        http = @{
            bind = "127.0.0.1"
            port = $HttpPort
        }
        gateway = @{
            url = ""
            token = ""
        }
        proxy = @{
            mode = "local_port"
            local_port = 7890
            subscription_url = ""
            subscriptions = @(
                @{
                    id = "sub-file"
                    name = "migrated"
                    source = "file"
                    file_path = $sourceSubscriptionPath
                }
            )
            active_subscription_id = "sub-file"
            subscription_refresh_hours = 6
            subscription_probe_minutes = 60
            mihomo = @{
                mixed_port = 7890
                http_port = 7891
                socks_port = 7892
                controller_port = 9090
            }
        }
        system_proxy = @{
            enabled = $false
        }
    }

    $sourceConfigPath = Join-Path $sourceBase "config.json"
    New-Item -ItemType Directory -Force -Path $sourceBase | Out-Null
    Save-Json -Path $sourceConfigPath -Value $sourceConfig

    $env:APPDATA = $appDataRoot
    $env:ProgramData = $programDataRoot

    $process = Start-Process -FilePath $ServiceExePath -ArgumentList "--daemon --base-dir `"$targetBase`"" -PassThru -WindowStyle Hidden

    $baseUrl = "http://127.0.0.1:$HttpPort"
    $configResponse = Wait-ForConfig -BaseUrl $baseUrl -TimeoutSeconds $WarmupSeconds

    $targetConfigPath = Join-Path $targetBase "config.json"
    if (-not (Test-Path $targetConfigPath -PathType Leaf)) {
        throw "Target config was not created: $targetConfigPath"
    }

    $targetConfig = Get-Content $targetConfigPath -Raw | ConvertFrom-Json
    $migratedFilePath = $targetConfig.proxy.subscriptions[0].file_path

    if ($targetConfig.proxy.active_subscription_id -ne "sub-file") {
        throw "Active subscription was not migrated."
    }

    if (-not (Test-Path $migratedFilePath -PathType Leaf)) {
        throw "Migrated subscription file missing: $migratedFilePath"
    }

    if ([System.IO.Path]::GetDirectoryName($migratedFilePath) -ne $targetRuntimeSubscriptions) {
        throw "Migrated subscription file was not rewritten into target runtime."
    }

    [pscustomobject]@{
        SourceConfigPath = $sourceConfigPath
        TargetConfigPath = $targetConfigPath
        ActiveSubscriptionId = $targetConfig.proxy.active_subscription_id
        MigratedSubscriptionFile = $migratedFilePath
        ConfigEndpointOk = $configResponse.ok
        ConfigEndpointMode = $configResponse.config.proxy.mode
    } | ConvertTo-Json -Depth 6
}
finally {
    if ($process -and -not $process.HasExited) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }

    $env:APPDATA = $originalAppData
    $env:ProgramData = $originalProgramData

    if (-not $KeepTemp -and (Test-Path $tempRoot)) {
        Remove-Item $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
