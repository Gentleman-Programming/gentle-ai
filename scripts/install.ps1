#Requires -Version 5.1
<#
.SYNOPSIS
    Gentle-AI — Install Script for Windows
    Ecosystem, Frameworks, Workflows for AI coding agents.

.DESCRIPTION
    Downloads and installs the gentle-ai binary for Windows.
    Supports installation via Go or pre-built binary from GitHub Releases.

.EXAMPLE
    # Run directly:
    irm https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.ps1 | iex

    # Or download and run:
    Invoke-WebRequest -Uri https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.ps1 -OutFile install.ps1
    .\install.ps1

    # Force a specific method:
    .\install.ps1 -Method binary
    .\install.ps1 -Method go

    # Skip checksum verification (not recommended):
    .\install.ps1 -Method binary -Insecure
#>

[CmdletBinding()]
param(
    [ValidateSet("auto", "go", "binary")]
    [string]$Method = "auto",

    [string]$InstallDir = "",

    [switch]$Insecure
)

$ErrorActionPreference = "Stop"

# Ensure UTF-8 output so Unicode characters render correctly on all terminals.
# chcp 65001 sets the console code page; OutputEncoding makes .NET match it.
# Wrapped in try/catch: under ErrorActionPreference=Stop the .NET setter can
# throw IOException ("handle is invalid") in non-console hosts (ISE, remoting,
# some CI pipelines) and abort the whole install. Safe to swallow.
$null = & chcp 65001 2>$null
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch {}

$GITHUB_OWNER = "Gentleman-Programming"
$GITHUB_REPO = "gentle-ai"
$BINARY_NAME = "gentle-ai"

# ============================================================================
# Logging helpers
# ============================================================================

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

# ============================================================================
# Banner
# ============================================================================

function Show-Banner {
    Write-Host ""
    Write-Host "   ____            _   _              _    ___ " -ForegroundColor Cyan
    Write-Host "  / ___| ___ _ __ | |_| | ___        / \  |_ _|" -ForegroundColor Cyan
    Write-Host " | |  _ / _ \ '_ \| __| |/ _ \_____ / _ \  | | " -ForegroundColor Cyan
    Write-Host " | |_| |  __/ | | | |_| |  __/_____/ ___ \ | | " -ForegroundColor Cyan
    Write-Host "  \____|\___|_| |_|\__|_|\___|    /_/   \_\___|" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  Gentle-AI — Ecosystem, Frameworks, Workflows" -ForegroundColor DarkGray
    Write-Host ""
}

# ============================================================================
# Platform detection
# ============================================================================

function Get-Platform {
    Write-Step "Detecting platform"

    $arch = if ([Environment]::Is64BitOperatingSystem) {
        if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
    } else {
        Stop-WithError "32-bit Windows is not supported."
    }

    Write-Success "Platform: Windows ($arch)"
    return $arch
}

# ============================================================================
# Prerequisites
# ============================================================================

function Test-Prerequisites {
    Write-Step "Checking prerequisites"

    $missing = @()
    if (-not (Get-Command "curl" -ErrorAction SilentlyContinue)) { $missing += "curl" }
    if (-not (Get-Command "git" -ErrorAction SilentlyContinue))  { $missing += "git" }

    if ($missing.Count -gt 0) {
        Stop-WithError "Missing required tools: $($missing -join ', '). Please install them and try again."
    }

    Write-Success "curl and git are available"
}

# ============================================================================
# Install method detection
# ============================================================================

function Get-InstallMethod {
    param([string]$Forced)

    if ($Forced -ne "auto") {
        Write-Info "Using forced method: $Forced"
        return $Forced
    }

    Write-Step "Detecting best install method"

    # Prefer binary download over go install: GitHub Releases are instant
    # while the Go module proxy can lag behind new tags for up to 30 minutes,
    # causing `go install ...@latest` to install a stale version.
    Write-Info "Will download pre-built binary from GitHub Releases"
    return "binary"
}

# ============================================================================
# Install via go install
# ============================================================================

function Install-ViaGo {
    Write-Step "Installing via go install"

    $goPackage = "github.com/$($GITHUB_OWNER.ToLower())/$GITHUB_REPO/cmd/$BINARY_NAME@latest"
    Write-Info "Running: go install $goPackage"

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

    Write-Success "Installed $BINARY_NAME via go install"
}

