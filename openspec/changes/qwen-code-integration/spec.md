# Qwen Code Agent Integration — Specification

## Purpose

Define the behavioral requirements for the Qwen Code agent adapter, including detection, installation, config paths, strategy assignments, permissions, SDD integration, engram setup, and test coverage.

---

## 1. Agent Identity

### Requirement: Agent ID Constant

The system MUST define `AgentQwenCode` as a constant of type `model.AgentID` with the string value `"qwen-code"`.

#### Scenario: Constant value

- GIVEN the model package is compiled
- WHEN `AgentQwenCode` is accessed
- THEN its string value equals `"qwen-code"`

### Requirement: Support Tier

The Qwen Code adapter MUST report `model.TierFull`, indicating complete ecosystem support (SDD, skills, MCP, persona, permissions, engram).

#### Scenario: Tier value

- GIVEN a Qwen Code adapter instance
- WHEN `Tier()` is called
- THEN it returns `model.TierFull`

---

## 2. Detection

### Requirement: Binary Detection

The adapter MUST detect Qwen Code's presence by searching for the `qwen` binary in PATH using `exec.LookPath`.

#### Scenario: Binary found

- GIVEN `qwen` is installed and on PATH
- WHEN `Detect()` is called
- THEN `installed` returns `true` and `binaryPath` is the resolved path

#### Scenario: Binary not found

- GIVEN `qwen` is not on PATH
- WHEN `Detect()` is called
- THEN `installed` returns `false` and `binaryPath` is empty

### Requirement: Config Directory Detection

The adapter MUST check for `~/.qwen/` directory existence and report its presence state.

#### Scenario: Config directory exists

- GIVEN `~/.qwen/` exists as a directory
- WHEN `Detect()` is called
- THEN `configFound` returns `true` and `configPath` is the absolute path

#### Scenario: Config directory missing

- GIVEN `~/.qwen/` does not exist
- WHEN `Detect()` is called
- THEN `configFound` returns `false`

#### Scenario: Stat error propagates

- GIVEN `~/.qwen/` stat returns a permission error
- WHEN `Detect()` is called
- THEN the error is returned to the caller

---

## 3. Installation

### Requirement: Auto-Install Support

The adapter MUST report `SupportsAutoInstall() = true`, indicating it can be installed via npm.

#### Scenario: Auto-install enabled

- GIVEN a Qwen Code adapter instance
- WHEN `SupportsAutoInstall()` is called
- THEN it returns `true`

### Requirement: Install Command Resolution

The adapter MUST return the correct npm install command based on platform profile:
- Linux without writable npm: `["sudo", "npm", "install", "-g", "@qwen-code/qwen-code@latest"]`
- All other platforms: `["npm", "install", "-g", "@qwen-code/qwen-code@latest"]`

#### Scenario: Darwin uses npm without sudo

- GIVEN OS is `darwin`
- WHEN `InstallCommand()` is called
- THEN it returns `["npm", "install", "-g", "@qwen-code/qwen-code@latest"]`

#### Scenario: Linux system npm uses sudo

- GIVEN OS is `linux` and `NpmWritable` is `false`
- WHEN `InstallCommand()` is called
- THEN it returns `["sudo", "npm", "install", "-g", "@qwen-code/qwen-code@latest"]`

#### Scenario: Linux nvm skips sudo

- GIVEN OS is `linux` and `NpmWritable` is `true`
- WHEN `InstallCommand()` is called
- THEN it returns `["npm", "install", "-g", "@qwen-code/qwen-code@latest"]`

#### Scenario: Windows uses npm without sudo

- GIVEN OS is `windows` and `NpmWritable` is `true`
- WHEN `InstallCommand()` is called
- THEN it returns `["npm", "install", "-g", "@qwen-code/qwen-code@latest"]`

---

## 4. Config Paths

### Requirement: Global Config Directory

`GlobalConfigDir(homeDir)` MUST return `~/.qwen`.

### Requirement: System Prompt Directory

`SystemPromptDir(homeDir)` MUST return `~/.qwen`.

### Requirement: System Prompt File

`SystemPromptFile(homeDir)` MUST return `~/.qwen/QWEN.md`.

### Requirement: Skills Directory

`SkillsDir(homeDir)` MUST return `~/.qwen/skills`.

### Requirement: Settings Path

`SettingsPath(homeDir)` MUST return `~/.qwen/settings.json`.

### Requirement: MCP Config Path

`MCPConfigPath(homeDir, serverName)` MUST return `~/.qwen/settings.json` (ignores `serverName`).

### Requirement: Commands Directory (Slash Commands)

`CommandsDir(homeDir)` MUST return `~/.qwen/commands`, supporting namespaced slash commands (e.g., `commands/sdd/init.md` → `/sdd:init`).

---

## 5. Strategy Assignments

### Requirement: System Prompt Strategy

`SystemPromptStrategy()` MUST return `model.StrategyFileReplace`. The system prompt file (`QWEN.md`) is managed via marker-based section injection, with persona content written as base and SDD/engram sections appended.

### Requirement: MCP Strategy

`MCPStrategy()` MUST return `model.StrategyMergeIntoSettings`. MCP servers are merged into `settings.json` under the `mcpServers` key via deep JSON merge.

---

## 6. Capability Flags

### Requirement: Capability Values

