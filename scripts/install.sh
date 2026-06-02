#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# Gentle-AI — Install Script
# Ecosystem, Frameworks, Workflows for AI coding agents.
#
# Usage:
#   curl -sL https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh | bash
#
# Or download and run:
#   curl -sLO https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/scripts/install.sh
#   chmod +x install.sh
#   ./install.sh
# ============================================================================

GITHUB_OWNER="Gentleman-Programming"
GITHUB_REPO="gentle-ai"
BINARY_NAME="gentle-ai"
BREW_TAP="Gentleman-Programming/homebrew-tap"

# ============================================================================
# Color support
# ============================================================================

setup_colors() {
    if [ -t 1 ] && [ "${TERM:-}" != "dumb" ]; then
        RED='\033[0;31m'
        GREEN='\033[0;32m'
        YELLOW='\033[1;33m'
        BLUE='\033[0;34m'
        CYAN='\033[0;36m'
        BOLD='\033[1m'
        DIM='\033[2m'
        NC='\033[0m'
    else
        RED='' GREEN='' YELLOW='' BLUE='' CYAN='' BOLD='' DIM='' NC=''
    fi
}

# ============================================================================
# Logging helpers
# ============================================================================

info()    { echo -e "${BLUE}[info]${NC}    $*"; }
success() { echo -e "${GREEN}[ok]${NC}      $*"; }
warn()    { echo -e "${YELLOW}[warn]${NC}    $*"; }
error()   { echo -e "${RED}[error]${NC}   $*" >&2; }
fatal()   { error "$@"; exit 1; }
step()    { echo -e "\n${CYAN}${BOLD}==>${NC} ${BOLD}$*${NC}"; }

# ============================================================================
# Help
# ============================================================================

show_help() {
    cat <<EOF
${BOLD}Gentle-AI installer${NC}

Usage: install.sh [OPTIONS]

Options:
  --method METHOD   Force install method: brew, go, binary (default: auto-detect)
  --dir DIR         Custom install directory for binary method
  --insecure        Skip checksum verification (not recommended)
  -h, --help        Show this help

Install methods (auto-detected in priority order):
  1. brew    — Homebrew tap (recommended)
  2. go      — go install from source
  3. binary  — Pre-built binary from GitHub Releases

Examples:
  curl -sL https://raw.githubusercontent.com/${GITHUB_OWNER}/${GITHUB_REPO}/main/scripts/install.sh | bash
  ./install.sh --method binary
  ./install.sh --method binary --dir \$HOME/.local/bin
  ./install.sh --method binary --insecure   # skip checksum (not recommended)

EOF
}

# ============================================================================
# Platform detection
# ============================================================================

detect_platform() {
    local uname_os uname_arch

    uname_os="$(uname -s)"
    uname_arch="$(uname -m)"

    case "$uname_os" in
        Darwin) OS="darwin"; OS_LABEL="macOS"; GORELEASER_OS="darwin" ;;
        Linux)  OS="linux";  OS_LABEL="Linux"; GORELEASER_OS="linux" ;;
        *)      fatal "Unsupported OS: $uname_os. Only macOS and Linux are supported." ;;
    esac

    case "$uname_arch" in
        x86_64|amd64)   ARCH="amd64" ;;
        arm64|aarch64)  ARCH="arm64" ;;
        *)              fatal "Unsupported architecture: $uname_arch. Only amd64 and arm64 are supported." ;;
    esac

    success "Platform: ${OS_LABEL} (${OS}/${ARCH})"
}

# ============================================================================
# GoReleaser archive naming
#
# From .goreleaser.yaml:
#   name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
#
# GoReleaser v2 {{ .Os }} produces GOOS values (lowercase: darwin, linux)
# GoReleaser {{ .Arch }} produces GOARCH values (amd64, arm64)
# Examples:
#   gentle-ai_1.0.0_darwin_arm64.tar.gz
#   gentle-ai_1.0.0_linux_amd64.tar.gz
# ============================================================================

get_archive_name() {
    local version="$1"
    echo "${BINARY_NAME}_${version}_${GORELEASER_OS}_${ARCH}.tar.gz"
}

# ============================================================================
# Prerequisites
# ============================================================================

