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
$STABLE_SOURCE_COMMAND = "go install github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai@latest"

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
    $version = if ($Channel -eq "beta") { "main" } else { "latest" }
    # /v2 is part of the module path, not decoration: Go refuses to resolve a
    # module whose tags are v2.x unless the import path carries the major
    # version suffix.
    $goPackage = "github.com/$($GITHUB_OWNER.ToLower())/$GITHUB_REPO/v2/cmd/$BINARY_NAME@$version"
    Write-Info "Running: go install $goPackage"

    if ($Channel -eq "beta") {
        Add-GoEnvPattern -Name "GONOSUMDB" -Pattern "github.com/gentleman-programming/gentle-ai/v2"
        Add-GoEnvPattern -Name "GOPRIVATE" -Pattern "github.com/gentleman-programming/gentle-ai/v2"
        Add-GoEnvPattern -Name "GONOPROXY" -Pattern "github.com/gentleman-programming/gentle-ai/v2"
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
