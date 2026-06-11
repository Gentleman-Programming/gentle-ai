# Hermes Agent Support — Specification

## Purpose

Define the behavioral requirements for the Hermes (Nous Research) agent adapter, including detection, installation (detect-only), config paths, YAML MCP strategy, SOUL.md instruction injection, persona handling, SDD integration, engram wiring, and test coverage.

All requirements in this document describe what MUST be true after the change is applied. Implementation choices belong in `design.md`.

---

## 1. Agent Identity

### Requirement: Agent ID Constant

The system MUST define `AgentHermes` as a constant of type `model.AgentID` with the string value `"hermes"`.

#### Scenario: Constant value

- GIVEN the model package is compiled
- WHEN `AgentHermes` is accessed
- THEN its string value equals `"hermes"`

### Requirement: Support Tier

The Hermes adapter MUST report `model.TierFull`, indicating complete ecosystem support (detect/validate/TUI/configure: MCP, SDD, persona, engram).

#### Scenario: Tier value

- GIVEN a Hermes adapter instance
- WHEN `Tier()` is called
- THEN it returns `model.TierFull`

---

## 2. Detection

### Requirement: Binary Detection

The adapter MUST detect Hermes by searching for the `hermes` binary in PATH using `exec.LookPath`.

#### Scenario: Binary found

- GIVEN `hermes` is installed and on PATH
- WHEN `Detect()` is called
- THEN `installed` returns `true` and `binaryPath` is the resolved absolute path

#### Scenario: Binary not found

- GIVEN `hermes` is not on PATH
- WHEN `Detect()` is called
- THEN `installed` returns `false` and `binaryPath` is empty

### Requirement: Config Directory Detection

The adapter MUST check for `~/.hermes/` directory existence and report its presence.

#### Scenario: Config directory exists

- GIVEN `~/.hermes/` exists as a directory
- WHEN `Detect()` is called
- THEN `configFound` returns `true` and `configPath` is the absolute path to `~/.hermes/`

#### Scenario: Config directory missing

- GIVEN `~/.hermes/` does not exist
- WHEN `Detect()` is called
- THEN `configFound` returns `false`

#### Scenario: Stat error propagates

- GIVEN `~/.hermes/` stat returns a permission error
- WHEN `Detect()` is called
- THEN the error is returned to the caller

---

## 3. Installation (Detect-Only)

### Requirement: Auto-Install Not Supported

The adapter MUST report `SupportsAutoInstall() = false`. Hermes is detect-only; gentle-ai will never attempt to install it automatically.

#### Scenario: Auto-install disabled

- GIVEN a Hermes adapter instance
- WHEN `SupportsAutoInstall()` is called
- THEN it returns `false`

### Requirement: Install Command Returns Not-Installable Error

`InstallCommand()` MUST return an error indicating Hermes is not auto-installable, with a message directing the user to install Hermes manually. It MUST NOT return a command slice that gentle-ai will execute.

#### Scenario: Install command rejected

- GIVEN a Hermes adapter instance
- WHEN `InstallCommand()` is called
- THEN it returns a non-nil error and an empty or nil command slice

---

## 4. Config Paths

### Requirement: Global Config Directory

`GlobalConfigDir(homeDir)` MUST return `~/.hermes`.

### Requirement: System Prompt File

`SystemPromptFile(homeDir)` MUST return `~/.hermes/SOUL.md`.

### Requirement: Skills Directory

`SkillsDir(homeDir)` MUST return `~/.hermes/skills`.

### Requirement: MCP Config Path

`MCPConfigPath(homeDir, serverName)` MUST return `~/.hermes/config.yaml` (ignores `serverName`).

### Requirement: Settings Path

`SettingsPath(homeDir)` MUST return `~/.hermes/config.yaml`.

---

## 5. Strategy Assignments

### Requirement: System Prompt Strategy

`SystemPromptStrategy()` MUST return `model.StrategyMarkdownSections`. SOUL.md content is managed via `<!-- gentle-ai:... -->` marker-based section injection without replacing user content.

#### Scenario: System prompt strategy

- GIVEN a Hermes adapter instance
- WHEN `SystemPromptStrategy()` is called
- THEN it returns `model.StrategyMarkdownSections`

### Requirement: MCP Strategy

`MCPStrategy()` MUST return `model.StrategyMergeIntoYAML`. MCP servers are merged into `~/.hermes/config.yaml` under the top-level `mcp_servers:` key using comment-preserving YAML helpers.

