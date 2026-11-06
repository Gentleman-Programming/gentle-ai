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
$STABLE_SOURCE_COMMAND = "go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@latest"

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
    if ($Channel -eq "beta") {
        $version = "main"
        # /v4 is part of the module path, not decoration: Go refuses to resolve a
        # module whose tags are v4.x unless the import path carries the major
        # version suffix.
        $goPackage = "github.com/$($GITHUB_OWNER.ToLower())/$GITHUB_REPO/v4/cmd/$BINARY_NAME@$version"
    } else {
        # Stable pins the latest release tag and derives the module path from
        # its major. Never use @latest: before a v4.x tag exists Go resolves the
        # /v4 path to an unreleased pseudo-version at the default-branch tip.
        $releaseUrl = "https://api.github.com/repos/$GITHUB_OWNER/$GITHUB_REPO/releases/latest"
        $tag = ""
        try {
            [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
            $tag = [string](Invoke-RestMethod -Uri $releaseUrl -UseBasicParsing).tag_name
        } catch {}
        if ($tag -notmatch '^v(\d+)\.\d+\.\d+$') {
            Stop-WithError ("Could not resolve a valid latest release tag from {0}; refusing to install an unreleased version." -f $releaseUrl)
        }
        # Go module rule: majors 0 and 1 are unsuffixed, 2+ append /vN.
        $major = [int]$Matches[1]
        $module = "github.com/{0}/{1}" -f $GITHUB_OWNER.ToLower(), $GITHUB_REPO
        if ($major -ge 2) { $module = "{0}/v{1}" -f $module, $major }
        $goPackage = "{0}/cmd/{1}@{2}" -f $module, $BINARY_NAME, $tag
    }
    Write-Info "Running: go install $goPackage"

    if ($Channel -eq "beta") {
        Add-GoEnvPattern -Name "GONOSUMDB" -Pattern "github.com/gentleman-programming/gentle-ai/v4"
        Add-GoEnvPattern -Name "GOPRIVATE" -Pattern "github.com/gentleman-programming/gentle-ai/v4"
        Add-GoEnvPattern -Name "GONOPROXY" -Pattern "github.com/gentleman-programming/gentle-ai/v4"
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
    Write-Success "Installed $BINARY_NAME from source via go install"

    Add-InstallDirToPath -Directory $gobin
}

# Puts the install dir on PATH for this session and future terminals, and says
# where. Prints nothing when the session already resolves the directory.
function Add-InstallDirToPath {
    param([string]$Directory)

    if ((Add-UserPathEntry -Directory $Directory) -ne "added") { return }
    Write-Host ""
    Write-Host ("Current terminal: {0} was added to PATH" -f $Directory)
    Write-Host "Future terminals: open a new terminal"
    Write-Success ("PATH was added to the user environment (HKCU\Environment Path): {0}" -f $Directory)
}

# Reads a user environment variable as stored, keeping %VAR% references
# unexpanded so writing it back does not flatten them.
function Get-UserEnv {
    param([string]$Key)

    $envKey = (Get-Item -Path "HKCU:").OpenSubKey("Environment")
    if ($null -eq $envKey) { return $null }
    try {
        return $envKey.GetValue($Key, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    } finally {
        $envKey.Close()
    }
}

# Writes a user environment variable keeping its registry value kind
# (REG_EXPAND_SZ stays ExpandString), then tells running programs about it.
function Set-UserEnv {
    param([string]$Key, [string]$Value)

    $envKey = (Get-Item -Path "HKCU:").OpenSubKey("Environment", $true)
    try {
        $kind = [Microsoft.Win32.RegistryValueKind]::String
        if ($Value.Contains("%")) {
            $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
        } elseif ($null -ne $envKey.GetValue($Key)) {
            $kind = $envKey.GetValueKind($Key)
        }
        $envKey.SetValue($Key, $Value, $kind)
    } finally {
        $envKey.Close()
    }

    Publish-EnvChange
}

# Broadcasts WM_SETTINGCHANGE ("Environment") so Explorer and terminals opened
# from now on read the new user PATH without signing out.
function Publish-EnvChange {
    if (-not ("Win32.GentleAiNativeMethods" -as [Type])) {
        Add-Type -Namespace Win32 -Name GentleAiNativeMethods -MemberDefinition @"
[DllImport("user32.dll", SetLastError = true, CharSet = CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(
    IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam,
    uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
"@
    }

    $hwndBroadcast = [IntPtr]0xffff
    $wmSettingChange = 0x1a
    $result = [UIntPtr]::Zero
    [Win32.GentleAiNativeMethods]::SendMessageTimeout($hwndBroadcast, $wmSettingChange, [UIntPtr]::Zero, "Environment", 2, 5000, [ref]$result) | Out-Null
}

# Puts Directory first on the persisted user PATH (HKCU\Environment) and on this
# session's PATH. Returns "session" when this session already resolves Directory
# (Machine PATH, $PROFILE): nothing is written. Returns "present" when only the
# user PATH held it (the session gains it), and "added" when it was written.
function Add-UserPathEntry {
    param([string]$Directory)

    $normalize = {
        param([string]$Entry)
        [Environment]::ExpandEnvironmentVariables($Entry.Trim()).TrimEnd("\").ToLowerInvariant()
    }
    $wanted = & $normalize $Directory

    $separator = [string][IO.Path]::PathSeparator
    $session = @($env:PATH -split [regex]::Escape($separator) | Where-Object { $_ })
    if (@($session | Where-Object { (& $normalize $_) -eq $wanted }).Count -gt 0) { return "session" }

    $current = Get-UserEnv -Key "Path"
    $entries = @()
    if ($current) { $entries = @($current -split ";" | Where-Object { $_ }) }
    $present = @($entries | Where-Object { (& $normalize $_) -eq $wanted }).Count -gt 0
    if (-not $present) {
        Set-UserEnv -Key "Path" -Value ((@($Directory) + $entries) -join ";")
    }

    $env:PATH = ((@($Directory) + $session) -join $separator)

    if ($present) { return "present" }
    return "added"
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
