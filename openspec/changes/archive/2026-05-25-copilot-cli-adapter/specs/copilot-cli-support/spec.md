# copilot-cli-support

## Purpose

Defines detection, config injection, skill loading, MCP merge, and uninstall behavior for GitHub Copilot CLI adapter support.

## Requirements

### Requirement: Copilot CLI Detection

The system MUST detect Copilot CLI as installed when the `copilot` binary is on PATH AND `~/.copilot/config.json` exists. The adapter MUST remain registered for agent listings even when detection fails.

#### Scenario: Fully installed

- GIVEN `copilot` is on PATH and `~/.copilot/config.json` exists
- WHEN detection runs
- THEN the agent is marked as installed

#### Scenario: Binary present, config missing

- GIVEN `copilot` is on PATH but `~/.copilot/config.json` does NOT exist
- WHEN detection runs
- THEN the agent is NOT marked as installed

#### Scenario: Neither binary nor config

- GIVEN `copilot` is NOT on PATH
- WHEN detection runs
- THEN the agent is NOT marked as installed
- AND the adapter still appears in agent catalog

### Requirement: System Prompt Instructions Sync

The system MUST write canonical SDD instructions to `.github/copilot-instructions.md` at workspace root. Writes MUST be atomic and idempotent: skip when content matches.

#### Scenario: First-time sync

- GIVEN `.github/copilot-instructions.md` does NOT exist
- WHEN `gga install --agent copilot-cli` runs
- THEN the canonical instructions are written to `.github/copilot-instructions.md`

#### Scenario: Idempotent re-sync

- GIVEN `.github/copilot-instructions.md` matches the canonical instructions
- WHEN `gga install --agent copilot-cli` runs again
- THEN the file is NOT overwritten

#### Scenario: Stale instructions updated

- GIVEN `.github/copilot-instructions.md` has stale content
- WHEN `gga install --agent copilot-cli` runs
- THEN the file is atomically replaced with current canonical instructions

### Requirement: Skills Directory Injection

The system MUST copy project skill files from `skills/` to `~/.copilot/skills/` using an atomic no-op pattern: skip files with identical content.

#### Scenario: First-time injection

- GIVEN `skills/` contains project skill files and `~/.copilot/skills/` is empty
- WHEN `gga install --agent copilot-cli` runs
- THEN all skill files are copied to `~/.copilot/skills/`

#### Scenario: Partial update

- GIVEN `~/.copilot/skills/` has a stale skill and a new skill exists in `skills/`
- WHEN `gga install --agent copilot-cli` runs
- THEN the stale skill is replaced and the new skill is added
- AND unchanged skills are NOT overwritten

### Requirement: MCP Configuration Merge

The system MUST merge Gentleman AI MCP entries (Context7, Engram) into `~/.copilot/mcp-config.json`. Existing non-Gentleman entries MUST be preserved. The merge MUST skip writes when config is unchanged.

#### Scenario: Merge with existing user entries

- GIVEN `~/.copilot/mcp-config.json` contains user-defined MCP servers
- WHEN `gga install --agent copilot-cli` runs
- THEN Context7 and Engram entries are added
- AND existing user entries remain intact

#### Scenario: Idempotent merge

- GIVEN `~/.copilot/mcp-config.json` already has canonical Gentleman MCP entries
- WHEN `gga install --agent copilot-cli` runs
- THEN the file is NOT rewritten

### Requirement: Uninstall Cleanup

The system MUST, during `gga uninstall --agent copilot-cli`, remove all Gentleman-injected artifacts: `.github/copilot-instructions.md`, Gentleman-injected skill files from `~/.copilot/skills/`, and Gentleman MCP entries from `~/.copilot/mcp-config.json`. Non-Gentleman content in shared files MUST remain intact.

#### Scenario: Full uninstall

- GIVEN Gentleman AI has injected config, skills, and instructions for Copilot CLI
- WHEN `gga uninstall --agent copilot-cli` runs
- THEN `.github/copilot-instructions.md` is removed
- AND Gentleman skill files are removed from `~/.copilot/skills/`
- AND Context7/Engram MCP entries are removed from `~/.copilot/mcp-config.json`
- AND user-defined skills and MCP entries remain

#### Scenario: Uninstall with nothing injected

- GIVEN the adapter is registered but no injection has occurred
- WHEN `gga uninstall --agent copilot-cli` runs
- THEN no files are removed and the command exits successfully
