param(
    [string]$Version = "0.1.2",
    [string]$Configuration = "Release",
    [string]$Runtime = "win-x64",
    [string]$Platform = "x64",
    [string]$MihomoPath = "",
    [switch]$SkipBackend
)

$ErrorActionPreference = "Stop"

$scriptPath = Join-Path $PSScriptRoot "build-installer.ps1"

function Invoke-BuildInstaller {
    param(
        [string]$PublishModel,
        [string]$ArtifactSuffix,
        [string]$ResolvedMihomoPath,
        [switch]$DisableBundledMihomo
    )

    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", $scriptPath,
        "-Version", $Version,
        "-Configuration", $Configuration,
        "-Runtime", $Runtime,
        "-Platform", $Platform,
        "-PublishModel", $PublishModel,
        "-ArtifactSuffix", $ArtifactSuffix
    )

    if ($SkipBackend) {
        $args += "-SkipBackend"
    }
    if ($DisableBundledMihomo) {
        $args += "-DisableBundledMihomo"
    }
    if (-not [string]::IsNullOrWhiteSpace($ResolvedMihomoPath)) {
        $args += @("-MihomoPath", $ResolvedMihomoPath)
    }

    & powershell @args
    if ($LASTEXITCODE -ne 0) {
        throw "build-installer failed for $ArtifactSuffix"
    }
}

Invoke-BuildInstaller -PublishModel "self-contained" -ArtifactSuffix "full" -ResolvedMihomoPath $MihomoPath
Invoke-BuildInstaller -PublishModel "framework-dependent" -ArtifactSuffix "slim" -ResolvedMihomoPath "" -DisableBundledMihomo