check_prerequisites() {
    step "Checking prerequisites"

    local missing=()

    if ! command -v curl &>/dev/null; then
        missing+=("curl")
    fi

    if ! command -v git &>/dev/null; then
        missing+=("git")
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        fatal "Missing required tools: ${missing[*]}. Please install them and try again."
    fi

    success "curl and git are available"
}

# ============================================================================
# Install method detection
# ============================================================================

detect_install_method() {
    if [ -n "${FORCE_METHOD:-}" ]; then
        case "$FORCE_METHOD" in
            brew|go|binary) INSTALL_METHOD="$FORCE_METHOD" ;;
            *) fatal "Unknown install method: $FORCE_METHOD. Use: brew, go, or binary" ;;
        esac
        info "Using forced method: $INSTALL_METHOD"
        return
    fi

    step "Detecting best install method"

    # Priority: brew > binary > go
    # Brew handles upgrades natively and is instant.
    # Binary download from GitHub Releases is always up-to-date.
    # go install is last resort because the Go module proxy can lag
    # behind new tags for up to 30 minutes, causing @latest to install
    # a stale version.
    if command -v brew &>/dev/null; then
        INSTALL_METHOD="brew"
        success "Homebrew found — will install via brew tap"
    else
        INSTALL_METHOD="binary"
        info "Will download pre-built binary from GitHub Releases"
    fi
}

# ============================================================================
# Install via Homebrew
# ============================================================================

install_brew() {
    step "Installing via Homebrew"

    # Always refresh the tap to pick up new releases
    info "Refreshing ${BREW_TAP}..."
    brew untap "$BREW_TAP" 2>/dev/null || true
    if ! brew tap "$BREW_TAP"; then
        fatal "Failed to tap $BREW_TAP"
    fi

    if brew list "$BINARY_NAME" &>/dev/null; then
        info "Already installed, upgrading ${BINARY_NAME}..."
        if brew upgrade "$BINARY_NAME" 2>/dev/null; then
            success "Upgraded ${BINARY_NAME} via Homebrew"
        else
            # "already up-to-date" also exits non-zero on some brew versions
            success "${BINARY_NAME} is already at the latest version"
        fi
    else
        info "Installing ${BINARY_NAME}..."
        if brew install "$BINARY_NAME"; then
            success "Installed ${BINARY_NAME} via Homebrew"
        else
            fatal "Failed to install ${BINARY_NAME} via Homebrew"
        fi
    fi
}

# ============================================================================
# Install via go install
# ============================================================================

install_go() {
    step "Installing via go install"

    local go_package="github.com/${GITHUB_OWNER,,}/${GITHUB_REPO}/cmd/${BINARY_NAME}@latest"

    info "Running: go install ${go_package}"
    if ! go install "$go_package"; then
        fatal "Failed to install via go install. Make sure Go is properly configured."
    fi

    # Verify GOBIN / GOPATH/bin is in PATH
    local gobin
    gobin="$(go env GOBIN)"
    if [ -z "$gobin" ]; then
        gobin="$(go env GOPATH)/bin"
    fi

    if [[ ":$PATH:" != *":$gobin:"* ]]; then
        warn "${gobin} is not in your PATH"
        warn "Add this to your shell profile: export PATH=\"\$PATH:${gobin}\""
    fi

    success "Installed ${BINARY_NAME} via go install"
}

# ============================================================================
# Install via binary download
# ============================================================================

get_latest_version() {
    local url="https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/latest"

    info "Fetching latest release from GitHub..."

    local response
    response="$(curl -sL -w "\n%{http_code}" "$url")" || fatal "Failed to fetch latest release"

    local http_code body
    http_code="$(echo "$response" | tail -n1)"
    body="$(echo "$response" | sed '$d')"

    if [ "$http_code" != "200" ]; then
        fatal "GitHub API returned HTTP $http_code. Rate limited? Try again later or use --method brew/go"
    fi

    # Extract tag_name — works without jq
    LATEST_VERSION="$(echo "$body" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"

    if [ -z "$LATEST_VERSION" ]; then
        fatal "Could not determine latest version from GitHub API response"
    fi

    # Strip leading 'v' for archive naming (goreleaser uses version without v prefix)
    VERSION_NUMBER="${LATEST_VERSION#v}"

    success "Latest version: ${LATEST_VERSION}"
}

