#Requires -Version 5.1
<#
.SYNOPSIS
    Gentle-AI source installer for Windows.

.DESCRIPTION
    Installs Gentle AI from source with Go. Official Windows binary distribution
    and Scoop are temporarily unavailable until public-trust Authenticode signing
    is enforced. Accepted channels: stable (default), beta, nightly.

.EXAMPLE
    .\install.ps1

.EXAMPLE
    .\install.ps1 -Method go -Channel beta

.EXAMPLE
    # The legacy binary method fails closed with source-install guidance.
    .\install.ps1 -Method binary
#>

$ErrorActionPreference = "Stop"

# Ensure UTF-8 output so Unicode characters render correctly on all terminals.
$null = & chcp 65001 2>$null
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}

$GITHUB_OWNER = "Gentleman-Programming"
$GITHUB_REPO = "gentle-ai"
$BINARY_NAME = "gentle-ai"
$WINDOWS_DISTRIBUTION_HOLD = "Windows binary distribution and Scoop are temporarily unavailable until publicly trusted Authenticode signing is enforced."
$STABLE_SOURCE_COMMAND = ".\install.ps1 -Method go -Channel stable"

function Write-Info    { param([string]$Message) Write-Host "[info]    $Message" -ForegroundColor Blue }
function Write-Success { param([string]$Message) Write-Host "[ok]      $Message" -ForegroundColor Green }
function Write-Warn    { param([string]$Message) Write-Host "[warn]    $Message" -ForegroundColor Yellow }
function Write-Err     { param([string]$Message) Write-Host "[error]   $Message" -ForegroundColor Red }
function Write-Step    { param([string]$Message) Write-Host "`n==> $Message" -ForegroundColor Cyan }

function Stop-WithError {
    param([string]$Message)
    Write-Err $Message
    exit 1
}

function Show-Banner {
    Write-Host ""
    Write-Host "   ____            _   _              _    ___ " -ForegroundColor Cyan
    Write-Host "  / ___| ___ _ __ | |_| | ___        / \  |_ _|" -ForegroundColor Cyan
    Write-Host " | |  _ / _ \ '_ \| __| |/ _ \_____ / _ \  | | " -ForegroundColor Cyan
    Write-Host " | |_| |  __/ | | | |_| |  __/_____/ ___ \ | | " -ForegroundColor Cyan
    Write-Host "  \____|\___|_| |_|\__|_|\___|    /_/   \_\___|" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  Gentle-AI - Ecosystem, Frameworks, Workflows" -ForegroundColor DarkGray
    Write-Host ""
}

function Get-Platform {
    Write-Step "Detecting platform"
    if (-not [Environment]::Is64BitOperatingSystem) {
        Stop-WithError "32-bit Windows is not supported."
    }
    $arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
    Write-Success "Platform: Windows ($arch)"
}

function Test-Prerequisites {
    Write-Step "Checking prerequisites"
    if (-not (Get-Command "go" -ErrorAction SilentlyContinue)) {
        Stop-WithError "$WINDOWS_DISTRIBUTION_HOLD Install Go 1.25.10 or newer, then run: $STABLE_SOURCE_COMMAND"
    }
    Write-Success "Go is available"
}

function Get-InstallMethod {
    param([string]$Forced, [string]$Channel)

    if ($Forced -eq "binary") {
        Stop-WithError "$WINDOWS_DISTRIBUTION_HOLD No unsigned binary will be downloaded or executed. Install from source with Go 1.25.10 or newer: $STABLE_SOURCE_COMMAND"
    }
    if ($Channel -eq "beta") {
        Write-Info "Using beta channel - installing $BINARY_NAME from main via go install"
    } else {
        Write-Info "Using source installation via go install"
    }
    return "go"
}

function Install-ViaGo {
    param([string]$Channel = "stable")

    Write-Step "Installing via go install"
    $api = "https://api.github.com/repos/$GITHUB_OWNER/$GITHUB_REPO"
    if ($Channel -eq "beta") {
        $version = (Invoke-RestMethod -Uri "$api/commits/main").sha
        if ($version -isnot [string] -or $version -cnotmatch '\A[0-9a-fA-F]{40}\z') {
            Stop-WithError "Could not resolve main to a full commit SHA."
        }
    } else {
        $version = (Invoke-RestMethod -Uri "$api/releases/latest").tag_name
        if ($version -isnot [string] -or $version -cnotmatch '\Av[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?\z') {
            Stop-WithError "Could not resolve the latest release tag."
        }
    }
    $module = Get-GoModule -Version $version
    $goPackage = "$module/cmd/$BINARY_NAME@$version"
    Write-Info "Running: go install $goPackage"

    if ($Channel -eq "beta") {
        Add-GoEnvPattern -Name "GONOSUMDB" -Pattern $module
        Add-GoEnvPattern -Name "GOPRIVATE" -Pattern $module
        Add-GoEnvPattern -Name "GONOPROXY" -Pattern $module
    }

    & go install $goPackage
    if ($LASTEXITCODE -ne 0) {
        Stop-WithError "Failed to install via go install. Make sure Go is properly configured."
    }

    $gobin = & go env GOBIN 2>$null
    if (-not $gobin) {
        $gopath = & go env GOPATH 2>$null
        $gobin = Join-Path $gopath "bin"
    }
    if ($env:PATH -notlike "*$gobin*") {
        Write-Warn "$gobin is not in your PATH"
        Write-Warn "Add it to your PATH environment variable."
    }
    Write-Success "Installed $BINARY_NAME from source via go install"
}

