#!/usr/bin/env bash
# lib.sh — shared test helpers for gentle-ai E2E tests
# Sourced by e2e_test.sh; never executed directly.
set -euo pipefail

# ---------------------------------------------------------------------------
# Colors
# ---------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ---------------------------------------------------------------------------
# Counters
# ---------------------------------------------------------------------------
PASSED=0
FAILED=0
SKIPPED=0

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log_test()  { printf "${YELLOW}[TEST]${NC}  %s\n" "$1"; }
log_pass()  { printf "${GREEN}[PASS]${NC}  %s\n" "$1"; PASSED=$((PASSED + 1)); }
log_fail()  { printf "${RED}[FAIL]${NC}  %s\n" "$1"; FAILED=$((FAILED + 1)); }
log_skip()  { printf "${BLUE}[SKIP]${NC}  %s\n" "$1"; SKIPPED=$((SKIPPED + 1)); }
log_info()  { printf "${BLUE}[INFO]${NC}  %s\n" "$1"; }

# ---------------------------------------------------------------------------
# Binary resolution
# ---------------------------------------------------------------------------
# The binary should be built and placed at /usr/local/bin/gentle-ai inside
# the Docker container. If not found, fall back to $HOME/gentle-ai or the
# current directory.
# Resolution priority (highest → lowest):
#   1. ./gentle-ai in the current repo directory (freshly built local binary)
#   2. ~/gentle-ai (explicit copy in home)
#   3. gentle-ai on PATH (system-installed, e.g. Homebrew)
# This ensures `go build ./cmd/gentle-ai && bash e2e/e2e_test.sh` always
# tests the locally built binary rather than the installed release version.
resolve_binary() {
    # Prefer the locally built binary (./gentle-ai) produced by `go build ./cmd/gentle-ai`.
    # We check both the current directory and the script's parent directory so
    # the resolver works whether the test is invoked from the repo root or from e2e/.
    local repo_root
    repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
    if [ -x "$repo_root/gentle-ai" ]; then
        echo "$repo_root/gentle-ai"
    elif [ -x "./gentle-ai" ]; then
        echo "./gentle-ai"
    elif [ -x "$HOME/gentle-ai" ]; then
        echo "$HOME/gentle-ai"
    elif command -v gentle-ai >/dev/null 2>&1; then
        echo "gentle-ai"
    else
        echo ""
    fi
}

# ---------------------------------------------------------------------------
# Cleanup helpers
# ---------------------------------------------------------------------------

# cleanup_test_env — reset filesystem state between tests.
# Removes config dirs and files that the installer writes.
cleanup_test_env() {
    rm -rf "$HOME/.config/opencode" 2>/dev/null || true
    rm -rf "$HOME/.config/gga" 2>/dev/null || true
    rm -rf "$HOME/.config/Windsurf" 2>/dev/null || true
    rm -rf "$HOME/.claude" 2>/dev/null || true
    rm -rf "$HOME/.codex" 2>/dev/null || true
    rm -rf "$HOME/.gemini" 2>/dev/null || true
    rm -rf "$HOME/.gentle-ai" 2>/dev/null || true
    rm -rf "$HOME/.codeium" 2>/dev/null || true
    rm -rf "$HOME/.cursor" 2>/dev/null || true
    rm -rf "$HOME/.qwen" 2>/dev/null || true
    rm -rf "$HOME/.kiro" 2>/dev/null || true
    rm -rf "$HOME/.kimi" 2>/dev/null || true
    mkdir -p "$HOME/.config"
}

# setup_fake_configs — seed fake config files so backup tests have something
# to snapshot and restore.
setup_fake_configs() {
    mkdir -p "$HOME/.config/opencode"
    echo '{"fake-settings": true}' > "$HOME/.config/opencode/opencode.json"

    mkdir -p "$HOME/.claude"
    echo '# Fake CLAUDE.md' > "$HOME/.claude/CLAUDE.md"

    mkdir -p "$HOME/.claude/mcp"
    echo '{"fake": true}' > "$HOME/.claude/mcp/engram.json"
}

# ---------------------------------------------------------------------------
# Assertion helpers
# ---------------------------------------------------------------------------

# assert_file_exists FILE LABEL
# Checks that FILE exists and is a regular file (non-empty by default).
assert_file_exists() {
    local file="$1"
    local label="${2:-$file}"
    if [ -f "$file" ]; then
        log_pass "File exists: $label"
        return 0
    else
        log_fail "File NOT found: $label ($file)"
        return 1
    fi
}

