# Docker E2E Testing

End-to-end tests that validate the `gentle-ai` installer binary inside Docker containers running real Linux distributions.

## Architecture

```
e2e/
  lib.sh              # Shared helpers: colors, counters, logging, cleanup
  e2e_test.sh         # All test cases, tiered by env vars
  Dockerfile.ubuntu   # Ubuntu 22.04 test image
  Dockerfile.arch     # Arch Linux test image
  Dockerfile.fedora   # Fedora test image
  Dockerfile.opensuse # openSUSE Tumbleweed test image
  docker-test.sh      # Orchestrator: build + run all platforms
```

## Quick start

```bash
# Run Tier 1 only (basic binary + dry-run tests)
./e2e/docker-test.sh

# Run all tiers
RUN_FULL_E2E=1 RUN_BACKUP_TESTS=1 ./e2e/docker-test.sh
```

## Test tiers

| Tier | Env var | What it tests |
|------|---------|---------------|
| 1 (default) | — | Binary exists, runs, dry-run output format, flag validation |
| 2 | `RUN_FULL_E2E=1` | Full install: opencode+permissions, claude-code+persona, context7, sdd |
| 3 | `RUN_BACKUP_TESTS=1` | Backup snapshot creation, backup file contents |

## Supported platforms

| Platform | Dockerfile | Package manager |
|----------|-----------|-----------------|
| Ubuntu 22.04 | `Dockerfile.ubuntu` | apt |
| Arch Linux | `Dockerfile.arch` | pacman |
| Fedora | `Dockerfile.fedora` | dnf |
| openSUSE Tumbleweed | `Dockerfile.opensuse` | zypper |

## How it works

1. **docker-test.sh** iterates over the platform matrix
2. For each platform, it builds a Docker image that:
   - Installs system dependencies (git, curl, sudo, Go)
   - Creates a non-root `testuser` with passwordless sudo
   - Copies the project source and builds the binary (`go build`)
   - Copies the E2E test scripts
3. Runs the container, which executes `e2e_test.sh` as `testuser`
4. Collects pass/fail per platform and exits non-zero if any failed

## Running individual platforms

```bash
# Build and run Ubuntu only
docker build -f e2e/Dockerfile.ubuntu -t gentle-ai-e2e-ubuntu .
docker run --rm gentle-ai-e2e-ubuntu

# Run with full E2E on Arch
docker build -f e2e/Dockerfile.arch -t gentle-ai-e2e-arch .
docker run --rm -e RUN_FULL_E2E=1 gentle-ai-e2e-arch

# Run openSUSE only
docker build -f e2e/Dockerfile.opensuse -t gentle-ai-e2e-opensuse .
docker run --rm gentle-ai-e2e-opensuse

# Interactive debugging
docker run --rm -it gentle-ai-e2e-ubuntu /bin/bash
```

## Adding a new platform

1. Create `e2e/Dockerfile.<platform>` following the existing pattern
2. Add the entry to `PLATFORMS` array in `docker-test.sh`
3. Ensure the Dockerfile creates `testuser` with NOPASSWD sudo
4. Build the `gentle-ai` binary for `linux/amd64`

## Adding new test cases

1. Add a `test_*` function to `e2e_test.sh`
2. Use `log_test`, `log_pass`, `log_fail` from `lib.sh`
3. Call `cleanup_test_env` before tests that write to filesystem
4. Place the function call under the appropriate tier section
5. Gate Tier 2/3 tests behind `RUN_FULL_E2E` / `RUN_BACKUP_TESTS`

## CI integration

```yaml
# GitHub Actions example
jobs:
  e2e-linux:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run E2E tests (Tier 1)
        run: ./e2e/docker-test.sh
      - name: Run full E2E tests
        if: github.ref == 'refs/heads/main'
        run: RUN_FULL_E2E=1 RUN_BACKUP_TESTS=1 ./e2e/docker-test.sh
```
