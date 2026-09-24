#!/usr/bin/env bash
# Tests for the version-aware module path derivation in scripts/install.sh.
#
# Issue: gentle-ai#4689 — the Unix installer used to hard-code /v3 in the
# go-install target and in the GONOSUMDB/GOPRIVATE/GONOPROXY env patterns.
# A future /v4 source revision silently broke every `curl | bash` install.
# The fix derives the module path from go.mod at the resolved ref (latest
# release tag for stable, main commit SHA for beta) so a /vN bump needs
# no code change here.
#
# Approach: source scripts/install.sh per test in an isolated subshell,
# replace curl/go with mock functions that return canned responses, and
# assert the arguments recorded for `go install` and the exported env
# vars. Compatible with macOS system bash 3.2 — no associative arrays, no
# ${var,,}, no mapfile. Each test must be self-contained because fatal()
# exits the subshell on failure and we cannot unwind state.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SH="${SCRIPT_DIR}/install.sh"

if [ ! -r "${INSTALL_SH}" ]; then
    printf 'install.sh not readable at %s\n' "${INSTALL_SH}" >&2
    exit 2
fi

# ---- output -----------------------------------------------------------------
if [ -t 1 ] && [ "${TERM:-dumb}" != "dumb" ]; then
    C_RED='\033[0;31m'
    C_GREEN='\033[0;32m'
    C_YELLOW='\033[1;33m'
    C_BOLD='\033[1m'
    C_DIM='\033[2m'
    C_NC='\033[0m'
else
    C_RED='' C_GREEN='' C_YELLOW='' C_BOLD='' C_DIM='' C_NC=''
fi

TESTS_TOTAL=0
TESTS_PASSED=0
TESTS_FAILED=0
FAILED_NAMES=()

run_test() {
    local name="$1"
    local test_fn="$2"
    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    # Each test runs in its own subshell so fatal() (which calls exit)
    # can't kill the runner. The subshell sources install.sh fresh so the
    # global state (MODULE_PATH, LATEST_VERSION, GONOSUMDB, etc.) starts
    # clean for every case.
    if (
        # shellcheck disable=SC1090
        . "${INSTALL_SH}"

        # Silence install.sh's logging so the runner output stays readable.
        # Each test can still override these locally if it wants to observe.
        info()    { :; }
        success() { :; }
        warn()    { :; }
        error()   { :; }
        step()    { :; }

        "${test_fn}"
    ); then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        printf '  %sPASS%s %s\n' "${C_GREEN}" "${C_NC}" "${name}"
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        FAILED_NAMES+=("${name}")
        printf '  %sFAIL%s %s\n' "${C_RED}" "${C_NC}" "${name}"
    fi
}

# ---- assertion helpers ------------------------------------------------------
assert_eq() {
    local label="$1"
    local expected="$2"
    local actual="$3"
    if [ "${expected}" = "${actual}" ]; then
        return 0
    fi
    printf '         %s\n         expected: %s\n         got:      %s\n' \
        "${label}" "${expected}" "${actual}" >&2
    return 1
}

assert_contains() {
    local label="$1"
    local haystack="$2"
    local needle="$3"
    if [[ "${haystack}" == *"${needle}"* ]]; then
        return 0
    fi
    printf '         %s\n         expected to contain: %s\n         got:                %s\n' \
        "${label}" "${needle}" "${haystack}" >&2
    return 1
}

assert_not_contains() {
    local label="$1"
    local haystack="$2"
    local needle="$3"
    if [[ "${haystack}" != *"${needle}"* ]]; then
        return 0
    fi
    printf '         %s\n         must NOT contain: %s\n         got:             %s\n' \
        "${label}" "${needle}" "${haystack}" >&2
    return 1
}

# ---- default mocks (overridable per test) ------------------------------------
# Per-test state — the test body resets these before defining its own
# overrides; tests run in fresh subshells so leakage between cases is
# impossible.
INSTALL_LOG=""
GO_WAS_CALLED=0
EXPECTED_TAG=""
EXPECTED_SHA=""
GOMOD_MODULE="github.com/gentleman-programming/gentle-ai/v3"
GOMOD_HTTP="200"
GOMOD_NO_MODULE="0"
RELEASES_HTTP="200"

