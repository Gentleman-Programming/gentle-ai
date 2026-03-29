# Non-Interactive Mode

Use non-interactive mode for CI, scripts, or reproducible local setup.

## Command

```bash
go run ./cmd/gentle-ai install [flags]
```

## Supported flags

- `--agent`, `--agents`: comma-separated and repeatable.
- `--component`, `--components`: comma-separated and repeatable.
- `--skill`, `--skills`: comma-separated and repeatable.
- `--persona`: explicit persona id.
- `--preset`: explicit preset id.
- `--dry-run`: render plan without executing.

## Platform behavior

The installer detects the platform automatically at runtime — there is no flag to override platform selection. The detected platform profile determines which package manager is used for install commands:

| Platform | Package manager | Example install command |
|---|---|---|
| macOS | `brew` | `brew install anomalyco/tap/opencode` |
| Ubuntu/Debian | `apt` | `sudo npm install -g opencode-ai` |
| Arch | `pacman` | `sudo npm install -g opencode-ai` |
| Fedora/RHEL family | `dnf` | `sudo npm install -g opencode-ai` |

The `--dry-run` output includes a `Platform decision` line showing `os`, `distro`, `package-manager`, and `status`.

## Examples

macOS (or any supported platform — same flags, platform is auto-detected):

```bash
go run ./cmd/gentle-ai install \
  --agent claude-code,opencode \
  --component engram,sdd,skills \
  --skill sdd-apply \
  --persona gentleman \
  --preset full-gentleman \
  --dry-run
```

The flags are identical across platforms. Only the resolved install commands change based on detection.

## Error handling

- Unknown or unsupported options fail fast with validation errors.
- Running on an unsupported platform exits immediately before any install work begins.
