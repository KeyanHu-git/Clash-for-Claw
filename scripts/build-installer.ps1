param(
    [string]$Version = "0.1.1",
    [string]$Configuration = "Release",
    [string]$Runtime = "win-x64",
    [string]$Platform = "x64",
    [ValidateSet("self-contained", "framework-dependent")][string]$PublishModel = "self-contained",
    [string]$ArtifactSuffix = "",
    [Alias("AdapterPath")][string]$BackendPath = "",
    [Alias("AdapterSourceDir")][string]$BackendSourceDir = "",
    [string]$MihomoPath = "",
    [string]$InnoCompilerPath = "",
    [switch]$IncludePortable,
    [switch]$DisableBundledMihomo,
    [Alias("SkipAdapter")][switch]$SkipBackend
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.IO.Compression.FileSystem

function Remove-PathIfExists {
    param([string]$Path)

    if (Test-Path $Path) {
        Remove-Item $Path -Recurse -Force
    }
}

function Get-GoArchForRuntime {
    param([string]$RuntimeId)

    switch ($RuntimeId) {
        "win-x64" { return "amd64" }
        "win-arm64" { return "arm64" }
        "win-x86" { return "386" }
        default { throw "Unsupported runtime '$RuntimeId' for backend build." }
    }
}

function Get-MihomoArchForRuntime {
    param([string]$RuntimeId)

    switch ($RuntimeId) {
        "win-x64" { return "amd64" }
        "win-arm64" { return "arm64" }
        default { return "" }
    }
}

function Get-InnoSetupArchConfig {
    param([string]$RuntimeId)

    switch ($RuntimeId) {
        "win-x64" {
            return @{
                Allowed = "x64compatible"
                InstallIn64BitMode = "x64compatible"
            }
        }
        "win-arm64" {
            return @{
                Allowed = "arm64"
                InstallIn64BitMode = "arm64"
            }
        }
        "win-x86" {
            return @{
                Allowed = ""
                InstallIn64BitMode = ""
            }
        }
        default { throw "Unsupported runtime '$RuntimeId' for Inno Setup build." }
    }
}

function Resolve-InnoSetupCompiler {
    param([string]$ExplicitPath)

    $candidates = @()
    if (-not [string]::IsNullOrWhiteSpace($ExplicitPath)) {
        $candidates += $ExplicitPath
    }

    $candidates += @(
        (Join-Path $env:LOCALAPPDATA "Programs\Inno Setup 6\ISCC.exe"),
        "C:\Program Files (x86)\Inno Setup 6\ISCC.exe",
        "C:\Program Files\Inno Setup 6\ISCC.exe"
    )

    foreach ($candidate in $candidates) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path $candidate -PathType Leaf)) {
            return (Resolve-Path $candidate).Path
        }
    }

    throw "Inno Setup compiler not found. Install JRSoftware.InnoSetup or pass -InnoCompilerPath."
}

function Test-UsableBinary {
    param([string]$Path)

    if (-not (Test-Path $Path -PathType Leaf)) {
        return $false
    }

    return (Get-Item $Path).Length -gt 1MB
}

function Resolve-MihomoAssetPath {
    param(
        [string]$ExplicitPath,
        [string]$RuntimeId,
        [string]$RepoRoot,
        [bool]$AllowBundledFallback = $true
    )

    $candidates = @()
    if (-not [string]::IsNullOrWhiteSpace($ExplicitPath)) {
        $candidates += $ExplicitPath
    }

    foreach ($envName in @("CLASH_FOR_CLAW_MIHOMO_PATH", "OPENCLAW_ADAPTER_MIHOMO_PATH")) {
        $value = [Environment]::GetEnvironmentVariable($envName)
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            $candidates += $value
        }
    }

    foreach ($candidate in $candidates) {
        if (Test-UsableBinary $candidate) {
            return (Resolve-Path $candidate).Path
        }
    }

    if ($AllowBundledFallback) {
        $bundledArch = Get-MihomoArchForRuntime -RuntimeId $RuntimeId
        if (-not [string]::IsNullOrWhiteSpace($bundledArch)) {
            $bundledCandidate = Join-Path $RepoRoot ("backend\ClashForClaw.Service\internal\mihomo\assets\windows\" + $bundledArch + "\mihomo.exe")
            if (Test-UsableBinary $bundledCandidate) {
                return (Resolve-Path $bundledCandidate).Path
            }
        }
    }

    return ""
}