# Generic curl mock. The real curl signature used inside install.sh is
# `curl -sL -w "\n%{http_code}" URL` (and the install_binary download uses
# `curl -sfL -o PATH URL`, but install_go never takes that branch). We
# don't need to support -o here. Discarding every flag with a leading dash
# leaves the URL as the lone positional arg, which is enough for routing.
mock_curl() {
    local url=""
    while [ $# -gt 0 ]; do
        case "$1" in
            -*) shift ;;
            *)   url="$1"; shift ;;
        esac
    done

    case "${url}" in
        *"/releases/latest")
            if [ "${RELEASES_HTTP}" != "200" ]; then
                printf '\n%s\n' "${RELEASES_HTTP}"
                return 0
            fi
            printf '{"tag_name":"%s","name":"%s"}\n200\n' \
                "${EXPECTED_TAG}" "${EXPECTED_TAG}"
            ;;
        *"/commits/main")
            if [ "${COMMITS_HTTP:-200}" != "200" ]; then
                printf '\n%s\n' "${COMMITS_HTTP:-200}"
                return 0
            fi
            printf '{"sha":"%s"}\n200\n' "${EXPECTED_SHA}"
            ;;
        *"/go.mod")
            if [ "${GOMOD_HTTP}" != "200" ]; then
                printf '\n%s\n' "${GOMOD_HTTP}"
                return 0
            fi
            if [ "${GOMOD_NO_MODULE}" = "1" ]; then
                # Valid HTTP, but no module line — exercises the parser
                # empty-match fatal path.
                printf '// just a comment\n\ngo 1.24\n\n200\n'
            else
                printf 'module %s\n\ngo 1.24\n\n200\n' "${GOMOD_MODULE}"
            fi
            ;;
        *)
            printf '\n500\n'
            ;;
    esac
    return 0
}

mock_go() {
    case "$1" in
        install)
            GO_WAS_CALLED=1
            shift
            INSTALL_LOG="$*"
            ;;
        env)
            case "${2:-}" in
                GOBIN)   printf '/usr/local/bin\n' ;;
                GOPATH)  printf '/root/go\n' ;;
                *)       printf '\n' ;;
            esac
            ;;
        *) ;;
    esac
    return 0
}

# ---- tests ------------------------------------------------------------------

# Stable channel with the current /v3 module declaration: the installer
# must resolve the latest tag, fetch go.mod at that tag, parse the module
# line, and call `go install <module>/cmd/gentle-ai@<tag>`.
test_stable_v3_current() {
    INSTALL_LOG=""
    GO_WAS_CALLED=0
    EXPECTED_TAG="v3.7.0"
    RELEASES_HTTP="200"
    GOMOD_HTTP="200"
    GOMOD_NO_MODULE="0"
    GOMOD_MODULE="github.com/gentleman-programming/gentle-ai/v3"

    curl() { mock_curl "$@"; }
    go()   { mock_go "$@"; }

    CHANNEL="stable"
    LATEST_VERSION=""
    install_go

    assert_eq "go install called" "1" "${GO_WAS_CALLED}" || return 1
    assert_eq "install target" \
        "github.com/gentleman-programming/gentle-ai/v3/cmd/gentle-ai@v3.7.0" \
        "${INSTALL_LOG}" || return 1
    return 0
}

# The actual regression: the repo bumps to /v4 tomorrow. The installer
# must use the new path without anyone touching this script.
test_stable_v4_regression() {
    INSTALL_LOG=""
    GO_WAS_CALLED=0
    EXPECTED_TAG="v4.0.0"
    RELEASES_HTTP="200"
    GOMOD_HTTP="200"
    GOMOD_NO_MODULE="0"
    GOMOD_MODULE="github.com/gentleman-programming/gentle-ai/v4"

    curl() { mock_curl "$@"; }
    go()   { mock_go "$@"; }

    CHANNEL="stable"
    LATEST_VERSION=""
    install_go

    assert_eq "go install called" "1" "${GO_WAS_CALLED}" || return 1
    assert_eq "install target follows the new /v4 path" \
        "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0" \
        "${INSTALL_LOG}" || return 1
    return 0
}