install_binary() {
    step "Installing pre-built binary"

    get_latest_version

    local archive_name
    archive_name="$(get_archive_name "$VERSION_NUMBER")"
    local download_url="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/download/${LATEST_VERSION}/${archive_name}"
    local checksums_url="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/download/${LATEST_VERSION}/checksums.txt"

    # Create temp directory — clean up on exit
    local tmpdir
    tmpdir="$(mktemp -d)"
    trap '[ -n "${tmpdir:-}" ] && rm -rf "$tmpdir"' EXIT

    # Download archive
    info "Downloading ${archive_name}..."
    if ! curl -sfL -o "${tmpdir}/${archive_name}" "$download_url"; then
        fatal "Failed to download ${download_url}"
    fi

    # Verify file was actually downloaded (not a 404 HTML page)
    local file_size
    file_size="$(wc -c < "${tmpdir}/${archive_name}" | tr -d '[:space:]')"
    if [ "$file_size" -lt 1000 ]; then
        fatal "Downloaded file is suspiciously small (${file_size} bytes). Archive may not exist for this platform."
    fi

    success "Downloaded ${archive_name} (${file_size} bytes)"

    # Download and verify checksum — fail closed unless --insecure is set
    info "Verifying checksum..."
    if curl -sL -o "${tmpdir}/checksums.txt" "$checksums_url"; then
        local expected_checksum
        expected_checksum="$(grep "${archive_name}" "${tmpdir}/checksums.txt" 2>/dev/null | awk '{print $1}' || true)"

        if [ -n "$expected_checksum" ]; then
            local actual_checksum
            if command -v sha256sum &>/dev/null; then
                actual_checksum="$(sha256sum "${tmpdir}/${archive_name}" | awk '{print $1}')"
            elif command -v shasum &>/dev/null; then
                actual_checksum="$(shasum -a 256 "${tmpdir}/${archive_name}" | awk '{print $1}')"
            else
                if [ "$INSECURE" = "true" ]; then
                    warn "No sha256sum or shasum found — checksum verification skipped (--insecure)"
                    actual_checksum="$expected_checksum"
                else
                    fatal "No sha256sum or shasum tool found. Cannot verify checksum.\nInstall coreutils (sha256sum) or use --insecure to skip (not recommended)."
                fi
            fi

            if [ "$actual_checksum" != "$expected_checksum" ]; then
                fatal "Checksum mismatch!\n  Expected: ${expected_checksum}\n  Got:      ${actual_checksum}"
            fi
            success "Checksum verified"
        else
            if [ "$INSECURE" = "true" ]; then
                warn "Archive '${archive_name}' not found in checksums.txt — checksum verification skipped (--insecure)"
            else
                fatal "Archive '${archive_name}' not found in checksums.txt. Refusing to install unverified binary.\nUse --insecure to skip (not recommended)."
            fi
        fi
    else
        if [ "$INSECURE" = "true" ]; then
            warn "Could not download checksums.txt — checksum verification skipped (--insecure)"
        else
            fatal "Could not download checksums.txt from:\n  ${checksums_url}\nRefusing to install without integrity verification.\nUse --insecure to skip (not recommended)."
        fi
    fi

    # Extract binary
    info "Extracting ${BINARY_NAME}..."
    if ! tar -xzf "${tmpdir}/${archive_name}" -C "$tmpdir"; then
        fatal "Failed to extract archive"
    fi

    if [ ! -f "${tmpdir}/${BINARY_NAME}" ]; then
        fatal "Binary '${BINARY_NAME}' not found in archive"
    fi

    # Determine install directory
    local install_dir="${INSTALL_DIR:-}"

    if [ -z "$install_dir" ]; then
        if [ -d "/usr/local/bin" ] && [ -w "/usr/local/bin" ]; then
            install_dir="/usr/local/bin"
        elif [ "$(id -u)" = "0" ]; then
            install_dir="/usr/local/bin"
        else
            install_dir="${HOME}/.local/bin"
        fi
    fi

    # Create install dir if needed
    mkdir -p "$install_dir"

    # Install binary
    info "Installing to ${install_dir}/${BINARY_NAME}..."
    if cp "${tmpdir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}" 2>/dev/null; then
        chmod +x "${install_dir}/${BINARY_NAME}"
    elif command -v sudo &>/dev/null; then
        warn "Permission denied. Trying with sudo..."
        sudo cp "${tmpdir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}"
        sudo chmod +x "${install_dir}/${BINARY_NAME}"
    else
        fatal "Cannot write to ${install_dir}. Run with sudo or use --dir to specify a writable directory."
    fi

    success "Installed ${BINARY_NAME} to ${install_dir}/${BINARY_NAME}"

    # Check if install dir is in PATH
    if [[ ":$PATH:" != *":${install_dir}:"* ]]; then
        warn "${install_dir} is not in your PATH"
        echo ""
        warn "Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
        echo -e "  ${DIM}export PATH=\"\$PATH:${install_dir}\"${NC}"
        echo ""
    fi
}