function Build-OptimizedBackend {
    param(
        [string]$SourceDir,
        [string]$RuntimeId,
        [string]$OutputPath
    )

    if (-not (Test-Path (Join-Path $SourceDir "go.mod"))) {
        throw "Backend source not found at $SourceDir."
    }

    $previousGoos = $env:GOOS
    $previousGoarch = $env:GOARCH
    $previousCgo = $env:CGO_ENABLED
    $pushedLocation = $false

    try {
        $env:GOOS = "windows"
        $env:GOARCH = Get-GoArchForRuntime $RuntimeId
        $env:CGO_ENABLED = "0"

        Push-Location $SourceDir
        $pushedLocation = $true
        & go build -trimpath -ldflags="-s -w" -o $OutputPath ./cmd
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE."
        }
    }
    finally {
        if ($pushedLocation) {
            Pop-Location
        }
        $env:GOOS = $previousGoos
        $env:GOARCH = $previousGoarch
        $env:CGO_ENABLED = $previousCgo
    }
}

function Prune-PublishOutput {
    param([string]$PublishPath)

    $filesToRemove = @(
        "Microsoft.Web.WebView2.Core.dll",
        "Microsoft.Web.WebView2.Core.Projection.dll",
        "WebView2Loader.dll",
        "System.Windows.Forms.Design.dll",
        "System.Windows.Forms.Design.Editors.dll",
        "Microsoft.VisualBasic.Forms.dll",
        "Microsoft.Windows.AI.ContentModerationInternal.winmd",
        "Microsoft.Windows.AI.ContentSafety.dll",
        "Microsoft.Windows.AI.ContentSafety.Projection.dll",
        "Microsoft.Windows.AI.ContentSafety.winmd",
        "Microsoft.Windows.AI.Foundation.Projection.dll",
        "Microsoft.Windows.AI.Foundation.winmd",
        "Microsoft.Windows.AI.FoundationInternal.winmd",
        "Microsoft.Windows.AI.GenerativeInternal.winmd",
        "Microsoft.Windows.AI.Imaging.dll",
        "Microsoft.Windows.AI.Imaging.Projection.dll",
        "Microsoft.Windows.AI.Imaging.winmd",
        "Microsoft.Windows.AI.MachineLearning.dll",
        "Microsoft.Windows.AI.MachineLearning.Projection.dll",
        "Microsoft.Windows.AI.MachineLearning.winmd",
        "Microsoft.Windows.AI.Projection.dll",
        "Microsoft.Windows.AI.Text.dll",
        "Microsoft.Windows.AI.Text.Projection.dll",
        "Microsoft.Windows.AI.Text.winmd",
        "Microsoft.Windows.AI.winmd",
        "Microsoft.Windows.Workloads.dll",
        "Microsoft.Windows.Workloads.Resources.dll",
        "Microsoft.Windows.Workloads.Resources_ec.dll",
        "Microsoft.Windows.Workloads.winmd",
        "Microsoft.ML.OnnxRuntime.dll",
        "onnxruntime.dll",
        "onnxruntime_providers_shared.dll",
        "DirectML.dll",
        "workloads.365.json",
        "workloads.j32.json",
        "workloads.json",
        "workloads.lnl.json",
        "workloads.qnn.json",
        "workloads.stx.json",
        "Microsoft.DiaSymReader.Native.amd64.dll",
        "mscordaccore_amd64_amd64_10.0.426.12010.dll",
        "mscordaccore.dll",
        "mscordbi.dll",
        "clrgcexp.dll",
        "clrgc.dll",
        "clretwrc.dll",
        "Microsoft.UI.Designer.dll",
        "ClashForClaw.pdb",
        "OpenClawAdapter.pdb",
        "createdump.exe",
        "System.Design.dll",
        "System.Drawing.Design.dll",
        "Microsoft.WindowsAppRuntime.Insights.Resource.dll",
        "RestartAgent.exe"
    )

    foreach ($name in $filesToRemove) {
        Remove-PathIfExists (Join-Path $PublishPath $name)
    }

    $directoriesToRemove = @(
        "NpuDetect"
    )

    foreach ($name in $directoriesToRemove) {
        Remove-PathIfExists (Join-Path $PublishPath $name)
    }

    $languageWhitelist = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($name in @("en-US", "en-us", "zh-CN", "zh-cn")) {
        [void]$languageWhitelist.Add($name)
    }

    $languageDirectoryPattern = '^[A-Za-z]{2,3}(?:-[A-Za-z]{2,8}){1,2}$'
    foreach ($directory in Get-ChildItem $PublishPath -Directory) {
        if ($directory.Name -match $languageDirectoryPattern -and -not $languageWhitelist.Contains($directory.Name)) {
            Remove-PathIfExists $directory.FullName
        }
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$artifactsRoot = Join-Path $repoRoot "artifacts\release"
$artifactLabel = if ([string]::IsNullOrWhiteSpace($ArtifactSuffix)) { "" } else { "-" + $ArtifactSuffix.Trim() }
$publishDir = Join-Path $artifactsRoot ("publish-" + $Version + $artifactLabel + "-" + $Runtime)
$portableZip = Join-Path $artifactsRoot ("Clash-for-Claw-" + $Version + $artifactLabel + "-portable-" + $Runtime + ".zip")
$setupBaseName = "Clash-for-Claw-" + $Version + $artifactLabel + "-setup"
$setupExe = Join-Path $artifactsRoot ($setupBaseName + ".exe")
$defaultBackendSourceDir = Join-Path $repoRoot "backend\ClashForClaw.Service"
$selfContained = $PublishModel -eq "self-contained"

New-Item -ItemType Directory -Path $artifactsRoot -Force | Out-Null
Remove-PathIfExists $publishDir
New-Item -ItemType Directory -Path $publishDir -Force | Out-Null

dotnet publish (Join-Path $repoRoot "ClashForClaw.csproj") `
    -c $Configuration `
    -r $Runtime `
    -p:Platform=$Platform `
    -p:SelfContained=$selfContained `
    -p:WindowsAppSDKSelfContained=$selfContained `
    -p:PublishTrimmed=false `
    -p:PublishSingleFile=false `
    -p:PublishReadyToRun=false `
    -o $publishDir

Copy-Item (Join-Path $repoRoot "LICENSE") (Join-Path $publishDir "LICENSE.txt") -Force

$resolvedBackendPath = $BackendPath
$resolvedMihomoPath = Resolve-MihomoAssetPath -ExplicitPath $MihomoPath -RuntimeId $Runtime -RepoRoot $repoRoot -AllowBundledFallback (-not $DisableBundledMihomo)
if ([string]::IsNullOrWhiteSpace($resolvedBackendPath) -and -not $SkipBackend) {
    $resolvedBackendSourceDir = $BackendSourceDir
    if ([string]::IsNullOrWhiteSpace($resolvedBackendSourceDir)) {
        $resolvedBackendSourceDir = $defaultBackendSourceDir
    }

    $builtBackendPath = Join-Path $artifactsRoot ("ClashForClaw.Service-" + $Runtime + ".exe")
    if (Test-Path (Join-Path $resolvedBackendSourceDir "go.mod")) {
        Build-OptimizedBackend -SourceDir $resolvedBackendSourceDir -RuntimeId $Runtime -OutputPath $builtBackendPath
        $resolvedBackendPath = $builtBackendPath
    }
}

if (-not [string]::IsNullOrWhiteSpace($resolvedBackendPath)) {
    if (-not (Test-Path $resolvedBackendPath)) {
        throw "Backend not found at $resolvedBackendPath."
    }
    Copy-Item $resolvedBackendPath (Join-Path $publishDir "ClashForClaw.Service.exe") -Force
}

if (-not [string]::IsNullOrWhiteSpace($resolvedMihomoPath)) {
    $publishBinDir = Join-Path $publishDir "bin"
    New-Item -ItemType Directory -Path $publishBinDir -Force | Out-Null
    Copy-Item $resolvedMihomoPath (Join-Path $publishBinDir "mihomo.exe") -Force
}

Prune-PublishOutput -PublishPath $publishDir

if (Test-Path $portableZip) {
    Remove-Item $portableZip -Force
}
if (Test-Path $setupExe) {
    Remove-Item $setupExe -Force
}

if ($IncludePortable) {
    [System.IO.Compression.ZipFile]::CreateFromDirectory($publishDir, $portableZip, [System.IO.Compression.CompressionLevel]::Optimal, $false)
}

$compilerPath = Resolve-InnoSetupCompiler -ExplicitPath $InnoCompilerPath
$archConfig = Get-InnoSetupArchConfig -RuntimeId $Runtime
$issPath = Join-Path $repoRoot "installer\ClashForClaw.iss"
$licensePath = Join-Path $repoRoot "LICENSE"

$isccArgs = @(
    "/DMyAppVersion=$Version",
    "/DMyAppPublisher=KeyanHu",
    "/DMyAppName=Clash for Claw",
    "/DMyPublishDir=$publishDir",
    "/DMyOutputDir=$artifactsRoot",
    "/DMyOutputBaseFilename=$setupBaseName",
    "/DMyLicenseFile=$licensePath"
)

if (-not [string]::IsNullOrWhiteSpace($archConfig.Allowed)) {
    $isccArgs += "/DMyArchitecturesAllowed=$($archConfig.Allowed)"
}

if (-not [string]::IsNullOrWhiteSpace($archConfig.InstallIn64BitMode)) {
    $isccArgs += "/DMyArchitecturesInstallIn64BitMode=$($archConfig.InstallIn64BitMode)"
}

$isccArgs += $issPath

& $compilerPath @isccArgs
if ($LASTEXITCODE -ne 0) {
    throw "Inno Setup compilation failed with exit code $LASTEXITCODE."
}

if (-not (Test-Path $setupExe)) {
    throw "Installer output not found at $setupExe."
}

Write-Output "Installer package: $setupExe"
if ($IncludePortable -and (Test-Path $portableZip)) {
    Write-Output "Portable package: $portableZip"
}
