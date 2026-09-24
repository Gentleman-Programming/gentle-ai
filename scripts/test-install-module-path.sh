#!/usr/bin/env bash
# Regression tests for gentle-ai#4689: scripts/install.sh must derive the
# go-install module path (and the beta GONOSUMDB/GOPRIVATE/GONOPROXY
# patterns) from go.mod at the resolved source ref — latest release tag for
# stable, main commit SHA for beta — instead of hard-coding /v3. A future
# /vN bump must need no installer change.
#
# Each test sources install.sh in a fresh subshell (safe: main only runs
# when the script is executed, not sourced) and mocks curl/go. Compatible
# with macOS bash 3.2. Idempotence of prepend_go_env_pattern is covered by
# the Go guard TestInstallScriptBetaGoInstallBypassesPublicGoProxy.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SH="${SCRIPT_DIR}/install.sh"
[ -r "${INSTALL_SH}" ] || { printf 'install.sh not readable at %s\n' "${INSTALL_SH}" >&2; exit 2; }

C_GREEN=''; C_RED=''; C_NC=''
[ -t 1 ] && [ "${TERM:-dumb}" != "dumb" ] && { C_GREEN='\033[0;32m'; C_RED='\033[0;31m'; C_NC='\033[0m'; }

TESTS_TOTAL=0; TESTS_PASSED=0; TESTS_FAILED=0; FAILED_NAMES=()

# Each test runs in its own subshell so fatal()'s exit cannot kill the
# runner and mock state starts clean every time.
run_test() {
    TESTS_TOTAL=$((TESTS_TOTAL + 1))
    if (
        # shellcheck disable=SC1090
        . "${INSTALL_SH}"
        info() { :; }; success() { :; }; warn() { :; }; error() { :; }; step() { :; }
        "$2"
    ); then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        printf '  %sPASS%s %s\n' "${C_GREEN}" "${C_NC}" "$1"
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_NAMES+=("$1")
        printf '  %sFAIL%s %s\n' "${C_RED}" "${C_NC}" "$1"
    fi
}

assert_eq() {
    [ "$2" = "$3" ] && return 0
    printf '         %s\n         expected: %s\n         got:      %s\n' "$1" "$2" "$3" >&2
    return 1
}

assert_not_contains() {
    case "$2" in *"$3"*) printf '         %s\n         must NOT contain: %s\n' "$1" "$3" >&2; return 1 ;; esac
    return 0
}

# ---- mocks -------------------------------------------------------------------
# State consulted by mock_curl; reset_mocks sets the defaults per test.
EXPECTED_TAG=""; EXPECTED_SHA=""
GOMOD_MODULE="github.com/gentleman-programming/gentle-ai/v3"
GOMOD_HTTP="200"; GOMOD_NO_MODULE="0"; RELEASES_HTTP="200"; COMMITS_HTTP="200"
INSTALL_LOG=""; GO_WAS_CALLED=0

reset_mocks() {
    EXPECTED_TAG=""; EXPECTED_SHA=""
    GOMOD_MODULE="github.com/gentleman-programming/gentle-ai/v3"
    GOMOD_HTTP="200"; GOMOD_NO_MODULE="0"; RELEASES_HTTP="200"; COMMITS_HTTP="200"
    INSTALL_LOG=""; GO_WAS_CALLED=0
}

# curl is always invoked as `curl -sL -w "\n%{http_code}" URL` by the code
# under test; discarding dash-flags leaves the URL as the lone positional.
mock_curl() {
    local url=""
    while [ $# -gt 0 ]; do
        case "$1" in -*) shift ;; *) url="$1"; shift ;; esac
    done
    case "${url}" in
        *"/releases/latest")
            if [ "${RELEASES_HTTP}" != "200" ]; then printf '\n%s\n' "${RELEASES_HTTP}"; return 0; fi
            printf '{"tag_name":"%s"}\n200\n' "${EXPECTED_TAG}" ;;
        *"/commits/main")
            if [ "${COMMITS_HTTP}" != "200" ]; then printf '\n%s\n' "${COMMITS_HTTP}"; return 0; fi
            printf '{"sha":"%s"}\n200\n' "${EXPECTED_SHA}" ;;
        *"/go.mod")
            if [ "${GOMOD_HTTP}" != "200" ]; then printf '\n%s\n' "${GOMOD_HTTP}"; return 0; fi
            if [ "${GOMOD_NO_MODULE}" = "1" ]; then
                printf '// just a comment\n\ngo 1.24\n\n200\n'
            else
                printf 'module %s\n\ngo 1.24\n\n200\n' "${GOMOD_MODULE}"
            fi ;;
        *) printf '\n500\n' ;;
    esac
}

mock_go() {
    case "$1" in
        install) GO_WAS_CALLED=1; shift; INSTALL_LOG="$*" ;;
        env)     case "${2:-}" in GOBIN) printf '/usr/local/bin\n' ;; GOPATH) printf '/root/go\n' ;; *) printf '\n' ;; esac ;;
    esac
}