# Beta channel: resolve the main commit SHA, derive the module path at
# that SHA, install @<sha>, and export env patterns that no longer hard-
# code the owner literal or the /v3 suffix.
test_beta_v5_derives_module_and_env() {
    INSTALL_LOG=""
    GO_WAS_CALLED=0
    EXPECTED_SHA="abcdef1234567890abcdef1234567890abcdef12"
    COMMITS_HTTP="200"
    GOMOD_HTTP="200"
    GOMOD_NO_MODULE="0"
    GOMOD_MODULE="github.com/gentleman-programming/gentle-ai/v5"

    # Reset the env vars the installer touches so we can assert on what
    # install_go actually wrote. The bash 3.2-safe way to do that without
    # associative arrays is `unset`, then read back via indirect expansion.
    unset GONOSUMDB GOPRIVATE GONOPROXY 2>/dev/null || true

    curl() { mock_curl "$@"; }
    go()   { mock_go "$@"; }

    CHANNEL="beta"
    LATEST_VERSION=""

    install_go

    assert_eq "go install called" "1" "${GO_WAS_CALLED}" || return 1
    assert_eq "install target uses derived /v5 path and SHA" \
        "github.com/gentleman-programming/gentle-ai/v5/cmd/gentle-ai@abcdef1234567890abcdef1234567890abcdef12" \
        "${INSTALL_LOG}" || return 1

    # Env patterns must carry the derived module path (whatever major
    # version the source revision actually declares) and must NOT carry
    # the old hard-coded "/v3" literal.
    assert_eq "GONOSUMDB" \
        "github.com/gentleman-programming/gentle-ai/v5" \
        "${GONOSUMDB:-}" || return 1
    assert_eq "GOPRIVATE" \
        "github.com/gentleman-programming/gentle-ai/v5" \
        "${GOPRIVATE:-}" || return 1
    assert_eq "GONOPROXY" \
        "github.com/gentleman-programming/gentle-ai/v5" \
        "${GONOPROXY:-}" || return 1
    assert_not_contains "GONOSUMDB" "${GONOSUMDB:-}" "/v3" || return 1
    assert_not_contains "GOPRIVATE" "${GOPRIVATE:-}" "/v3" || return 1
    assert_not_contains "GONOPROXY" "${GONOPROXY:-}" "/v3" || return 1
    return 0
}

# Fail-closed: the go.mod fetch at the resolved ref returns 404. The
# installer must abort and never reach `go install`. We run install_go in
# an inner subshell because the original fatal() calls exit, which would
# terminate this test's outer subshell too.
test_go_mod_404_fail_closed() {
    INSTALL_LOG=""
    GO_WAS_CALLED=0
    EXPECTED_TAG="v3.7.0"
    RELEASES_HTTP="200"
    GOMOD_HTTP="404"
    GOMOD_NO_MODULE="0"

    curl() { mock_curl "$@"; }
    # `go` must refuse to record anything — if install_go somehow reached
    # the install call after a fatal, this is the only thing that catches it.
    go() {
        GO_WAS_CALLED=1
        return 0
    }

    CHANNEL="stable"
    LATEST_VERSION=""

    local sub_rc=0
    ( install_go ) || sub_rc=$?

    if [ "${GO_WAS_CALLED}" = "1" ]; then
        printf '         go install was called after fatal: %s\n' \
            "${INSTALL_LOG}" >&2
        return 1
    fi
    if [ "${sub_rc}" = "0" ]; then
        printf '         install_go returned 0 — expected fatal\n' >&2
        return 1
    fi
    return 0
}