| Method | Returns | Rationale |
|--------|---------|-----------|
| `SupportsOutputStyles()` | `false` | Qwen CLI does not expose configurable output styles |
| `OutputStyleDir()` | `""` | No output style directory |
| `SupportsSlashCommands()` | `true` | Qwen supports custom slash commands via `~/.qwen/commands/*.md` |
| `SupportsSkills()` | `true` | Skills directory exists and is used |
| `SupportsSystemPrompt()` | `true` | QWEN.md is the system prompt file |
| `SupportsMCP()` | `true` | settings.json supports mcpServers |

---

## 7. SDD Orchestrator

### Requirement: Agent-Specific Asset

`sddOrchestratorAsset(model.AgentQwenCode)` MUST return `"qwen/sdd-orchestrator.md"`.

#### Scenario: Asset selection

- GIVEN `sddOrchestratorAsset` is called with `AgentQwenCode`
- WHEN the function returns
- THEN the string equals `"qwen/sdd-orchestrator.md"`

### Requirement: Asset Content References Qwen Paths

The Qwen SDD orchestrator asset MUST reference `~/.qwen/skills/` for skill paths (not `~/.gemini/skills/` or `~/.claude/skills/`).

#### Scenario: Path correctness

- GIVEN the Qwen orchestrator asset content
- WHEN scanned for skill path references
- THEN it contains `~/.qwen/skills/`

---

## 8. Permissions

### Requirement: Auto-Edit Mode Overlay

The permissions component MUST define `qwenCodeOverlayJSON` with `{"permissions": {"defaultMode": "auto_edit"}}`.

#### Scenario: Overlay structure

- GIVEN `qwenCodeOverlayJSON` is defined
- WHEN parsed as JSON
- THEN it contains `permissions.defaultMode = "auto_edit"`

### Requirement: Permissions Switch Case

`agentOverlay(model.AgentQwenCode)` MUST return `qwenCodeOverlayJSON`.

#### Scenario: Overlay returned

- GIVEN `agentOverlay` is called with `AgentQwenCode`
- WHEN the function returns
- THEN the bytes equal `qwenCodeOverlayJSON`

---

## 9. Engram Setup

### Requirement: Agent Slug Mapping

`SetupAgentSlug(model.AgentQwenCode)` MUST return `("qwen-code", true)`.

#### Scenario: Slug correct

- GIVEN `SetupAgentSlug` is called with `AgentQwenCode`
- WHEN the function returns
- THEN `slug` equals `"qwen-code"` and `ok` equals `true`

#### Scenario: ShouldAttemptSetup

- GIVEN `SetupModeSupported` mode and `AgentQwenCode`
- WHEN `ShouldAttemptSetup()` is called
- THEN it returns `true`

---

## 10. Config Scan

### Requirement: Known Config Directory

`knownAgentConfigDirs(homeDir)` MUST include `{Agent: "qwen-code", Path: "~/.qwen"}`.

#### Scenario: Config scan entry exists

- GIVEN `knownAgentConfigDirs` is called
- WHEN the result is scanned
- THEN an entry with `Agent = "qwen-code"` and `Path` ending in `.qwen` is present

---

## 11. CLI Validation

### Requirement: Agent Validation Case

The CLI agent validation switch MUST include a case for `"qwen-code"` that appends `model.AgentQwenCode` to the agents list.

---

## 12. TUI Agent Selection

### Requirement: TUI State Restoration

The TUI's `loadSelection()` function MUST include a case for `"qwen-code"` that appends `model.AgentQwenCode` to the selected agents list.

---

## 13. SDD Injection

### Requirement: Orchestrator Written to QWEN.md

When `sdd.Inject()` is called with a Qwen Code adapter, the SDD orchestrator content MUST be injected into `~/.qwen/QWEN.md` using `<!-- gentle-ai:sdd-orchestrator -->` markers.

#### Scenario: SDD orchestrator injection

- GIVEN a fresh home directory with no `~/.qwen/`
- WHEN `Inject(homeDir, qwenAdapter, "")` is called
- THEN `~/.qwen/QWEN.md` is created and contains `<!-- gentle-ai:sdd-orchestrator -->` markers

#### Scenario: SDD skill files written

- GIVEN a fresh home directory
- WHEN `Inject()` is called with a Qwen Code adapter
- THEN SDD skill files are written to `~/.qwen/skills/{skill}/SKILL.md`

---

## 14. Test Coverage

### Requirement: Adapter Tests

The `internal/agents/qwen/` package MUST include:
- Table-driven detection tests (binary found, binary missing, stat error)
- Table-driven install command tests (darwin, linux+sudo, linux+nvm, windows)
- Config path cross-platform tests (all path methods return correct values)
- Capability tests (all boolean flags verified)

### Requirement: Integration Tests

- `TestInjectQwenCodeWritesSDDOrchestratorAndSkills` in `internal/components/sdd/inject_test.go`
- `TestSDDOrchestratorAssetSelection` includes `AgentQwenCode` case
- Engram setup test includes `AgentQwenCode` case

### Requirement: Registry and Config Tests

- `internal/agents/registry_test.go` includes `AgentQwenCode` in default registry test
- `internal/cli/install_test.go` includes `AgentQwenCode` in default selection and agent mapping tests
- `internal/tui/model_test.go` includes `"qwen-code"` in `makeDetectionWithAgents()` known agents list
- `internal/system/config_scan_test.go` accounts for `qwen-code` in total agent count
