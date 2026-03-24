param(
    [string]$Port = "13051",
    [string]$LocalPort = "17890"
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendRoot = Join-Path $repoRoot "backend\ClashForClaw.Service"
$smokeDir = Join-Path $repoRoot "artifacts\self-shutdown-smoke"
$runtimeDir = Join-Path $repoRoot "artifacts\self-shutdown-smoke-runtime"
$exePath = Join-Path $smokeDir "ClashForClaw.Service.exe"
$configPath = Join-Path $runtimeDir "config.json"

if (Test-Path $smokeDir) {
    Remove-Item $smokeDir -Recurse -Force
}
if (Test-Path $runtimeDir) {
    Remove-Item $runtimeDir -Recurse -Force
}

New-Item -ItemType Directory -Path $smokeDir | Out-Null
New-Item -ItemType Directory -Path $runtimeDir | Out-Null

Push-Location $backendRoot
try {
    & go build -trimpath -ldflags="-s -w" -o $exePath ./cmd
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$portNumber = [int]$Port
$localPortNumber = [int]$LocalPort
$config = @{
    version = 1
    http = @{
        bind = "127.0.0.1"
        port = $portNumber
    }
    gateway = @{
        url = "http://127.0.0.1:18789/chat?session=smoke"
        token = ""
    }
    proxy = @{
        mode = "local_port"
        local_port = $localPortNumber
        subscription_url = ""
        subscriptions = @()
        active_subscription_id = ""
        subscription_refresh_hours = 6
        subscription_probe_minutes = 60
        mihomo = @{
            mixed_port = $localPortNumber
            http_port = $localPortNumber + 1
            socks_port = $localPortNumber + 2
            controller_port = $localPortNumber + 1000
        }
    }
    system_proxy = @{
        enabled = $false
    }
} | ConvertTo-Json -Depth 8

$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($configPath, $config, $utf8NoBom)

$process = Start-Process -FilePath $exePath -ArgumentList "--base-dir", $runtimeDir -PassThru -WindowStyle Hidden

try {
    $ready = $false
    for ($i = 0; $i -lt 40; $i++) {
        Start-Sleep -Milliseconds 250
        try {
            $null = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$Port/health" -TimeoutSec 2
            $ready = $true
            break
        }
        catch {
        }
    }

    if (-not $ready) {
        throw "smoke backend did not become ready"
    }

    $nonceResponse = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/nonce" -Method Get -TimeoutSec 3
    $nonce = $nonceResponse.nonce
    if ([string]::IsNullOrWhiteSpace($nonce)) {
        throw "nonce missing"
    }

    $response = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/daemon/shutdown" -Method Post -Headers @{ "X-Adapter-Nonce" = $nonce } -ContentType "application/json" -Body "{}" -TimeoutSec 3
    $exited = $process.WaitForExit(10000)
    if (-not $exited) {
        throw "daemon did not exit after shutdown call"
    }

    Write-Output "SMOKE_OK pid=$($process.Id) exit=$($process.ExitCode) response_ok=$($response.ok)"
}
finally {
    if (-not $process.HasExited) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }
}
