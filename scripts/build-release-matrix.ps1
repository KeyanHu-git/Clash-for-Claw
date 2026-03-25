param(
    [string]$Version = "0.1.1",
    [string]$Configuration = "Release",
    [string]$Runtime = "win-x64",
    [string]$Platform = "x64",
    [string]$MihomoPath = "",
    [switch]$SkipBackend,
    [switch]$IncludePortable
)

$ErrorActionPreference = "Stop"

# Compatibility wrapper for older release commands.
# The canonical release entrypoint is scripts/build-installer.ps1.
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
        "-PublishModel", $PublishModel
    )

    if (-not [string]::IsNullOrWhiteSpace($ArtifactSuffix)) {
        $args += @("-ArtifactSuffix", $ArtifactSuffix)
    }

    if ($SkipBackend) {
        $args += "-SkipBackend"
    }
    if ($IncludePortable) {
        $args += "-IncludePortable"
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

Invoke-BuildInstaller -PublishModel "self-contained" -ArtifactSuffix "" -ResolvedMihomoPath $MihomoPath
