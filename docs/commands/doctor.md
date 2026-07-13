# gentle-ai doctor

Run ecosystem health diagnostics for your AI development environment.

## Overview

The `doctor` command performs comprehensive health checks across three categories:
- **Hardware** — CPU cores, RAM, disk space, GPU/drivers, thermal throttling
- **Software** — Required binaries (git, go), optional tools (docker, kubectl), environment variables, Engram reachability
- **Config** — YAML/JSON config validation, SSH keys, Git identity, agent config directories

## Usage

```bash
gentle-ai doctor [flags]
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--json` | `-j` | Output results as JSON (for CI/CD pipelines) |
| `--fix` | `-f` | Show OS-specific remediation commands for failed checks |
| `--verbose` | `-v` | Show passing checks (not just failures) |
| `--category` | `-c` | Comma-separated categories: `hw`, `sw`, `cfg` (default: all) |
| `--config-path` | | Additional config file paths to validate |
| `--help` | `-h` | Show help message |

## Categories

| Code | Name | Checks |
|------|------|--------|
| `hw` | Hardware | CPU cores, RAM, disk space, GPU detection, thermal |
| `sw` | Software | Required/optional binaries, env vars, Engram, versions |
| `cfg` | Config | Global config, state file, project configs, SSH, Git, agent dirs |

## Output Formats

### TUI (Default, Interactive)
Interactive table with color-coded status badges:
```
gentle-ai doctor — system health check

  Hardware:
  ✓  hardware:cpu:cores                     CPU cores sufficient
  ⚠  hardware:memory:ram                    Available RAM: 2.1 GB (recommended: 4 GB+)

  Software:
  ✓  software:binaries:git                  Git found at /usr/bin/git
  ✗  software:binaries:docker                Docker NOT FOUND

Summary: 15 passed, 2 failed, 3 warnings, 1 info, 0 skipped
Duration: 245ms  Status: degraded
```

Navigation: `↑/↓` scroll, `v` toggle verbose, `q/esc` quit

### Text (Non-TTY / CI)
Plain colored table output when not in a TTY:
```
gentle-ai doctor — system health check

  Hardware:
  ✓  hardware:cpu:cores          CPU cores sufficient
  ⚠  hardware:memory:ram         Available RAM: 2.1 GB

  Software:
  ✓  software:binaries:git       Git found at /usr/bin/git
  ✗  software:binaries:docker    Docker NOT FOUND
       fix: brew install docker   (requires sudo)

Summary: 15 passed, 2 failed, 3 warnings
```

### JSON (CI/CD)
```bash
gentle-ai doctor --json
```
```json
{
  "GeneratedAt": "2026-07-13T03:14:54-05:00",
  "Version": "1.39.5",
  "GOOS": "darwin",
  "GOARCH": "arm64",
  "Results": [
    {
      "Name": "hardware:cpu:cores",
      "Category": "hardware",
      "Status": "pass",
      "Summary": "CPU cores sufficient",
      "Detail": "Found 8 CPU cores (minimum 4 recommended)",
      "Remediation": null,
      "Duration": 1234567,
      "Metadata": {}
    },
    {
      "Name": "software:binaries:docker",
      "Category": "software",
      "Status": "fail",
      "Summary": "Docker NOT FOUND",
      "Detail": "Required for container-based agents",
      "Remediation": {
        "Description": "Install Docker",
        "Commands": {
          "darwin": "brew install --cask docker",
          "linux": "curl -fsSL https://get.docker.com | sh",
          "windows": "winget install Docker.DockerDesktop"
        },
        "ManualSteps": [],
        "Links": ["https://docs.docker.com/get-docker/"]
      },
      "Duration": 567890,
      "Metadata": {}
    }
  ],
  "Summary": {
    "Total": 20,
    "Pass": 15,
    "Warn": 3,
    "Fail": 2,
    "Info": 0,
    "Skip": 0,
    "Duration": 245000000
  }
}
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All checks passed (healthy) |
| `1` | One or more checks failed (unhealthy) |

## Examples

```bash
# Full health check with TUI
gentle-ai doctor

# JSON for CI pipeline
gentle-ai doctor --json | jq '.Summary.Fail'

# Show fix commands for failures
gentle-ai doctor --fix

# Only check hardware and software
gentle-ai doctor --category hw,sw

