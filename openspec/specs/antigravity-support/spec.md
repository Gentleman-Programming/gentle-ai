# Antigravity support

Defines the required behavior for supporting Google Antigravity through the public `antigravity` agent ID.

## Requirements

### Requirement: Unified public agent ID

The system MUST expose Antigravity as `antigravity` and MUST NOT expose a separate public `antigravity-cli` agent option.

#### Scenario: Install uses the unified Antigravity agent

- GIVEN the installer is invoked for `antigravity`
- WHEN agent validation runs
- THEN `antigravity` is accepted
- AND the separate `antigravity-cli` option is not listed in the catalog or TUI.

### Requirement: Antigravity writes to the supported config surface

The system MUST write Antigravity settings and skills under the resolved variant directory (IDE → Desktop → CLI fallback), MCP config to the shared global location, and the Engram plugin under `~/.gemini/config/plugins/` when migrated or `<variantDir>/plugins/` otherwise.

#### Scenario: Antigravity files are installed

- GIVEN the installer runs for `antigravity`
- WHEN SDD, Engram, or permission components are applied
- THEN settings are initialized at `<variantDir>/settings.json`
- AND MCP config is merged at `~/.gemini/config/mcp_config.json`
- AND skills are installed under `<variantDir>/skills/`
- AND Engram plugin is installed under `~/.gemini/config/plugins/gentle-ai-engram/` when `.migrated` is present, or `<variantDir>/plugins/gentle-ai-engram/` otherwise
- WHERE `<variantDir>` is the first existing directory among `~/.gemini/antigravity-ide/`, `~/.gemini/antigravity/` (Desktop), or `~/.gemini/antigravity-cli/` (fallback).

### Requirement: Antigravity uses dynamic subagents

The Antigravity orchestrator MUST use runtime dynamic subagent tools rather than static subagent files.

#### Scenario: SDD orchestration runs in Antigravity

- GIVEN the Antigravity SDD orchestrator is installed
- WHEN an SDD phase requires a subagent
- THEN the prompt instructs Antigravity to call `define_subagent`
- AND then call `invoke_subagent`.

### Requirement: Antigravity shares the Gemini global prompt surface

The system MUST write global prompt/persona content for Antigravity to `~/.gemini/GEMINI.md`.

#### Scenario: Antigravity and Gemini CLI are selected together

- GIVEN both `gemini-cli` and `antigravity` are selected
- WHEN the installer applies SDD prompt content
- THEN the installer warns that both agents share `~/.gemini/GEMINI.md`.
