param(
    [string]$Version = "0.1.1",
    [string]$Configuration = "Release",
    [string]$Runtime = "win-x64",
    [string]$Platform = "x64",
    [string]$AdapterPath = "",
    [switch]$SkipAdapter
)

$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.IO.Compression.FileSystem

function Remove-Directory {
    param([string]$Path)

    if (Test-Path $Path) {
        Remove-Item $Path -Recurse -Force
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$installerRoot = Join-Path $repoRoot "installer"
$artifactsRoot = Join-Path $repoRoot "artifacts\\release"
$publishDir = Join-Path $artifactsRoot ("publish-" + $Runtime)
$stageDir = Join-Path $artifactsRoot ("installer-stage-" + $Version)
$portableZip = Join-Path $artifactsRoot ("Clash-for-Claw-" + $Version + "-portable-" + $Runtime + ".zip")
$setupExe = Join-Path $artifactsRoot ("Clash-for-Claw-" + $Version + "-setup.exe")
$sedPath = Join-Path $stageDir "package.sed"
$payloadZip = Join-Path $stageDir "payload.zip"
$defaultAdapterPath = "D:\\openclaw-adapter\\OpenClaw-Adapter.exe"
$ddfPath = Join-Path $artifactsRoot ("~Clash-for-Claw-" + $Version + "-setup.DDF")

New-Item -ItemType Directory -Path $artifactsRoot -Force | Out-Null
Remove-Directory $publishDir
Remove-Directory $stageDir
New-Item -ItemType Directory -Path $publishDir -Force | Out-Null
New-Item -ItemType Directory -Path $stageDir -Force | Out-Null

dotnet publish (Join-Path $repoRoot "OpenClawAdapter.csproj") `
    -c $Configuration `
    -r $Runtime `
    -p:Platform=$Platform `
    -p:SelfContained=true `
    -p:WindowsAppSDKSelfContained=true `
    -p:PublishTrimmed=false `
    -p:PublishSingleFile=false `
    -o $publishDir

Copy-Item (Join-Path $repoRoot "LICENSE") (Join-Path $publishDir "LICENSE.txt") -Force

$resolvedAdapterPath = $AdapterPath
if ([string]::IsNullOrWhiteSpace($resolvedAdapterPath) -and -not $SkipAdapter -and (Test-Path $defaultAdapterPath)) {
    $resolvedAdapterPath = $defaultAdapterPath
}

if (-not [string]::IsNullOrWhiteSpace($resolvedAdapterPath)) {
    if (-not (Test-Path $resolvedAdapterPath)) {
        throw "Adapter not found at $resolvedAdapterPath."
    }
    Copy-Item $resolvedAdapterPath (Join-Path $publishDir "OpenClaw-Adapter.exe") -Force
}

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
    IncludesAdapter = -not [string]::IsNullOrWhiteSpace($resolvedAdapterPath)
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