# Verbose: show all passing checks too
gentle-ai doctor -v
```

## Fix Mode

When `--fix` is enabled, failed and warning checks include OS-specific remediation commands:

**macOS (Darwin):**
```bash
brew install docker
```

**Linux (Debian/Ubuntu):**
```bash
sudo apt-get update && sudo apt-get install -y docker.io
```

**Linux (Fedora/RHEL):**
```bash
sudo dnf install -y docker
```

**Linux (Arch):**
```bash
sudo pacman -Sy docker
```

**Windows:**
```powershell
winget install Docker.DockerDesktop
```

## Categories Detail

### Hardware (`hw`)
| Check | Description | Thresholds |
|-------|-------------|------------|
| `hardware:cpu:cores` | CPU core count | PASS ≥ 4, WARN 2-3, FAIL < 2 |
| `hardware:memory:ram` | Total RAM | PASS ≥ 8GB, WARN 4-8GB, FAIL < 4GB |
| `hardware:memory:available` | Available RAM | PASS ≥ 4GB, WARN 2-4GB, FAIL < 2GB |
| `hardware:disk:space` | Disk space (~/.config/gentle-ai) | PASS ≥ 2GB, WARN 1-2GB, FAIL < 1GB |
| `hardware:gpu:vendor` | GPU detection | INFO: NVIDIA/AMD/Intel if found |
| `hardware:thermal:throttling` | CPU thermal throttling | PASS/WARN based on throttling state |

### Software (`sw`)
| Check | Description | Thresholds |
|-------|-------------|------------|
| `software:binaries:git` | Git in PATH | FAIL if missing |
| `software:binaries:go` | Go in PATH | FAIL if missing |
| `software:binaries:docker` | Docker in PATH | WARN if missing |
| `software:binaries:kubectl` | kubectl in PATH | WARN if missing |
| `software:binaries:node` | Node.js in PATH | WARN if missing |
| `software:env:vars` | HOME, PATH, SHELL | FAIL if unset |
| `software:engram:reachable` | Engram health endpoint | WARN if unreachable |
| `software:gentle-ai-tools` | gentle-ai, engram, gga | WARN if missing |
| `software:versions:git` | Git ≥ 2.30.0 | WARN if older |
| `software:versions:go` | Go ≥ 1.21.0 | WARN if older |

### Config (`cfg`)
| Check | Description | Thresholds |
|-------|-------------|------------|
| `config:global` | ~/.config/gentle-ai/config.yaml | WARN if missing/invalid |
| `config:state` | ~/.config/gentle-ai/state.json | WARN if missing, FAIL if invalid JSON |
| `config:project` | gentle-ai.yaml / .gentle-ai.yaml | INFO per project found |
| `config:ssh:keys` | ~/.ssh/id_* keys | WARN if none found |
| `config:git:config` | ~/.gitconfig user.name/email | WARN if missing |
| `config:agent:dirs` | Agent config directories | INFO per agent (.claude, .config/opencode, etc.) |

## CI/CD Integration

```yaml
# .github/workflows/health-check.yml
name: Environment Health Check
on: [push, pull_request]
jobs:
  doctor:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install gentle-ai
        run: go install github.com/gentleman-programming/gentle-ai/cmd/gentle-ai@latest
      - name: Run doctor
        run: gentle-ai doctor --json | tee doctor-report.json
      - name: Check for failures
        run: |
          FAIL_COUNT=$(jq '.Summary.Fail' doctor-report.json)
          if [ "$FAIL_COUNT" -gt 0 ]; then
            echo "::error::Health check failed with $FAIL_COUNT failures"
            jq '.Results[] | select(.Status=="fail") | {name: .Name, summary: .Summary, fix: .Remediation}' doctor-report.json
            exit 1
          fi
```

## Troubleshooting

### "Binary not found" errors
Ensure required tools are in your PATH:
```bash
export PATH=$PATH:/usr/local/bin:$HOME/.local/bin
```

### Engram unreachable
Start Engram or configure as MCP server:
```bash
engram serve  # or configure in your MCP client
```

### Low disk space
Clean caches:
```bash
# macOS
rm -rf ~/Library/Caches/*

# Linux
rm -rf ~/.cache/*
journalctl --vacuum-time=7d
```

## Extending Checks

The doctor command uses a plugin architecture. To add custom checks:

1. Implement the `Checker` interface in `pkg/doctor/checker/`
2. Register in `internal/app/doctor_flags.go::createDoctorCheckers()`
3. Add OS-specific fixes in `pkg/doctor/fixer/fixer.go`

## Performance

Typical run time: **200-500ms**
- Hardware checks: ~50ms (gopsutil)
- Software checks: ~100ms (exec.LookPath)
- Config checks: ~50ms (file I/O)

Run with `--category` to limit scope for faster execution.

## Version

Added in gentle-ai v1.40.0