# headroom-mcp Specification

## Purpose

Defines the install, runtime, and uninstall behavior of the Headroom context compression MCP component. Covers pip-based deployment of `headroom-ai[all]`, agent MCP injection, TUI presence, health checks, and cleanup.

## Requirements

### Requirement: Pip-based Install

The system MUST install `headroom-ai[all]` via pip/pip3 when the headroom component is selected. The installer MUST detect an existing installation before attempting install and skip if already present.

#### Scenario: Fresh install succeeds

- GIVEN pip is available and headroom-ai[all] is not installed
- WHEN the installer runs for the headroom component
- THEN pip installs `headroom-ai[all]` from PyPI

#### Scenario: Existing install is detected

- GIVEN headroom-ai[all] is already installed
- WHEN the installer runs for the headroom component
- THEN pip install is skipped and MCP injection proceeds

#### Scenario: Pip is unavailable

- GIVEN pip and pip3 are both unreachable
- WHEN the installer runs for the headroom component
- THEN installation halts with a clear error message

---

### Requirement: MCP Tool Registration

The running Headroom MCP server MUST expose three tools: `headroom_compress`, `headroom_retrieve`, `headroom_stats`.

#### Scenario: Compress compresses and retrieves

- GIVEN the headroom MCP server is running
- WHEN the client calls `headroom_compress` with text content
- THEN the server returns a compressed representation recoverable via `headroom_retrieve`

#### Scenario: Stats reports metrics

- GIVEN the headroom MCP server is running
- WHEN the client calls `headroom_stats`
- THEN the server returns compression ratio and token savings

---

### Requirement: Agent Injection

The system MUST inject headroom MCP config into Claude Code and OpenCode via the existing `mcp.Inject()` mechanism using pip-resolved binary path.

#### Scenario: Claude Code gets headroom config

- GIVEN headroom-ai[all] is installed
- WHEN `mcp.Inject` runs for Claude Code
- THEN headroom is registered in the Claude Code MCP config with the correct binary path

#### Scenario: OpenCode gets headroom config

- GIVEN headroom-ai[all] is installed
- WHEN `mcp.Inject` runs for OpenCode
- THEN headroom is registered in the OpenCode MCP config with the correct binary path

---

### Requirement: TUI Selection

The headroom component MUST be selectable in the TUI component screen. It MUST appear in the FullGentleman and EcosystemOnly presets.

#### Scenario: Headroom in FullGentleman preset

- GIVEN the user opens the TUI component selection
- WHEN the FullGentleman preset is applied
- THEN headroom is included in the enabled component list

#### Scenario: Headroom selectable individually

- GIVEN the user opens the TUI component screen
- WHEN the user browses available components
- THEN headroom appears as a selectable standalone component

---

### Requirement: Doctor Health Check

The doctor command MUST verify headroom availability by checking pip package presence and active MCP config.

#### Scenario: Healthy headroom passes

- GIVEN headroom-ai[all] is installed and MCP config is present
- WHEN `gentle-ai doctor` runs
- THEN headroom shows as pass

#### Scenario: Missing headroom fails

- GIVEN headroom-ai[all] is not installed
- WHEN `gentle-ai doctor` runs
- THEN headroom shows a clear failure message

---

### Requirement: Clean Uninstall

The uninstall command MUST remove the pip package and all headroom MCP config entries from every agent that received injection.

#### Scenario: Uninstall removes everything

- GIVEN headroom-ai[all] is installed with active MCP configs
- WHEN the uninstall command runs
- THEN `pip uninstall headroom-ai -y` executes successfully
- AND all headroom MCP config entries are removed from every agent

---

### Requirement: Backup and Rollback Compatibility

The pre-install backup snapshot MUST cover all headroom MCP config files. Rollback MUST restore the pre-install MCP state for every affected agent.

#### Scenario: Rollback restores pre-install state

- GIVEN a headroom install has been backed up via pre-install snapshot
- WHEN rollback is triggered
- THEN all headroom MCP config entries present in the backup are restored exactly
- AND no orphaned headroom config remains