# ============================================================================
# Verify installation
# ============================================================================

verify_installation() {
    step "Verifying installation"

    # Allow PATH changes to take effect
    hash -r 2>/dev/null || true

    if command -v "$BINARY_NAME" &>/dev/null; then
        local version_output
        version_output="$("$BINARY_NAME" version 2>&1 || true)"
        success "${BINARY_NAME} is installed: ${version_output}"
        return 0
    fi

    # Check common locations even if not in PATH
    local locations=(
        "/usr/local/bin/${BINARY_NAME}"
        "${HOME}/.local/bin/${BINARY_NAME}"
        "$(go env GOPATH 2>/dev/null || echo "")/bin/${BINARY_NAME}"
    )

    for loc in "${locations[@]}"; do
        if [ -n "$loc" ] && [ -x "$loc" ]; then
            local version_output
            version_output="$("$loc" version 2>&1 || true)"
            success "Found ${BINARY_NAME} at ${loc}: ${version_output}"
            warn "Binary location is not in your PATH. Add it to use '${BINARY_NAME}' directly."
            return 0
        fi
    done

    warn "Could not verify installation. You may need to restart your shell."
    return 0
}

# ============================================================================
# Print next steps
# ============================================================================

print_banner() {
    echo ""
    echo -e "${CYAN}${BOLD}"
    echo "   ____            _   _              _    ___ "
    echo "  / ___| ___ _ __ | |_| | ___        / \  |_ _|"
    echo " | |  _ / _ \ '_ \| __| |/ _ \_____ / _ \  | | "
    echo " | |_| |  __/ | | | |_| |  __/_____/ ___ \ | | "
    echo "  \____|\___|_| |_|\__|_|\___|    /_/   \_\___|"
    echo -e "${NC}"
    echo -e "  ${DIM}Gentle-AI — Ecosystem, Frameworks, Workflows${NC}"
    echo ""
}

print_next_steps() {
    echo ""
    echo -e "${GREEN}${BOLD}Installation complete!${NC}"
    echo ""
    echo -e "${BOLD}Next steps:${NC}"
    echo -e "  ${CYAN}1.${NC} Run ${BOLD}${BINARY_NAME}${NC} to start the TUI installer"
    echo -e "  ${CYAN}2.${NC} Select your AI agent(s) and tools to configure"
    echo -e "  ${CYAN}3.${NC} Follow the interactive prompts"
    echo ""
    echo -e "${DIM}For help: ${BINARY_NAME} --help${NC}"
    echo -e "${DIM}Docs:     https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}${NC}"
    echo ""
}

# ============================================================================
# Main
# ============================================================================

main() {
    setup_colors

    # Parse arguments
    FORCE_METHOD=""
    INSTALL_DIR=""
    INSECURE="false"

    while [ $# -gt 0 ]; do
        case "$1" in
            --method)
                [ $# -lt 2 ] && fatal "--method requires an argument"
                FORCE_METHOD="$2"; shift 2
                ;;
            --dir)
                [ $# -lt 2 ] && fatal "--dir requires an argument"
                INSTALL_DIR="$2"; shift 2
                ;;
            --insecure)
                INSECURE="true"; shift
                ;;
            -h|--help)
                setup_colors
                show_help
                exit 0
                ;;
            *)
                fatal "Unknown option: $1. Use --help for usage."
                ;;
        esac
    done

    print_banner

    step "Detecting platform"
    detect_platform

    check_prerequisites
    detect_install_method

    case "$INSTALL_METHOD" in
        brew)   install_brew ;;
        go)     install_go ;;
        binary) install_binary ;;
    esac

    verify_installation
    print_next_steps
}

main "$@"
