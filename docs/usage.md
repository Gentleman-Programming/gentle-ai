# Usage

← [Back to README](../README.md)

---

## Persona Modes

| Persona | ID | Description |
|---------|-----|-------------|
| Gentleman | `gentleman` | Teaching-oriented mentor persona — pushes back on bad practices, explains the why |
| Neutral | `neutral` | Same teacher, same philosophy, no regional language — warm and professional |
| Custom | `custom` | Bring your own persona instructions |

---

## Interactive TUI

Just run it — the Bubbletea TUI guides you through agent selection, components, skills, and presets:

```bash
gentle-ai
```

---

## CLI Commands

### install

First-time setup — detects your tools, configures agents, injects all components:

```bash
# Full ecosystem for multiple agents
gentle-ai install \
  --agent claude-code,opencode,gemini-cli \
  --preset full-gentleman

# Minimal setup for Cursor
gentle-ai install \
  --agent cursor \
  --preset minimal

# Pick specific components and skills
gentle-ai install \
  --agent claude-code \
  --component engram,sdd,skills,context7,persona,permissions \
  --skill go-testing,skill-creator,branch-pr,issue-creation \
  --persona gentleman

# Dry-run first (preview plan without applying changes)
gentle-ai install --dry-run \
  --agent claude-code,opencode \
  --preset full-gentleman
```

### sync

Refresh managed assets to the current version. Use after `brew upgrade gentle-ai` or when you want your local configs aligned with the latest release. Does NOT reinstall binaries (engram, GGA) — only updates prompt content, skills, MCP configs, and SDD orchestrators.

```bash
# Sync all installed agents
gentle-ai sync

# Sync specific agents only
gentle-ai sync --agent cursor --agent windsurf

# Sync a specific component
gentle-ai sync --component sdd
gentle-ai sync --component skills
gentle-ai sync --component engram
```

Sync is safe and idempotent — running it twice produces no changes the second time.

### update / upgrade

Check for and install new versions of `gentle-ai` itself:

```bash
# Check if a newer version is available
gentle-ai update

# Upgrade to the latest release (downloads new binary, replaces current)
gentle-ai upgrade
```

After upgrading, run `gentle-ai sync` to refresh all managed assets to the new version's content.

### version

```bash
gentle-ai version
gentle-ai --version
gentle-ai -v
```

---

## CLI Flags (install)

| Flag | Description |
|------|-------------|
| `--agent`, `--agents` | Agents to configure (comma-separated) |
| `--component`, `--components` | Components to install (comma-separated) |
| `--skill`, `--skills` | Skills to install (comma-separated) |
| `--persona` | Persona mode: `gentleman`, `neutral`, `custom` |
| `--preset` | Preset: `full-gentleman`, `ecosystem-only`, `minimal`, `custom` |
| `--dry-run` | Preview the install plan without applying changes |

## CLI Flags (sync)

| Flag | Description |
|------|-------------|
| `--agent`, `--agents` | Agents to sync (defaults to all installed agents) |
| `--component` | Sync a specific component only: `sdd`, `engram`, `context7`, `skills`, `gga`, `permissions`, `theme` |
| `--profile` | Create or update an SDD profile: `name:provider/model` (sets the default model for all phases) |
| `--profile-phase` | Override a specific phase in a profile: `name:phase:provider/model` |
| `--include-permissions` | Include permissions sync (opt-in) |
| `--include-theme` | Include theme sync (opt-in) |

**Profile examples:**

```bash
# Create a "cheap" profile using a free model for all phases
gentle-ai sync --profile cheap:openrouter/qwen/qwen3-30b-a3b:free

# Override the design phase to use a stronger model
gentle-ai sync --profile-phase cheap:sdd-design:anthropic/claude-sonnet-4-20250514

# Create multiple profiles in one command
gentle-ai sync \
  --profile cheap:openrouter/qwen/qwen3-30b-a3b:free \
  --profile premium:anthropic/claude-sonnet-4-20250514
```

See [OpenCode SDD Profiles](opencode-profiles.md) for the full guide.

---

## Typical Workflow

```bash
# First time: install everything
brew install gentleman-programming/tap/gentle-ai
gentle-ai install --agent claude-code,cursor --preset full-gentleman

# After a new release: upgrade + sync
brew upgrade gentle-ai
gentle-ai sync

# Adding a new agent later
gentle-ai install --agent windsurf --preset full-gentleman
```

---

## Dependency Management

`gentle-ai` auto-detects prerequisites before installation and provides platform-specific guidance:

- **Detected tools**: git, curl, node, npm, brew, go
- **Version checks**: validates minimum versions where applicable
- **Platform-aware hints**: suggests `brew install`, `apt install`, `pacman -S`, `dnf install`, or `winget install` depending on your OS
- **Node LTS alignment**: on apt/dnf systems, Node.js hints use NodeSource LTS bootstrap before package install
- **Dependency-first approach**: detects what's installed, calculates what's needed, shows the full dependency tree before installing anything, then verifies each dependency after installation