function Get-GoModule {
    param([string]$Version)

    $saved = @{}
    foreach ($name in @("GOTOOLCHAIN", "GOWORK", "GOFLAGS")) {
        $saved[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
    }
    $tempFile = [System.IO.Path]::GetTempFileName()
    try {
        Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/$GITHUB_OWNER/$GITHUB_REPO/$Version/go.mod" -OutFile $tempFile
        # Parse metadata without switching toolchains or loading a workspace.
        $env:GOTOOLCHAIN = "local"
        $env:GOWORK = "off"
        $env:GOFLAGS = ""
        $metadata = & go mod edit -json $tempFile
        if ($LASTEXITCODE -ne 0) {
            Stop-WithError "Could not parse the selected version's go.mod."
        }
        $module = ($metadata | ConvertFrom-Json).Module.Path
        if ($module -isnot [string] -or $module -cnotmatch '\Agithub\.com/gentleman-programming/gentle-ai(?:/v(?:[2-9]|[1-9][0-9]+))?\z') {
            Stop-WithError "The selected version declares an unexpected Go module."
        }
    } finally {
        foreach ($name in $saved.Keys) {
            if ($null -eq $saved[$name]) {
                Remove-Item -LiteralPath "Env:$name" -ErrorAction SilentlyContinue
            } else {
                [Environment]::SetEnvironmentVariable($name, $saved[$name], "Process")
            }
        }
        Remove-Item -LiteralPath $tempFile -Force
    }
    return $module
}

function Add-GoEnvPattern {
    param(
        [string]$Name,
        [string]$Pattern
    )

    $current = [Environment]::GetEnvironmentVariable($Name, "Process")
    if (-not $current) {
        Set-Item -Path "Env:$Name" -Value $Pattern
        return
    }

    $patterns = $current.Split(",", [System.StringSplitOptions]::RemoveEmptyEntries).Trim()
    if ($patterns -contains $Pattern) { return }
    Set-Item -Path "Env:$Name" -Value ("{0},{1}" -f $Pattern, $current)
}

function Test-Installation {
    Write-Step "Verifying installation"
    $gobin = & go env GOBIN 2>$null
    if (-not $gobin) {
        $gopath = & go env GOPATH 2>$null
        $gobin = Join-Path $gopath "bin"
    }
    $binaryPath = Join-Path $gobin "$BINARY_NAME.exe"
    if (-not (Test-Path $binaryPath)) {
        Write-Warn "Could not verify $binaryPath. Check go env GOBIN and go env GOPATH."
        return
    }

    $env:GENTLE_AI_NO_SELF_UPDATE = "1"
    $versionOutput = & $binaryPath --version 2>&1
    Remove-Item Env:GENTLE_AI_NO_SELF_UPDATE -ErrorAction SilentlyContinue
    Write-Success "$BINARY_NAME installed at $binaryPath`: $versionOutput"
}

function Show-NextSteps {
    param([string]$Channel = "stable")

    Write-Host ""
    Write-Host "Installation complete!" -ForegroundColor Green
    Write-Host ""
    if ($Channel -eq "beta") {
        Write-Host ('  Run ''$env:GENTLE_AI_CHANNEL = "beta"; {0} install'' to keep using the beta channel' -f $BINARY_NAME) -ForegroundColor Cyan
    } else {
        Write-Host "  Run '$BINARY_NAME' to start the TUI installer" -ForegroundColor Cyan
    }
    Write-Host "Docs: https://github.com/$GITHUB_OWNER/$GITHUB_REPO" -ForegroundColor DarkGray
    Write-Host ""
}

function Main {
    [CmdletBinding()]
    param(
        [ValidateSet("auto", "go", "binary")]
        [string]$Method = "auto",

        [ValidateSet("stable", "beta", "nightly")]
        [string]$Channel = $(if ($env:GENTLE_AI_CHANNEL) { $env:GENTLE_AI_CHANNEL } else { "stable" }),

        [string]$InstallDir = "",

        [switch]$Insecure
    )

    Show-Banner
    if ($Insecure) {
        Stop-WithError "$WINDOWS_DISTRIBUTION_HOLD The legacy -Insecure switch cannot bypass this policy. $STABLE_SOURCE_COMMAND"
    }
    if ($InstallDir) {
        Stop-WithError "-InstallDir is unavailable for source installation. Configure go env GOBIN instead."
    }
    if ($Channel -eq "nightly") { $Channel = "beta" }

    $installMethod = Get-InstallMethod -Forced $Method -Channel $Channel
    Get-Platform
    Test-Prerequisites
    if ($installMethod -eq "go") {
        Install-ViaGo -Channel $Channel
    }
    Test-Installation
    Show-NextSteps -Channel $Channel
}

Main @args