#### Scenario: MCP strategy

- GIVEN a Hermes adapter instance
- WHEN `MCPStrategy()` is called
- THEN it returns `model.StrategyMergeIntoYAML`

### Requirement: New MCPStrategy Constant

`model.StrategyMergeIntoYAML` MUST be defined as `MCPStrategy = 4`, joining the existing strategy enum (`StrategySeparateMCPFiles=0`, `StrategyMergeIntoSettings=1`, `StrategyMCPConfigFile=2`, `StrategyTOMLFile=3`). No existing constant values may be changed.

#### Scenario: Constant value

- GIVEN the model package is compiled
- WHEN `StrategyMergeIntoYAML` is accessed
- THEN its integer value equals `4`

---

## 6. Capability Flags

### Requirement: Capability Values

| Method | Returns | Rationale |
|--------|---------|-----------|
| `SupportsOutputStyles()` | `false` | Hermes does not expose configurable output styles |
| `OutputStyleDir()` | `""` | No output style directory |
| `SupportsSlashCommands()` | `false` | Hermes does not support custom slash commands |
| `SupportsSkills()` | `true` | `~/.hermes/skills/` is Hermes's native skill directory |
| `SupportsSystemPrompt()` | `true` | SOUL.md is the system prompt (slot #1) |
| `SupportsMCP()` | `true` | config.yaml supports `mcp_servers` |

---

## 7. YAML MCP Config Merge

### Requirement: YAML Upsert — Insert When Key Absent

When `~/.hermes/config.yaml` does not contain a `mcp_servers:` key, the YAML merge helper MUST create the key and insert the new MCP server block beneath it.

#### Scenario: First-run insert

- GIVEN `~/.hermes/config.yaml` exists with user content but no `mcp_servers:` key
- WHEN a YAML upsert is performed for server `context7`
- THEN `~/.hermes/config.yaml` contains a `mcp_servers:` key with the `context7` block beneath it
- AND all pre-existing user content is preserved verbatim

#### Scenario: File absent — created on first run

- GIVEN `~/.hermes/config.yaml` does not exist
- WHEN a YAML upsert is performed for server `context7`
- THEN the file is created and contains `mcp_servers:` with the `context7` block

### Requirement: YAML Upsert — Idempotent Re-run

When a YAML upsert is performed for a server that already exists in `mcp_servers:`, the helper MUST NOT duplicate the entry.

#### Scenario: Idempotent re-run

- GIVEN `~/.hermes/config.yaml` already contains `context7` under `mcp_servers:`
- WHEN the same YAML upsert is performed again
- THEN `~/.hermes/config.yaml` contains exactly one `context7` entry under `mcp_servers:`

### Requirement: YAML Upsert — Preserve User Comments

The YAML merge helper MUST preserve comments and content outside the managed `mcp_servers:` block.

#### Scenario: Comments outside managed block preserved

- GIVEN `~/.hermes/config.yaml` contains user comments above and below `mcp_servers:`
- WHEN a YAML upsert is performed
- THEN comments and content outside the managed block appear unchanged in the output

### Requirement: YAML Indentation

All YAML emitted by the merge helper MUST use consistent 2-space indentation. Entries under `mcp_servers:` MUST be indented by 2 spaces.

---

## 8. MCP Injection — context7

### Requirement: context7 Server Injected

When gentle-ai configures Hermes, the `context7` MCP server block MUST be written to `~/.hermes/config.yaml` under `mcp_servers:`.

#### Scenario: context7 injection

- GIVEN a fresh `~/.hermes/` directory
- WHEN `mcp.Inject()` is called with the Hermes adapter and context7 enabled
- THEN `~/.hermes/config.yaml` contains a `context7` entry under `mcp_servers:`

#### Scenario: context7 injection idempotent

- GIVEN `~/.hermes/config.yaml` already contains `context7` under `mcp_servers:`
- WHEN `mcp.Inject()` is called again
- THEN the file contains exactly one `context7` entry under `mcp_servers:`

---

## 9. MCP Injection — Engram

### Requirement: Engram Server Injected

When gentle-ai configures Hermes with engram enabled, the `engram` MCP server block MUST be written to `~/.hermes/config.yaml` under `mcp_servers:`.

#### Scenario: Engram injection

- GIVEN a fresh `~/.hermes/` directory
- WHEN `engram.Inject()` (or `mcp.Inject()`) is called with the Hermes adapter
- THEN `~/.hermes/config.yaml` contains an `engram` entry under `mcp_servers:`

#### Scenario: Engram injection idempotent

- GIVEN `~/.hermes/config.yaml` already contains `engram` under `mcp_servers:`
- WHEN injection is called again
- THEN the file contains exactly one `engram` entry under `mcp_servers:`

### Requirement: Hermes in isStandardAgent

The `engram.isStandardAgent()` helper (or its equivalent) MUST return `true` for `AgentHermes`, enabling the standard YAML injection path.

### Requirement: Engram Command Recovery from config.yaml

When gentle-ai reconfigures Hermes, it MUST recover any engram `command` already present under `mcp_servers.engram` in `~/.hermes/config.yaml` and preserve it, rather than overwriting it with a bare `engram` command. Recovery MUST read the YAML config (via a read-only YAML helper, e.g. `filemerge.ReadYAMLMCPServerCommand(content, "engram")`) and feed the recovered command through the same stabilization path used for JSON agents (`stableEngramCommandForExisting`), so a versioned Homebrew cellar path is replaced with the stable `engram`/stable path while a user-customized absolute path is preserved. When no engram command is recovered, the stable fallback `engram` command MUST be used.

This requires `existingMergedEngramCommand` (or its equivalent) to branch on `AgentHermes` to the YAML reader BEFORE attempting JSON parsing, so YAML content never reaches the JSON merge path.

#### Scenario: Custom command preserved on reconfigure

- GIVEN `~/.hermes/config.yaml` has `mcp_servers:` → `engram:` → `command: /custom/path/engram`
- WHEN gentle-ai reconfigures Hermes (engram injection runs again)
- THEN the recovered `command` remains `/custom/path/engram` and is NOT clobbered with bare `engram`

#### Scenario: Versioned cellar command stabilized

- GIVEN `~/.hermes/config.yaml` has an engram `command` pointing at a versioned Homebrew cellar path
- WHEN gentle-ai reconfigures Hermes
- THEN recovery returns that command AND stabilization replaces it with the stable `engram` (or stable Homebrew path)

#### Scenario: No engram present — stable fallback used

- GIVEN `~/.hermes/config.yaml` has no `engram` entry under `mcp_servers:`
- WHEN gentle-ai reconfigures Hermes
- THEN recovery returns `("", false)` AND the stable fallback `engram` command is used

#### Scenario: Command as YAML list — executable recovered

- GIVEN `~/.hermes/config.yaml` has `mcp_servers.engram.command` expressed as a YAML list (e.g. `- engram` then `- mcp`)
- WHEN recovery runs
- THEN the first element (the executable) is recovered

---

## 10. Engram Setup Slug

### Requirement: No Native Engram Setup Slug

`SetupAgentSlug(model.AgentHermes)` MUST return `("", false)`. There is no native `engram setup hermes` command; MCP injection via `config.yaml` is the only supported path.

#### Scenario: Slug absent

- GIVEN `SetupAgentSlug` is called with `AgentHermes`
- WHEN the function returns
- THEN `slug` equals `""` and `ok` equals `false`

#### Scenario: ShouldAttemptSetup returns false

- GIVEN `AgentHermes` with no slug
- WHEN `ShouldAttemptSetup()` is evaluated
- THEN it returns `false`

---

## 11. SOUL.md Instruction Injection

### Requirement: SDD Orchestrator Injected into SOUL.md

When `sdd.Inject()` is called with the Hermes adapter, the SDD orchestrator content MUST be written into `~/.hermes/SOUL.md` using `<!-- gentle-ai:sdd-orchestrator -->` markers, without replacing content outside the markers.

#### Scenario: Orchestrator injection — fresh SOUL.md

- GIVEN `~/.hermes/SOUL.md` does not exist
- WHEN `sdd.Inject(homeDir, hermesAdapter, "")` is called
- THEN `~/.hermes/SOUL.md` is created and contains `<!-- gentle-ai:sdd-orchestrator -->` markers with orchestrator content between them

#### Scenario: Orchestrator injection — existing user content preserved

- GIVEN `~/.hermes/SOUL.md` exists with user-authored content
- WHEN `sdd.Inject()` is called
- THEN user content outside the `<!-- gentle-ai:sdd-orchestrator -->` markers is preserved verbatim
- AND the orchestrator block is inserted or updated within the markers

#### Scenario: Orchestrator injection — idempotent

- GIVEN `~/.hermes/SOUL.md` already contains the `<!-- gentle-ai:sdd-orchestrator -->` block
- WHEN `sdd.Inject()` is called again
- THEN the block is replaced in place, not appended again

### Requirement: SDD Orchestrator Asset References Hermes Paths

The Hermes SDD orchestrator asset MUST reference `~/.hermes/skills/` for skill paths (not `~/.claude/skills/`, `~/.qwen/skills/`, or any other agent path).

#### Scenario: Path correctness

- GIVEN the Hermes SDD orchestrator asset content
- WHEN scanned for skill path references
- THEN it contains `~/.hermes/skills/`

### Requirement: Agent-Specific Asset Selected

`sddOrchestratorAsset(model.AgentHermes)` MUST return `"hermes/sdd-orchestrator.md"`.

#### Scenario: Asset selection

- GIVEN `sddOrchestratorAsset` is called with `AgentHermes`
- WHEN the function returns
- THEN the string equals `"hermes/sdd-orchestrator.md"`

### Requirement: Strict-TDD Instructions Injected

The strict-TDD instructions MUST be injected into `~/.hermes/SOUL.md` via `<!-- gentle-ai:strict-tdd -->` markers using the standard markdown-sections flow.

---

## 12. Engram Protocol Documentation in SOUL.md

### Requirement: Engram Section Documents Complementary Relationship

The engram protocol section injected into `~/.hermes/SOUL.md` MUST explicitly explain the complementary relationship between engram (cross-agent, cross-session memory protocol) and Hermes's native memory and skill-learning loop, so users do not perceive them as conflicting.

#### Scenario: Complementary relationship documented

- GIVEN `~/.hermes/SOUL.md` with engram section injected
- WHEN its content is read
- THEN the engram section contains an explanation that Hermes native memory and engram serve different purposes (e.g., short-term context vs. long-term cross-session/cross-agent protocol)

---

## 13. Persona Injection (Option B — Agent-Specific Assets)

### Requirement: Hermes-Specific Persona Asset for Gentleman Options

For the `gentleman` and `gentleman-neutral-artifacts` persona options, `personaContent()` MUST return the content of a dedicated Hermes asset (e.g., `hermes/persona-gentleman.md`), not the generic `generic/persona-gentleman.md`.

The Hermes-specific asset MUST be a copy of the generic persona with the "Contextual Skill Loading (MANDATORY)" block rewritten to reference Hermes's native skill model (`~/.hermes/skills/` by category), removing the `<available_skills>` system-prompt assumption that is specific to Claude Code.

#### Scenario: Gentleman persona uses Hermes-specific asset

- GIVEN persona option is `gentleman` or `gentleman-neutral-artifacts`
- WHEN `personaContent(model.AgentHermes, personaOption)` is called
- THEN it returns content from `hermes/persona-gentleman.md`
- AND the content does NOT contain `<available_skills>` as an injection mechanism

#### Scenario: Generic asset not used for Hermes gentleman

- GIVEN persona option is `gentleman`
- WHEN `personaContent(model.AgentHermes, ...)` is called
- THEN it does NOT return content from `generic/persona-gentleman.md`

### Requirement: Hermes-Specific Persona Asset for Neutral Option

For the `neutral` persona option, `personaContent()` MUST return the content of a dedicated Hermes asset (`hermes/persona-neutral.md`), not the generic `generic/persona-neutral.md`. This requires `personaContent()` to support per-agent asset selection in the `PersonaNeutral` case (today the `PersonaNeutral` case returns a single, non-per-agent generic asset).

The Hermes neutral asset MUST be a copy of the generic neutral persona with the "Contextual Skill Loading (MANDATORY)" block rewritten for Hermes's native skill model (`~/.hermes/skills/` by category), removing the `<available_skills>` system-prompt assumption.

The per-agent neutral selection MUST return the byte-identical existing `generic/persona-neutral.md` for every non-Hermes agent (no regression).

#### Scenario: Neutral persona uses Hermes-specific asset

- GIVEN persona option is `neutral`
- WHEN `personaContent(model.AgentHermes, "neutral")` is called
- THEN it returns content from `hermes/persona-neutral.md`
- AND the content does NOT contain `<available_skills>` as an injection mechanism

#### Scenario: Neutral persona unchanged for other agents

- GIVEN persona option is `neutral` and the agent is not Hermes
- WHEN `personaContent(agent, "neutral")` is called
- THEN it returns the byte-identical content of `generic/persona-neutral.md`

### Requirement: Custom Persona Injects No Content

For the `custom` persona option, `personaContent()` MUST return empty content (no persona is written). This is identical to the behavior for all other agents.

#### Scenario: Custom persona skips injection

- GIVEN persona option is `custom`
- WHEN `personaContent(model.AgentHermes, "custom")` is called
- THEN it returns empty content and no persona block is injected into SOUL.md

### Requirement: Persona Content Written to SOUL.md

When persona injection runs for Hermes, the persona content MUST be written into `~/.hermes/SOUL.md` using the standard `StrategyMarkdownSections` flow (same `<!-- gentle-ai:persona -->` markers).

#### Scenario: Persona injected into SOUL.md

- GIVEN persona option is `gentleman` and Hermes adapter is selected
- WHEN persona injection runs
- THEN `~/.hermes/SOUL.md` contains the Hermes-specific gentleman persona content within `<!-- gentle-ai:persona -->` markers

### Requirement: Persona Language Contract — Technical Artifacts in English

Technical artifact content (code comments, SOUL.md injected blocks, skill references, identifiers) generated by gentle-ai for Hermes MUST default to English, regardless of persona voice or conversation language. The persona governs agent tone in SOUL.md replies, not the language of embedded technical content.

---

## 14. Permissions Injection

### Requirement: Permissions Injection Skipped

`permissions.Inject()` called with the Hermes adapter MUST return `nil` without writing any file. Hermes's permission model format is undocumented; injection is deferred until the format is known.

#### Scenario: Permissions skipped

- GIVEN the Hermes adapter
- WHEN `permissions.Inject(homeDir, hermesAdapter)` is called
- THEN no file is written and the return value is `nil`

---

## 15. Catalog Registration

### Requirement: Hermes in Agent Catalog

`catalog/agents.go` MUST include an entry for Hermes with tier `TierFull` and config path `~/.hermes`.

#### Scenario: Catalog entry exists

- GIVEN `catalog.AllAgents()` is called
- WHEN the result is scanned
- THEN an entry with `ID = "hermes"`, `Tier = TierFull`, and `ConfigPath` ending in `.hermes` is present

---

## 16. Config Scan

### Requirement: Known Config Directory

`knownAgentConfigDirs(homeDir)` MUST include `{Agent: "hermes", Path: "~/.hermes"}`.

#### Scenario: Config scan entry exists

- GIVEN `knownAgentConfigDirs` is called
- WHEN the result is scanned
- THEN an entry with `Agent = "hermes"` and `Path` ending in `.hermes` is present

---

## 17. CLI Validation

### Requirement: Agent Validation Case

The CLI agent validation switch MUST include a case for `"hermes"` that appends `model.AgentHermes` to the agents list.

---

## 18. TUI Agent Selection

### Requirement: TUI State Restoration

The TUI's `loadSelection()` function MUST include a case for `"hermes"` that appends `model.AgentHermes` to the selected agents list.

---

## 19. Factory and Registry

### Requirement: Factory Registration

`agents/factory.go` MUST register the Hermes adapter so that it is included in the default agent registry returned by `NewFactory().AllAdapters()`.

#### Scenario: Hermes in default registry

- GIVEN `NewFactory()` is called
- WHEN `AllAdapters()` is returned
- THEN the result contains an adapter with `ID() == "hermes"`

---

## 20. Assets Embedding

### Requirement: Hermes Assets Embedded

`internal/assets/assets.go` MUST include an `//go:embed all:hermes` directive (or equivalent) so that all files under `internal/assets/hermes/` are embedded in the binary.

#### Scenario: Asset embedded

- GIVEN the binary is compiled
- WHEN `assets.ReadFile("hermes/sdd-orchestrator.md")` is called
- THEN it returns the asset content without error

---

## 21. Explicit Non-Goals

The following are out of scope for this change and MUST NOT be implemented:

| Non-goal | Constraint |
|----------|------------|
| Auto-install Hermes | `SupportsAutoInstall()` returns `false`; no npm/curl/bash invocation |
| Profile-based config (`~/.hermes/profiles/<name>/`) | Only `~/.hermes/config.yaml` is targeted; documented limitation |
| Permissions overlay | Skip (return `nil`) until format is documented |
| Native `engram setup` slug | `SetupAgentSlug` returns `"", false` |
| Full YAML parser (anchors, multi-doc, deep nesting) | Helpers cover only flat KV + single `mcp_servers` table |

---

## 22. Test Coverage

### Requirement: Adapter Tests

`internal/agents/hermes/` MUST include:

- Table-driven detection tests: binary found, binary not found, stat error
- Install command test: `InstallCommand()` returns a non-nil error for Hermes
- `SupportsAutoInstall()` returns `false`
- Config path tests: all path methods return correct values for a given `homeDir`
- Capability flag tests: all boolean methods verified against expected values
- Strategy tests: `SystemPromptStrategy()` = `StrategyMarkdownSections`, `MCPStrategy()` = `StrategyMergeIntoYAML`

### Requirement: YAML Merge Tests

`internal/components/filemerge/yaml_test.go` MUST include golden-file or table-driven tests for:

- Insert when `mcp_servers:` key is absent
- Insert when `config.yaml` does not exist (file creation)
- Upsert is idempotent (second run produces no duplicate)
- User comments outside the managed block are preserved verbatim
- 2-space indentation is consistent in all emitted YAML

`internal/components/filemerge/yaml_test.go` MUST also include table-driven tests for the read-only recovery helper `ReadYAMLMCPServerCommand`:

- Scalar command (`command: engram`) → `("engram", true)`
- List command (`command:` followed by `- /path/engram` items) → first element `("/path/engram", true)`
- Server absent under `mcp_servers:` → `("", false)`
- `mcp_servers:` key absent entirely → `("", false)`
- Comment lines (`# ...`) inside/around the block are ignored and do not break recovery

### Requirement: SDD Injection Tests

`internal/components/sdd/inject_test.go` MUST include:

- `TestInjectHermesWritesSDDOrchestratorToSOULMD`: fresh home dir, SOUL.md created with markers
- `TestSDDOrchestratorAssetSelection` includes `AgentHermes` → `"hermes/sdd-orchestrator.md"` case
- Existing user content outside markers is preserved

### Requirement: MCP Injection Tests

`internal/components/mcp/inject_test.go` MUST include:

- `context7` injected into `~/.hermes/config.yaml` for Hermes
- `engram` injected into `~/.hermes/config.yaml` for Hermes
- Both injections are idempotent
- `StrategyMergeIntoYAML` case is covered

### Requirement: Engram Injection Tests

`internal/components/engram/inject_test.go` MUST include:

- Hermes cases for YAML overlay injection
- A recovery case asserting that a custom `mcp_servers.engram.command` already present in `config.yaml` is preserved across re-injection (not clobbered with bare `engram`)
- `setup_test.go` includes `AgentHermes` → `("", false)` case

### Requirement: Persona Injection Tests

`internal/components/persona/inject_test.go` MUST include:

- Hermes with `gentleman` option uses `hermes/persona-gentleman.md`
- Hermes with `neutral` option uses `hermes/persona-neutral.md`
- Non-Hermes agents with `neutral` option still use byte-identical `generic/persona-neutral.md` (no regression)
- Hermes with `custom` option injects no persona content
- Content does not contain `<available_skills>` for Hermes

### Requirement: Permissions Tests

`internal/components/permissions/inject_test.go` MUST include:

- Hermes returns `nil` from `Inject()` and writes no file

### Requirement: Registry and Config Scan Tests

- `internal/agents/registry_test.go` includes `AgentHermes` in the default registry test count
- `internal/system/config_scan_test.go` accounts for `"hermes"` in the total agent count
- `internal/cli/validate.go` test or manual coverage includes the `"hermes"` case
- `internal/tui/model_test.go` includes `"hermes"` in `makeDetectionWithAgents()` known agents list

---

## 23. Backward Compatibility

### Requirement: No Regression for Existing Agents

All changes MUST be purely additive. No existing agent's behavior, config paths, strategies, or test expectations may change as a result of adding Hermes support.

#### Scenario: Existing agent registry count

- GIVEN the existing agent registry before this change includes N agents
- WHEN this change is applied
- THEN the registry contains exactly N+1 agents, with all original agents intact and unmodified