# assert_file_not_exists FILE LABEL
assert_file_not_exists() {
    local file="$1"
    local label="${2:-$file}"
    if [ ! -f "$file" ]; then
        log_pass "File correctly absent: $label"
        return 0
    else
        log_fail "File should NOT exist: $label ($file)"
        return 1
    fi
}

# assert_dir_exists DIR LABEL
assert_dir_exists() {
    local dir="$1"
    local label="${2:-$dir}"
    if [ -d "$dir" ]; then
        log_pass "Directory exists: $label"
        return 0
    else
        log_fail "Directory NOT found: $label ($dir)"
        return 1
    fi
}

# assert_file_contains FILE PATTERN LABEL
# Checks that FILE contains a grep-compatible PATTERN.
assert_file_contains() {
    local file="$1"
    local pattern="$2"
    local label="${3:-$file contains '$pattern'}"
    if [ ! -f "$file" ]; then
        log_fail "Cannot check content — file not found: $file"
        return 1
    fi
    if grep -q "$pattern" "$file"; then
        log_pass "$label"
        return 0
    else
        log_fail "Pattern NOT found: '$pattern' in $file"
        return 1
    fi
}

# assert_file_not_contains FILE PATTERN LABEL
# Checks that FILE does NOT contain a grep-compatible PATTERN.
assert_file_not_contains() {
    local file="$1"
    local pattern="$2"
    local label="${3:-$file does NOT contain '$pattern'}"
    if [ ! -f "$file" ]; then
        # File doesn't exist = pattern not found. That's a pass.
        log_pass "$label (file doesn't exist)"
        return 0
    fi
    if grep -q "$pattern" "$file"; then
        log_fail "Pattern FOUND (unexpected): '$pattern' in $file"
        return 1
    else
        log_pass "$label"
        return 0
    fi
}

# assert_file_size_min FILE BYTES LABEL
# Checks that FILE is at least BYTES bytes.
assert_file_size_min() {
    local file="$1"
    local min_bytes="$2"
    local label="${3:-$file >= $min_bytes bytes}"
    if [ ! -f "$file" ]; then
        log_fail "Cannot check size — file not found: $file"
        return 1
    fi
    local actual_size
    actual_size=$(wc -c < "$file" | tr -d ' ')
    if [ "$actual_size" -ge "$min_bytes" ]; then
        log_pass "$label (${actual_size}b)"
        return 0
    else
        log_fail "File too small: $file is ${actual_size}b, expected >= ${min_bytes}b"
        return 1
    fi
}

# assert_valid_json FILE LABEL
# Checks that FILE contains parseable JSON.
assert_valid_json() {
    local file="$1"
    local label="${2:-$file is valid JSON}"
    if [ ! -f "$file" ]; then
        log_fail "Cannot check JSON — file not found: $file"
        return 1
    fi
    # Use python3 if available, else node, else skip
    if command -v python3 >/dev/null 2>&1; then
        if python3 -c "import json; json.load(open('$file'))" 2>/dev/null; then
            log_pass "$label"
            return 0
        else
            log_fail "Invalid JSON: $file"
            return 1
        fi
    elif command -v node >/dev/null 2>&1; then
        if node -e "JSON.parse(require('fs').readFileSync('$file','utf8'))" 2>/dev/null; then
            log_pass "$label"
            return 0
        else
            log_fail "Invalid JSON: $file"
            return 1
        fi
    else
        log_skip "No JSON parser available to validate $file"
        return 0
    fi
}

# json_files_equal FILE1 FILE2
# Returns 0 if both files contain semantically equal JSON (key order ignored).
# Uses python3 for comparison (available in CI and most dev machines).
json_files_equal() {
    local file1="$1"
    local file2="$2"
    if ! command -v python3 >/dev/null 2>&1; then
        # Fallback: byte comparison (may false-fail on key reorder)
        [ "$(md5sum "$file1" | cut -d' ' -f1)" = "$(md5sum "$file2" | cut -d' ' -f1)" ]
        return $?
    fi
    python3 -c "
import json, sys
a = json.load(open(sys.argv[1]))
b = json.load(open(sys.argv[2]))
sys.exit(0 if a == b else 1)
" "$file1" "$file2"
}

