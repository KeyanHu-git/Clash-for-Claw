param(
    [string]$Version = "0.1.2",
    [string]$Configuration = "Release",
    [string]$Runtime = "win-x64",
    [string]$Platform = "x64",
    [ValidateSet("self-contained", "framework-dependent")][string]$PublishModel = "self-contained",
    [string]$ArtifactSuffix = "",
    [Alias("AdapterPath")][string]$BackendPath = "",
    [Alias("AdapterSourceDir")][string]$BackendSourceDir = "",
    [string]$MihomoPath = "",
    [switch]$DisableBundledMihomo,
    [Alias("SkipAdapter")][switch]$SkipBackend
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.IO.Compression.FileSystem

function Remove-Directory {
    param([string]$Path)

    if (Test-Path $Path) {
        Remove-Item $Path -Recurse -Force
    }
}

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
            $bundledCandidate = Join-Path $RepoRoot ("backend\\ClashForClaw.Service\\internal\\mihomo\\assets\\windows\\" + $bundledArch + "\\mihomo.exe")
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
        "NpuDetect",
        "ru",
        "ja",
        "pt-BR",
        "de",
        "ko",
        "fr",
        "tr",
        "cs",
        "es",
        "it",
        "pl"
    )

    foreach ($name in $directoriesToRemove) {
        Remove-PathIfExists (Join-Path $PublishPath $name)
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$installerRoot = Join-Path $repoRoot "installer"
$artifactsRoot = Join-Path $repoRoot "artifacts\\release"
$stageDir = Join-Path $artifactsRoot ("installer-stage-" + $Version)
$artifactLabel = if ([string]::IsNullOrWhiteSpace($ArtifactSuffix)) { "" } else { "-" + $ArtifactSuffix.Trim() }
$publishDir = Join-Path $artifactsRoot ("publish-" + $Version + $artifactLabel + "-" + $Runtime)
$portableZip = Join-Path $artifactsRoot ("Clash-for-Claw-" + $Version + $artifactLabel + "-portable-" + $Runtime + ".zip")
$setupExe = Join-Path $artifactsRoot ("Clash-for-Claw-" + $Version + $artifactLabel + "-setup.exe")
$sedPath = Join-Path $stageDir "package.sed"
$payloadZip = Join-Path $stageDir "payload.zip"
$defaultBackendSourceDir = Join-Path $repoRoot "backend\\ClashForClaw.Service"
$ddfPath = Join-Path $artifactsRoot ("~Clash-for-Claw-" + $Version + $artifactLabel + "-setup.DDF")

$selfContained = $PublishModel -eq "self-contained"

New-Item -ItemType Directory -Path $artifactsRoot -Force | Out-Null
Remove-Directory $publishDir
Remove-Directory $stageDir
New-Item -ItemType Directory -Path $publishDir -Force | Out-Null
New-Item -ItemType Directory -Path $stageDir -Force | Out-Null

dotnet publish (Join-Path $repoRoot "ClashForClaw.csproj") `
    -c $Configuration `
    -r $Runtime `
    -p:Platform=$Platform `
    -p:SelfContained=$selfContained `
    -p:WindowsAppSDKSelfContained=$selfContained `
    -p:PublishTrimmed=false `
    -p:PublishSingleFile=false `
    -o $publishDir

Copy-Item (Join-Path $repoRoot "LICENSE") (Join-Path $publishDir "LICENSE.txt") -Force

$resolvedBackendPath = $BackendPath
$resolvedMihomoPath = Resolve-MihomoAssetPath -ExplicitPath $MihomoPath -RuntimeId $Runtime -RepoRoot $repoRoot -AllowBundledFallback (-not $DisableBundledMihomo)
if ([string]::IsNullOrWhiteSpace($resolvedBackendPath) -and -not $SkipBackend) {
    $resolvedBackendSourceDir = $BackendSourceDir
    if ([string]::IsNullOrWhiteSpace($resolvedBackendSourceDir)) {
        $resolvedBackendSourceDir = $defaultBackendSourceDir
    }

    $builtBackendPath = Join-Path $stageDir ("ClashForClaw.Service-" + $Runtime + ".exe")
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
if (Test-Path $ddfPath) {
    Remove-Item $ddfPath -Force
}

[System.IO.Compression.ZipFile]::CreateFromDirectory($publishDir, $portableZip, [System.IO.Compression.CompressionLevel]::Optimal, $false)
[System.IO.Compression.ZipFile]::CreateFromDirectory($publishDir, $payloadZip, [System.IO.Compression.CompressionLevel]::Optimal, $false)

$packageInfo = [ordered]@{
    AppName = "Clash for Claw"
    Publisher = "KeyanHu"
    Version = $Version
    Runtime = $Runtime
    PublishModel = $PublishModel
    IncludesBackend = -not [string]::IsNullOrWhiteSpace($resolvedBackendPath)
    IncludesMihomo = -not [string]::IsNullOrWhiteSpace($resolvedMihomoPath)
}
$packageInfo | ConvertTo-Json | Set-Content -Path (Join-Path $stageDir "package-info.json") -Encoding UTF8

Copy-Item (Join-Path $installerRoot "install.cmd") (Join-Path $stageDir "install.cmd") -Force
Copy-Item (Join-Path $installerRoot "install.ps1") (Join-Path $stageDir "install.ps1") -Force
Copy-Item (Join-Path $installerRoot "uninstall.cmd") (Join-Path $stageDir "uninstall.cmd") -Force
Copy-Item (Join-Path $installerRoot "uninstall.ps1") (Join-Path $stageDir "uninstall.ps1") -Force

$sedContent = @"
[Version]
Class=IEXPRESS
SEDVersion=3
[Options]
PackagePurpose=InstallApp
ShowInstallProgramWindow=0
HideExtractAnimation=0
UseLongFileName=1
InsideCompressed=0
CAB_FixedSize=0
CAB_ResvCodeSigning=0
RebootMode=I
InstallPrompt=
DisplayLicense=
FinishMessage=
TargetName=$setupExe
FriendlyName=Clash for Claw Setup
AppLaunched=cmd.exe /d /s /c ""install.cmd""
PostInstallCmd=<None>
AdminQuietInstCmd=cmd.exe /d /s /c ""install.cmd /quiet""
UserQuietInstCmd=cmd.exe /d /s /c ""install.cmd /quiet""
SourceFiles=SourceFiles
SelfDelete=0
FILE0=install.cmd
FILE1=install.ps1
FILE2=uninstall.cmd
FILE3=uninstall.ps1
FILE4=package-info.json
FILE5=payload.zip
[Strings]
FILE0=install.cmd
FILE1=install.ps1
FILE2=uninstall.cmd
FILE3=uninstall.ps1
FILE4=package-info.json
FILE5=payload.zip
[SourceFiles]
SourceFiles0=$stageDir\
[SourceFiles0]
%FILE0%=
%FILE1%=
%FILE2%=
%FILE3%=
%FILE4%=
%FILE5%=
"@
Set-Content -Path $sedPath -Value $sedContent -Encoding ASCII

$iexpress = Join-Path $env:SystemRoot "System32\\iexpress.exe"
& $iexpress /N $sedPath
if ($LASTEXITCODE -ne 0) {
    throw "IExpress failed with exit code $LASTEXITCODE."
}

$deadline = (Get-Date).AddMinutes(2)
while ((Get-Date) -lt $deadline -and -not (Test-Path $setupExe)) {
    Start-Sleep -Seconds 2
}
if (-not (Test-Path $setupExe)) {
    throw "IExpress did not create $setupExe within the expected time window."
}

Write-Output "Portable package: $portableZip"
Write-Output "Installer package: $setupExe"
