#!/usr/bin/env bash
# docker-test.sh — Build & run E2E tests across all supported Linux platforms.
#
# Usage:
#   ./e2e/docker-test.sh                      # Tier 1 only (default)
#   RUN_FULL_E2E=1 ./e2e/docker-test.sh       # Tier 1 + 2
#   RUN_BACKUP_TESTS=1 ./e2e/docker-test.sh   # Tier 1 + 3
#   RUN_FULL_E2E=1 RUN_BACKUP_TESTS=1 ./e2e/docker-test.sh  # All tiers
#
# Exit codes:
#   0 — all platforms passed
#   1 — at least one platform failed
set -uo pipefail

# ---------------------------------------------------------------------------
# Colors (duplicated here so the orchestrator can log independently of lib.sh)
# ---------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Platform matrix: <name>:<dockerfile>
PLATFORMS=(
    "ubuntu:Dockerfile.ubuntu"
    "arch:Dockerfile.arch"
    "fedora:Dockerfile.fedora"
)

# Environment variables to forward into containers
ENV_FLAGS=""
[ "${RUN_FULL_E2E:-0}" = "1" ]    && ENV_FLAGS="$ENV_FLAGS -e RUN_FULL_E2E=1"
[ "${RUN_BACKUP_TESTS:-0}" = "1" ] && ENV_FLAGS="$ENV_FLAGS -e RUN_BACKUP_TESTS=1"

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
TOTAL=0
PASS=0
FAIL=0
FAILED_PLATFORMS=""

printf "${BLUE}[ORCH]${NC} Starting E2E tests across %d platform(s)\n" "${#PLATFORMS[@]}"
printf "${BLUE}[ORCH]${NC} Project root: %s\n" "$PROJECT_ROOT"
echo ""

for entry in "${PLATFORMS[@]}"; do
    IFS=':' read -r name dockerfile <<< "$entry"
    image_tag="gentle-ai-e2e-${name}"

    TOTAL=$((TOTAL + 1))
    printf "${YELLOW}[BUILD]${NC} %s — building from %s\n" "$name" "$dockerfile"

    if docker build \
        -f "$SCRIPT_DIR/$dockerfile" \
        -t "$image_tag" \
        "$PROJECT_ROOT" 2>&1; then
        printf "${GREEN}[BUILD]${NC} %s — image built successfully\n" "$name"
    else
        printf "${RED}[BUILD]${NC} %s — build FAILED\n" "$name"
        FAIL=$((FAIL + 1))
        FAILED_PLATFORMS="$FAILED_PLATFORMS $name"
        continue
    fi

    printf "${YELLOW}[RUN]${NC}   %s — running tests\n" "$name"

    # shellcheck disable=SC2086
    if docker run --rm $ENV_FLAGS "$image_tag" 2>&1; then
        printf "${GREEN}[RUN]${NC}   %s — PASSED\n" "$name"
        PASS=$((PASS + 1))
    else
        printf "${RED}[RUN]${NC}   %s — FAILED\n" "$name"
        FAIL=$((FAIL + 1))
        FAILED_PLATFORMS="$FAILED_PLATFORMS $name"
    fi

    echo ""
done

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo "========================================"
echo "  E2E Docker Test Summary"
echo "========================================"
printf "  ${GREEN}PASSED${NC}: %d / %d\n" "$PASS" "$TOTAL"
printf "  ${RED}FAILED${NC}: %d / %d\n" "$FAIL" "$TOTAL"

if [ -n "$FAILED_PLATFORMS" ]; then
    printf "  Failed platforms:%s\n" "$FAILED_PLATFORMS"
fi

echo "========================================"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi

exit 0