# assert_file_count DIR PATTERN EXPECTED LABEL
# Checks that the number of files matching glob PATTERN in DIR equals EXPECTED.
assert_file_count() {
    local dir="$1"
    local pattern="$2"
    local expected="$3"
    local label="${4:-$dir/$pattern count == $expected}"
    if [ ! -d "$dir" ]; then
        log_fail "Cannot count files — directory not found: $dir"
        return 1
    fi
    local actual
    actual=$(find "$dir" -name "$pattern" -type f 2>/dev/null | wc -l | tr -d ' ')
    if [ "$actual" -eq "$expected" ]; then
        log_pass "$label (found $actual)"
        return 0
    else
        log_fail "File count mismatch in $dir: expected $expected '$pattern' files, got $actual"
        return 1
    fi
}

# assert_file_count_min DIR PATTERN MIN LABEL
# Checks that the number of files matching PATTERN >= MIN.
assert_file_count_min() {
    local dir="$1"
    local pattern="$2"
    local min="$3"
    local label="${4:-$dir/$pattern count >= $min}"
    if [ ! -d "$dir" ]; then
        log_fail "Cannot count files — directory not found: $dir"
        return 1
    fi
    local actual
    actual=$(find "$dir" -name "$pattern" -type f 2>/dev/null | wc -l | tr -d ' ')
    if [ "$actual" -ge "$min" ]; then
        log_pass "$label (found $actual)"
        return 0
    else
        log_fail "File count too low in $dir: expected >= $min '$pattern' files, got $actual"
        return 1
    fi
}

# assert_md5_match FILE1 FILE2 LABEL
# Checks that FILE1 and FILE2 have identical content.
assert_md5_match() {
    local file1="$1"
    local file2="$2"
    local label="${3:-$file1 == $file2}"
    if [ ! -f "$file1" ]; then
        log_fail "Cannot compare — file not found: $file1"
        return 1
    fi
    if [ ! -f "$file2" ]; then
        log_fail "Cannot compare — file not found: $file2"
        return 1
    fi
    local hash1 hash2
    hash1=$(md5sum "$file1" | cut -d' ' -f1)
    hash2=$(md5sum "$file2" | cut -d' ' -f1)
    if [ "$hash1" = "$hash2" ]; then
        log_pass "$label"
        return 0
    else
        log_fail "Content mismatch: $file1 ($hash1) != $file2 ($hash2)"
        return 1
    fi
}

# assert_no_duplicate_section FILE SECTION_ID LABEL
# Checks that the gentle-ai section marker appears exactly once (no duplicates).
assert_no_duplicate_section() {
    local file="$1"
    local section_id="$2"
    local label="${3:-No duplicate section '$section_id' in $file}"
    if [ ! -f "$file" ]; then
        log_fail "Cannot check sections — file not found: $file"
        return 1
    fi
    local marker="<!-- gentle-ai:${section_id} -->"
    local count
    count=$(grep -c "$marker" "$file" 2>/dev/null || echo "0")
    if [ "$count" -eq 1 ]; then
        log_pass "$label"
        return 0
    elif [ "$count" -eq 0 ]; then
        log_fail "Section marker not found: $marker in $file"
        return 1
    else
        log_fail "DUPLICATE section marker ($count occurrences): $marker in $file"
        return 1
    fi
}

# assert_output_contains OUTPUT PATTERN LABEL
# Checks that the output string contains a grep-compatible pattern.
assert_output_contains() {
    local output="$1"
    local pattern="$2"
    local label="${3:-output contains '$pattern'}"
    if echo "$output" | grep -qi "$pattern"; then
        log_pass "$label"
        return 0
    else
        log_fail "Output does NOT contain '$pattern'"
        return 1
    fi
}

# assert_output_not_contains OUTPUT PATTERN LABEL
assert_output_not_contains() {
    local output="$1"
    local pattern="$2"
    local label="${3:-output does NOT contain '$pattern'}"
    if echo "$output" | grep -qi "$pattern"; then
        log_fail "Output unexpectedly contains '$pattern'"
        return 1
    else
        log_pass "$label"
        return 0
    fi
}

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
print_summary() {
    echo ""
    echo "========================================"
    printf "  ${GREEN}PASSED${NC}: %d\n" "$PASSED"
    printf "  ${RED}FAILED${NC}: %d\n" "$FAILED"
    printf "  ${BLUE}SKIPPED${NC}: %d\n" "$SKIPPED"
    echo "  TOTAL : $((PASSED + FAILED + SKIPPED))"
    echo "========================================"

    if [ "$FAILED" -gt 0 ]; then
        printf "\n%bSome tests failed.%b\n" "$RED" "$NC"
        return 1
    fi

    printf "\n%bAll tests passed.%b\n" "$GREEN" "$NC"
    return 0
}