# Fail-closed: HTTP 200 but the go.mod body has no `module` line. Same
# fail-closed expectation — no install, no success.
test_go_mod_missing_module_fail_closed() {
    INSTALL_LOG=""
    GO_WAS_CALLED=0
    EXPECTED_TAG="v3.7.0"
    RELEASES_HTTP="200"
    GOMOD_HTTP="200"
    GOMOD_NO_MODULE="1"

    curl() { mock_curl "$@"; }
    go() {
        GO_WAS_CALLED=1
        return 0
    }

    CHANNEL="stable"
    LATEST_VERSION=""

    local sub_rc=0
    ( install_go ) || sub_rc=$?

    if [ "${GO_WAS_CALLED}" = "1" ]; then
        printf '         go install was called after fatal: %s\n' \
            "${INSTALL_LOG}" >&2
        return 1
    fi
    if [ "${sub_rc}" = "0" ]; then
        printf '         install_go returned 0 — expected fatal\n' >&2
        return 1
    fi
    return 0
}

# Env-pattern idempotence: prepend_go_env_pattern must not duplicate an
# already-present pattern. Pre-set GONOSUMDB to the same value the
# installer is about to write and verify it stays a single copy.
test_env_pattern_idempotent() {
    GONOSUMDB="github.com/gentleman-programming/gentle-ai"
    prepend_go_env_pattern GONOSUMDB "github.com/gentleman-programming/gentle-ai"
    assert_eq "GONOSUMDB unchanged after duplicate prepend" \
        "github.com/gentleman-programming/gentle-ai" \
        "${GONOSUMDB}" || return 1

    # And a fresh prepend against a different existing value must still
    # prepend without altering the existing entry.
    GONOSUMDB="github.com/example/foo"
    prepend_go_env_pattern GONOSUMDB "github.com/gentleman-programming/gentle-ai"
    assert_contains "GONOSUMDB contains new pattern" \
        "${GONOSUMDB}" "github.com/gentleman-programming/gentle-ai" || return 1
    assert_contains "GONOSUMDB preserves prior value" \
        "${GONOSUMDB}" "github.com/example/foo" || return 1
    return 0
}

# Regression for the execution guard: `curl -sL ... | bash` reads the
# program from stdin, where BASH_SOURCE[0] is empty and $0 is "bash". The
# guard must still run main in that mode — otherwise the installer's
# documented primary invocation silently no-ops.
test_stdin_execution_runs_main() {
    local out
    if ! out="$(cat "${INSTALL_SH}" | bash -s -- --help)"; then
        printf '         stdin execution exited non-zero\n' >&2
        return 1
    fi
    assert_contains "stdin execution reaches main (--help prints usage)" \
        "${out}" "Usage: install.sh" || return 1
    return 0
}

# ---- runner -----------------------------------------------------------------
run_test "stable channel installs /v3 at the resolved tag"           test_stable_v3_current
run_test "stable channel follows /v4 when go.mod declares it"        test_stable_v4_regression
run_test "beta channel derives module + env patterns from go.mod"    test_beta_v5_derives_module_and_env
run_test "go.mod fetch 404 fails closed (no install)"                test_go_mod_404_fail_closed
run_test "go.mod body without module line fails closed (no install)" test_go_mod_missing_module_fail_closed
run_test "prepend_go_env_pattern stays idempotent on duplicates"     test_env_pattern_idempotent
run_test "stdin execution (curl | bash) still runs main"             test_stdin_execution_runs_main

printf '\n'
if [ "${TESTS_FAILED}" -eq 0 ]; then
    printf '%sok%s — %d/%d installer module-path tests passed\n' \
        "${C_GREEN}${C_BOLD}" "${C_NC}" "${TESTS_PASSED}" "${TESTS_TOTAL}"
    exit 0
fi

printf '%sFAIL%s — %d/%d installer module-path tests failed:\n' \
    "${C_RED}${C_BOLD}" "${C_NC}" "${TESTS_FAILED}" "${TESTS_TOTAL}"
for name in "${FAILED_NAMES[@]}"; do
    printf '  - %s\n' "${name}"
done
exit 1