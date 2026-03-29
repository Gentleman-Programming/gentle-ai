# RTK (Rust Token Killer) Specification

## Purpose

Defines the install, configuration, and runtime behavior of the RTK ecosystem component — a CLI proxy that reduces LLM token consumption by 60-90% on common shell commands.

## Requirements

### Requirement: RTK Binary Installation

The system MUST install the RTK binary to the system PATH using the platform's preferred method.

#### Scenario: Install via Homebrew (macOS/Linux)

- GIVEN Homebrew is installed on the system
- WHEN the user selects RTK as a component
- THEN the system runs `brew install rtk`
- AND verifies the binary with `rtk --version`

#### Scenario: Install via curl (fallback)

- GIVEN Homebrew is NOT installed
- WHEN the user selects RTK as a component
- THEN the system downloads the latest release from GitHub Releases for the detected OS/arch
- AND places the binary in `~/.local/bin` or a system-wide location

#### Scenario: RTK already installed

- GIVEN RTK is already installed on the system
- WHEN the user selects RTK as a component
- THEN the system detects the existing installation via `rtk --version`
- AND skips reinstallation
- AND proceeds to hook configuration

---

### Requirement: Agent Hook Configuration

The system MUST configure RTK hooks for each selected agent, using RTK's native `rtk init` commands.

#### Scenario: Claude Code hook

- GIVEN RTK binary is installed and the user selected Claude Code
- WHEN the system configures RTK for Claude Code
- THEN it runs `rtk init -g` which installs the PreToolUse hook and RTK.md
- AND Claude Code transparently rewrites Bash commands to RTK equivalents

#### Scenario: OpenCode hook

- GIVEN RTK binary is installed and the user selected OpenCode
- WHEN the system configures RTK for OpenCode
- THEN it runs `rtk init -g --opencode` which creates the RTK plugin at `~/.config/opencode/plugins/rtk.ts`

#### Scenario: Cursor hook

- GIVEN RTK binary is installed and the user selected Cursor
- WHEN the system configures RTK for Cursor
- THEN it runs `rtk init -g --agent cursor` which patches `~/.cursor/hooks.json`

#### Scenario: Gemini CLI hook

- GIVEN RTK binary is installed and the user selected Gemini CLI
- WHEN the system configures RTK for Gemini CLI
- THEN it runs `rtk init -g --gemini` which patches `~/.gemini/settings.json`

#### Scenario: Multiple agents selected

- GIVEN RTK binary is installed and the user selected Claude Code + OpenCode + Cursor
- WHEN the system configures RTK
- THEN it runs `rtk init -g` for each agent sequentially
- AND each agent's hook is independently functional

---

### Requirement: Health Verification

The system MUST verify RTK integration after configuration completes.

#### Scenario: Binary health check

- GIVEN RTK installation completed
- WHEN the system runs `rtk --version`
- THEN the output MUST contain a valid version string (e.g., `rtk 0.34.1`)

#### Scenario: Hook health check

- GIVEN RTK hooks are configured for Claude Code
- WHEN the system runs `rtk init --show`
- THEN it reports the hook status as installed and active

#### Scenario: Failure during hook installation

- GIVEN RTK binary is installed
- WHEN `rtk init -g` fails for a specific agent
- THEN the system logs a warning with the agent name
- AND continues configuring remaining agents
- AND does NOT abort the entire installation

---

### Requirement: Preset Integration

RTK MUST be included in ecosystem presets as a configurable component.

#### Scenario: Full Gentleman preset

- GIVEN the user selects the "Full Gentleman" preset
- WHEN the system shows the component summary
- THEN RTK appears as included with all selected agents

#### Scenario: Ecosystem Only preset

- GIVEN the user selects the "Ecosystem Only" preset
- WHEN the system shows the component summary
- THEN RTK appears as an optional component (not forced)

#### Scenario: Custom preset

- GIVEN the user selects the "Custom" preset
- WHEN the system shows component selection
- THEN RTK appears as a selectable checkbox with description and estimated savings

---

### Requirement: Uninstall

The system MUST cleanly remove RTK hooks without affecting agent configurations.

#### Scenario: Uninstall hooks

- GIVEN RTK is configured for Claude Code
- WHEN the user runs `rtk init -g --uninstall`
- THEN the PreToolUse hook is removed from Claude Code settings
- AND `RTK.md` is deleted
- AND Claude Code continues to function normally without RTK

#### Scenario: Uninstall binary

- GIVEN RTK is installed via Homebrew
- WHEN the user runs `brew uninstall rtk`
- THEN the binary is removed from PATH
- AND all agent configs remain intact (hooks may report missing binary gracefully)
