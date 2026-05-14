# Spec: Trae IDE Agent Support

## Purpose

Define the exact requirements, scenarios, and acceptance criteria for adding Trae IDE as a first-class agent in gentle-ai.

---

## Requirements

### REQ-1: Agent Identity

The Trae adapter MUST expose `AgentTrae AgentID = "trae-ide"` and `TierFull` support tier.

#### Scenario: Agent ID and tier
- GIVEN the Trae adapter is instantiated via `trae.NewAdapter()`
- WHEN `Agent()` and `Tier()` are called
- THEN `Agent()` returns `"trae-ide"` and `Tier()` returns `"full"`

---

### REQ-2: Detection via config directory

The adapter MUST detect Trae by the presence of `~/.trae` as a directory. Trae is a desktop app — no binary appears on PATH.

#### Scenario: Trae installed (config dir exists)
- GIVEN `~/.trae` exists and is a directory
- WHEN `Detect(ctx, homeDir)` is called
- THEN returns `(true, "", "~/.trae", true, nil)`

#### Scenario: Trae not installed (config dir absent)
- GIVEN `~/.trae` does not exist
- WHEN `Detect(ctx, homeDir)` is called
- THEN returns `(false, "", "~/.trae", false, nil)`

#### Scenario: Stat error (permission / IO failure)
- GIVEN `os.Stat("~/.trae")` returns a non-`IsNotExist` error
- WHEN `Detect(ctx, homeDir)` is called
- THEN returns `(false, "", "", false, <error>)`

---

### REQ-3: Config paths

The adapter MUST return correct paths for all config methods. Trae uses a mixed layout: `~/.trae/` for detection and skills (cross-platform), and an OS-specific Trae User config dir (VS Code convention) for MCP and personal rules.

#### Scenario: GlobalConfigDir
- WHEN `GlobalConfigDir(homeDir)` is called
- THEN returns `{homeDir}/.trae`

#### Scenario: SystemPromptDir
- WHEN `SystemPromptDir(homeDir)` is called
- THEN returns the OS-specific Trae User config dir:
  - macOS: `{homeDir}/Library/Application Support/Trae/User`
  - Linux: `{homeDir}/.config/Trae/User` (or `$XDG_CONFIG_HOME/Trae/User`)
  - Windows: `%APPDATA%\Trae\User`

#### Scenario: SystemPromptFile
- WHEN `SystemPromptFile(homeDir)` is called
- THEN returns `{traeUserDir}/user_rules.md`

#### Scenario: SkillsDir
- WHEN `SkillsDir(homeDir)` is called
- THEN returns `{homeDir}/.trae/skills`

#### Scenario: MCPConfigPath
- WHEN `MCPConfigPath(homeDir, "")` is called
- THEN returns `{traeUserDir}/mcp.json`
- NOTE: `{traeUserDir}` is the OS-specific Trae User config dir (same as SystemPromptDir)

---

### REQ-4: Config strategies

The adapter MUST declare the correct strategies for system prompt and MCP injection.

#### Scenario: System prompt strategy
- WHEN `SystemPromptStrategy()` is called
- THEN returns `model.StrategyMarkdownSections`

#### Scenario: MCP strategy
- WHEN `MCPStrategy()` is called
- THEN returns `model.StrategyMCPConfigFile`

---

### REQ-5: Capability flags

The adapter MUST correctly report which optional capabilities Trae supports.

#### Scenario: Supported capabilities
- WHEN capability accessors are called
- THEN:
  - `SupportsSystemPrompt()` → `true`
  - `SupportsMCP()` → `true`
  - `SupportsSkills()` → `true`
  - `SupportsAutoInstall()` → `false`
  - `SupportsOutputStyles()` → `false`
  - `SupportsSlashCommands()` → `false`
  - `SupportsSubAgents()` → `false`
  - `SupportsWorkflows()` → `false`

---

### REQ-6: Not auto-installable

The adapter MUST return an error when `InstallCommand` is called, since Trae is a desktop app.

#### Scenario: InstallCommand returns error
- WHEN `InstallCommand(platformProfile)` is called
- THEN returns `(nil, AgentNotInstallableError{Agent: "trae"})`
- AND the error message contains `"trae"` and indicates it is a desktop app

---

### REQ-7: Factory and catalog registration

Trae MUST be included in the default agent registry so it participates in detection, sync, and list flows.

#### Scenario: Factory resolves Trae adapter
- WHEN `agents.NewAdapter(model.AgentTrae)` is called
- THEN returns a non-nil `*trae.Adapter` and `nil` error

#### Scenario: Trae in default registry
- WHEN `agents.NewDefaultRegistry()` is called
- THEN the registry contains an adapter for `model.AgentTrae`

#### Scenario: Trae in catalog
- WHEN `catalog.AllAgents()` is called
- THEN the result includes `{ID: model.AgentTrae, Name: "Trae", ConfigPath: "~/.trae"}`

---

### REQ-8: Config scan includes Trae

`system.ScanConfigs` MUST include Trae's config dir so the TUI detection screen and validation flows reflect Trae's installation state.

#### Scenario: ScanConfigs includes Trae entry
- WHEN `system.ScanConfigs(homeDir)` is called
- THEN the result contains an entry with `Agent == "trae"` and `Path == "{homeDir}/.trae"`

---

## Constraints

- All paths MUST use `filepath.Join` — no hardcoded separators.
- The adapter MUST NOT import the `system` package to avoid import cycles (follow `windsurf` adapter pattern).
- `Detect` MUST inject `statPath` via a struct field (same pattern as Windsurf) to enable unit testing without real filesystem.
- No production code in test files; adapter tests MUST use the injected `statPath` stub.

---

## Cases Limits

- `~/.trae` exists but is a file, not a directory: `Detect` returns `(false, "", "~/.trae", false, nil)` — `stat.isDir` is `false`.
- `homeDir` is empty string: paths degrade gracefully to `/.trae/...` — same behavior as all other adapters.
- `MCPConfigPath` receives a non-empty second argument (workspace dir): MUST be ignored (Trae MCP is user-level only).