# ---- tests -------------------------------------------------------------------

# Stable with the current /v3 go.mod: resolve the latest tag, derive the
# module at that tag, install @<tag>.
test_stable_v3_current() {
    reset_mocks; EXPECTED_TAG="v3.7.0"
    curl() { mock_curl "$@"; }; go() { mock_go "$@"; }
    CHANNEL="stable"; LATEST_VERSION=""
    install_go
    assert_eq "install target" \
        "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@v3.7.0" \
        "${INSTALL_LOG}"
}

# The issue's recurrence fixture: a future tag whose go.mod declares /v4
# must be followed without touching the script.
test_stable_v4_regression() {
    reset_mocks; EXPECTED_TAG="v4.0.0"
    GOMOD_MODULE="github.com/gentleman-programming/gentle-ai/v4"
    curl() { mock_curl "$@"; }; go() { mock_go "$@"; }
    CHANNEL="stable"; LATEST_VERSION=""
    install_go
    assert_eq "install target follows the new /v4 path" \
        "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0" \
        "${INSTALL_LOG}"
}

# Beta: resolve main's SHA, derive the module at that SHA, install @<sha>,
# and export env patterns carrying the derived path (never a /v3 literal).
test_beta_v5_derives_module_and_env() {
    reset_mocks; EXPECTED_SHA="abcdef1234567890abcdef1234567890abcdef12"
    GOMOD_MODULE="github.com/gentleman-programming/gentle-ai/v5"
    unset GONOSUMDB GOPRIVATE GONOPROXY 2>/dev/null || true
    curl() { mock_curl "$@"; }; go() { mock_go "$@"; }
    CHANNEL="beta"; LATEST_VERSION=""
    install_go
    assert_eq "install target uses derived /v5 path and SHA" \
        "github.com/gentleman-programming/gentle-ai/v5/cmd/gentle-ai@abcdef1234567890abcdef1234567890abcdef12" \
        "${INSTALL_LOG}" || return 1
    assert_eq "GONOSUMDB" "github.com/gentleman-programming/gentle-ai/v5" "${GONOSUMDB:-}" || return 1
    assert_eq "GOPRIVATE" "github.com/gentleman-programming/gentle-ai/v5" "${GOPRIVATE:-}" || return 1
    assert_eq "GONOPROXY" "github.com/gentleman-programming/gentle-ai/v5" "${GONOPROXY:-}" || return 1
    assert_not_contains "env pattern" "${GONOSUMDB:-}" "/v3"
}

# Fail-closed: a bad go.mod resolution (404, or 200 without a module line)
# must abort before go install ever runs. install_go runs in an inner
# subshell because fatal() exits.
test_go_mod_fail_closed() {
    local scenario
    for scenario in 404 no-module-line; do
        reset_mocks; EXPECTED_TAG="v3.7.0"
        if [ "${scenario}" = "404" ]; then GOMOD_HTTP="404"; else GOMOD_NO_MODULE="1"; fi
        curl() { mock_curl "$@"; }
        go() { GO_WAS_CALLED=1; }
        CHANNEL="stable"; LATEST_VERSION=""
        local rc=0
        ( install_go ) || rc=$?
        if [ "${GO_WAS_CALLED}" = "1" ]; then
            printf '         %s: go install ran after fatal\n' "${scenario}" >&2
            return 1
        fi
        if [ "${rc}" = "0" ]; then
            printf '         %s: install_go returned 0, expected fatal\n' "${scenario}" >&2
            return 1
        fi
    done
}

# The execution guard must run main when the script is piped to bash via
# stdin (`curl | bash`: BASH_SOURCE[0] empty, $0=bash), not only on direct
# execution — otherwise the documented primary invocation silently no-ops.
test_stdin_execution_runs_main() {
    local out
    out="$(cat "${INSTALL_SH}" | bash -s -- --help)"
    case "${out}" in *"Usage: install.sh"*) return 0 ;; esac
    printf '         stdin execution did not reach main\n' >&2
    return 1
}

# ---- runner ------------------------------------------------------------------
run_test "stable channel installs /v3 at the resolved tag"            test_stable_v3_current
run_test "stable channel follows /v4 when go.mod declares it"         test_stable_v4_regression
run_test "beta channel derives module + env patterns from go.mod"     test_beta_v5_derives_module_and_env
run_test "bad go.mod resolution fails closed (no install)"            test_go_mod_fail_closed
run_test "stdin execution (curl | bash) still runs main"              test_stdin_execution_runs_main

if [ "${TESTS_FAILED}" -eq 0 ]; then
    printf '%sok%s — %d/%d installer module-path tests passed\n' \
        "${C_GREEN}" "${C_NC}" "${TESTS_PASSED}" "${TESTS_TOTAL}"
    exit 0
fi
printf '%sFAIL%s — %d/%d failed:\n' "${C_RED}" "${C_NC}" "${TESTS_FAILED}" "${TESTS_TOTAL}"
for name in "${FAILED_NAMES[@]}"; do printf '  - %s\n' "${name}"; done
exit 1