# ============================================================================
# Install via binary download
# ============================================================================

function Get-LatestVersion {
    Write-Info "Fetching latest release from GitHub..."

    $url = "https://api.github.com/repos/$GITHUB_OWNER/$GITHUB_REPO/releases/latest"

    try {
        $response = Invoke-RestMethod -Uri $url -Headers @{ "User-Agent" = "gentle-ai-installer" }
    } catch {
        Stop-WithError "Failed to fetch latest release. Rate limited? Try again later or use -Method go"
    }

    $version = $response.tag_name
    if (-not $version) {
        Stop-WithError "Could not determine latest version from GitHub API response"
    }

    Write-Success "Latest version: $version"
    return $version
}

function Install-ViaBinary {
    param([string]$Arch)

    Write-Step "Installing pre-built binary"

    $version = Get-LatestVersion
    $versionNumber = $version.TrimStart("v")

    $archiveName = "${BINARY_NAME}_${versionNumber}_windows_${Arch}.zip"
    $downloadUrl = "https://github.com/$GITHUB_OWNER/$GITHUB_REPO/releases/download/$version/$archiveName"
    $checksumsUrl = "https://github.com/$GITHUB_OWNER/$GITHUB_REPO/releases/download/$version/checksums.txt"

    $tmpDir = Join-Path $env:TEMP "gentle-ai-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        # Download archive
        Write-Info "Downloading $archiveName..."
        $archivePath = Join-Path $tmpDir $archiveName
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing

        $fileSize = (Get-Item $archivePath).Length
        if ($fileSize -lt 1000) {
            Stop-WithError "Downloaded file is suspiciously small ($fileSize bytes). Archive may not exist for this platform."
        }
        Write-Success "Downloaded $archiveName ($fileSize bytes)"

        # Verify checksum
        Write-Info "Verifying checksum..."
        try {
            $checksumsPath = Join-Path $tmpDir "checksums.txt"
            Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing

            $checksums = Get-Content $checksumsPath
            $expectedLine = $checksums | Where-Object { $_ -match $archiveName }
            if ($expectedLine) {
                $expectedChecksum = ($expectedLine -split "\s+")[0]
                $actualChecksum = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()

                if ($actualChecksum -ne $expectedChecksum) {
                    Stop-WithError "Checksum mismatch!`n  Expected: $expectedChecksum`n  Got:      $actualChecksum"
                }
                Write-Success "Checksum verified"
            } else {
                if ($Insecure) {
                    Write-Warn "Archive '$archiveName' not found in checksums.txt - checksum verification skipped (-Insecure)"
                } else {
                    Stop-WithError "Archive '$archiveName' not found in checksums.txt. Refusing to install unverified binary.`nUse -Insecure to skip (not recommended)."
                }
            }
        } catch {
            if ($Insecure) {
                Write-Warn "Could not download checksums.txt - checksum verification skipped (-Insecure)"
            } else {
                Stop-WithError "Could not download checksums.txt from: $checksumsUrl`nRefusing to install without integrity verification.`nUse -Insecure to skip (not recommended)."
            }
        }

        # Extract binary
        Write-Info "Extracting $BINARY_NAME..."
        Expand-Archive -Path $archivePath -DestinationPath $tmpDir -Force

        $binaryPath = Join-Path $tmpDir "$BINARY_NAME.exe"
        if (-not (Test-Path $binaryPath)) {
            Stop-WithError "Binary '$BINARY_NAME.exe' not found in archive"
        }

        # Determine install directory
        $installDir = $InstallDir
        if (-not $installDir) {
            $installDir = Join-Path $env:LOCALAPPDATA "gentle-ai\bin"
        }

        if (-not (Test-Path $installDir)) {
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        }

        # Install binary
        $destPath = Join-Path $installDir "$BINARY_NAME.exe"
        Write-Info "Installing to $destPath..."
        Copy-Item -Path $binaryPath -Destination $destPath -Force

        Write-Success "Installed $BINARY_NAME to $destPath"

        # Persist install dir to the User PATH if not already present.
        # NOTE: [Environment]::GetEnvironmentVariable reads the registry value
        # after Windows expands any embedded %VAR% references, so REG_EXPAND_SZ
        # variables (e.g. %USERPROFILE%) are flattened to their current values.
        # This is a Windows API limitation when using the managed .NET accessor;
        # a fully lossless round-trip would require the Win32 Registry class with
        # GetValue(..., DoNotExpandEnvironmentNames). We accept the trade-off here
        # because user PATH entries that rely on unexpanded refs are uncommon and
        # we only ever append — we never rewrite the whole value.
        $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")

        # Split on ';' and compare entries case-insensitively so wildcard chars
        # in the path do not break the match and sibling directories with a
        # shared prefix do not trigger a false-positive.
        $pathEntries = if ($userPath) { $userPath -split ';' | Where-Object { $_ -ne '' } } else { @() }
        $alreadyPresent = $pathEntries | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }
        if (-not $alreadyPresent) {
            $newUserPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
            [Environment]::SetEnvironmentVariable("PATH", $newUserPath, "User")
            Write-Success "Added $installDir to your PATH (takes effect in new shells)"
        }

        # Also update the current session's PATH so Test-Installation can find the binary.
        $sessionEntries = $env:PATH -split ';' | Where-Object { $_ -ne '' }
        $sessionPresent = $sessionEntries | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }
        if (-not $sessionPresent) {
            $env:PATH = "$env:PATH;$installDir"
        }
    } finally {
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ============================================================================
# Verify installation
# ============================================================================

function Test-Installation {
    Write-Step "Verifying installation"

    # Build the list of candidate absolute paths to check, most-specific first.
    # We intentionally probe by absolute path rather than searching the current
    # session PATH so the check is deterministic and immune to stale PATH state.
    $gopath = $null
    if (Get-Command "go" -ErrorAction SilentlyContinue) {
        $gopath = & go env GOPATH 2>$null
    }
    $locations = @(
        (Join-Path $env:LOCALAPPDATA "gentle-ai\bin\$BINARY_NAME.exe")
    )
    if ($gopath) {
        $locations += (Join-Path $gopath "bin\$BINARY_NAME.exe")
    }

    foreach ($loc in $locations) {
        if (-not ($loc -and (Test-Path $loc))) { continue }

        # Use --version (fast, no system detection, no self-update) and suppress
        # self-update explicitly via GENTLE_AI_NO_SELF_UPDATE so the check is a
        # pure version read even if the binary is older.
        $env:GENTLE_AI_NO_SELF_UPDATE = "1"
        $versionOutput = & $loc --version 2>&1
        Remove-Item Env:GENTLE_AI_NO_SELF_UPDATE -ErrorAction SilentlyContinue

        Write-Success "$BINARY_NAME installed at $loc`: $versionOutput"

        # Inform the user if the binary is not yet reachable by name.
        $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
        $binaryDir = [System.IO.Path]::GetDirectoryName($loc)
        if ($userPath -notlike "*$binaryDir*") {
            Write-Warn "Binary location is not in your PATH. Open a new shell or add it manually."
        }
        return
    }

    Write-Warn "Could not verify installation. You may need to restart your terminal."
}

# ============================================================================
# Next steps
# ============================================================================

function Show-NextSteps {
    Write-Host ""
    Write-Host "Installation complete!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Next steps:" -ForegroundColor White
    Write-Host "  1. Run '$BINARY_NAME' to start the TUI installer" -ForegroundColor Cyan
    Write-Host "  2. Select your AI agent(s) and tools to configure" -ForegroundColor Cyan
    Write-Host "  3. Follow the interactive prompts" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "For help: $BINARY_NAME --help" -ForegroundColor DarkGray
    Write-Host "Docs:     https://github.com/$GITHUB_OWNER/$GITHUB_REPO" -ForegroundColor DarkGray
    Write-Host ""
}

# ============================================================================
# Main
# ============================================================================

function Main {
    Show-Banner

    $arch = Get-Platform
    Test-Prerequisites

    $installMethod = Get-InstallMethod -Forced $Method

    switch ($installMethod) {
        "go"     { Install-ViaGo }
        "binary" { Install-ViaBinary -Arch $arch }
    }

    Test-Installation
    Show-NextSteps
}

Main
