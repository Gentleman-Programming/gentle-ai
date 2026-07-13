#!/usr/bin/env bash
# scripts/validate_doctor.sh
# Integration validation for gentle-ai doctor command using Colima/Docker
# Tests the doctor command across different Linux distributions

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
BINARY_NAME="gentle-ai"
BUILD_DIR="$(dirname "$0")/../"
TEST_IMAGES=("alpine:latest" "ubuntu:latest" "fedora:latest")
TEST_TIMEOUT=120

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[FAIL]${NC} $*"; }

# Build the binary for Linux/amd64
build_binary() {
    log_info "Building ${BINARY_NAME} for linux/amd64..."
    cd "${BUILD_DIR}"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${BINARY_NAME}-linux-amd64" ./cmd/gentle-ai
    log_success "Binary built: ${BINARY_NAME}-linux-amd64"
}

# Test doctor command in a container
test_in_container() {
    local image="$1"
    local test_name="$2"
    local script="$3"
    
    log_info "Running ${test_name} in ${image}..."
    
    # Create container with the binary mounted
    container_id=$(docker run -d --rm \
        -v "$(pwd)/${BINARY_NAME}-linux-amd64:/usr/local/bin/${BINARY_NAME}:ro" \
        -w /tmp \
        "${image}" \
        sleep 300)
    
    # Run the test script in the container
    local result=0
    docker exec "${container_id}" sh -c "${script}" || result=$?
    
    # Cleanup
    docker kill "${container_id}" >/dev/null 2>&1 || true
    
    return ${result}
}

# Scenario 1: Clean install - no dependencies
test_clean_install() {
    local image="$1"
    local test_name="Clean install (no deps)"
    
    # Script that runs doctor and expects failures
    local script="
        set -e
        # First run should have failures (missing git, go, etc.)
        if ${BINARY_NAME} doctor 2>&1 | grep -q 'FAIL'; then
            echo 'FAIL detected as expected'
            exit 0
        else
            echo 'Expected FAIL status not found'
            exit 1
        fi
    "
    
    if test_in_container "${image}" "${test_name}" "${script}"; then
        log_success "${test_name} in ${image}"
        return 0
    else
        log_error "${test_name} in ${image}"
        return 1
    fi
}

# Scenario 2: Fix mode - verify correct install commands
test_fix_mode() {
    local image="$1"
    local test_name="Fix mode (OS-specific commands)"
    local expected_cmd=""
    
    case "${image}" in
        ubuntu:*)
            expected_cmd="apt-get install"
            ;;
        fedora:*)
            expected_cmd="dnf install"
            ;;
        alpine:*)
            expected_cmd="apk add"
            ;;
    esac
    
    local script="
        set -e
        # Run doctor with --fix and check for correct package manager command
        if ${BINARY_NAME} doctor --fix 2>&1 | grep -q '${expected_cmd}'; then
            echo 'Correct package manager command found: ${expected_cmd}'
            exit 0
        else
            echo 'Expected package manager command not found: ${expected_cmd}'
            ${BINARY_NAME} doctor --fix 2>&1 | head -30
            exit 1
        fi
    "
    
    if test_in_container "${image}" "${test_name}" "${script}"; then
        log_success "${test_name} in ${image}"
        return 0
    else
        log_error "${test_name} in ${image}"
        return 1
    fi
}

# Scenario 3: JSON output validation
test_json_output() {
    local image="$1"
    local test_name="JSON output validation"
    
    local script="
        set -e
        # Run doctor with --json and validate with jq
        ${BINARY_NAME} doctor --json 2>&1 | jq -e '.Summary.Fail >= 0' > /dev/null
        echo 'Valid JSON output with Summary field'
        exit 0
    "
    
    if test_in_container "${image}" "${test_name}" "${script}"; then
        log_success "${test_name} in ${image}"
        return 0
    else
        log_error "${test_name} in ${image}"
        return 1
    fi
}

# Scenario 4: Category filtering
test_category_filter() {
    local image="$1"
    local test_name="Category filtering"
    
    local script="
        set -e
        # Test hardware-only category
        ${BINARY_NAME} doctor --category hw 2>&1 | grep -q 'Hardware:'
        echo 'Hardware category filtering works'
        
        # Test software-only category
        ${BINARY_NAME} doctor --category sw 2>&1 | grep -q 'Software:'
        echo 'Software category filtering works'
        
        # Test config-only category
        ${BINARY_NAME} doctor --category cfg 2>&1 | grep -q 'Config:'
        echo 'Config category filtering works'
        
        exit 0
    "
    
    if test_in_container "${image}" "${test_name}" "${script}"; then
        log_success "${test_name} in ${image}"
        return 0
    else
        log_error "${test_name} in ${image}"
        return 1
    fi
}

# Main test runner
run_all_tests() {
    local failed=0
    local total=0
    
    log_info "Starting doctor command validation across ${#TEST_IMAGES[@]} images"
    
    for image in "${TEST_IMAGES[@]}"; do
        log_info "=== Testing ${image} ==="
        
        # Pull image if not present
        docker pull "${image}" >/dev/null 2>&1 || true
        
        # Run each scenario
        for scenario in "test_clean_install" "test_fix_mode" "test_json_output" "test_category_filter"; do
            ((total++))
            if ! ${scenario} "${image}"; then
                ((failed++))
            fi
        done
        
        log_info "=== Completed ${image} ==="
    done
    
    echo
    log_info "=== SUMMARY ==="
    log_info "Total tests: ${total}"
    log_info "Failed: ${failed}"
    
    if [ ${failed} -eq 0 ]; then
        log_success "All tests passed!"
        return 0
    else
        log_error "${failed} test(s) failed"
        return 1
    fi
}

# Cleanup function
cleanup() {
    log_info "Cleaning up..."
    rm -f "${BUILD_DIR}/${BINARY_NAME}-linux-amd64"
    # Kill any remaining test containers
    docker ps -q --filter "ancestor=alpine:latest" --filter "ancestor=ubuntu:latest" --filter "ancestor=fedora:latest" | xargs -r docker kill >/dev/null 2>&1 || true
}

# Trap cleanup on exit
trap cleanup EXIT

# Main
main() {
    log_info "gentle-ai doctor integration validation"
    log_info "========================================"
    
    # Check prerequisites
    if ! command -v docker &> /dev/null; then
        log_error "Docker not found. Please install Docker/Colima first."
        exit 1
    fi
    
    if ! command -v jq &> /dev/null; then
        log_warn "jq not found. JSON validation will be skipped."
    fi
    
    # Build binary
    build_binary
    
    # Run tests
    run_all_tests
}

main "$@"