# Exploration: Copilot CLI Adapter

## Current State

Gentle-AI currently has **15 agent adapters** in `internal/agents/`, all implementing `Adapter` from `interface.go`. The VS Code Copilot adapter (`vscode-copilot`) shares the `~/.copilot` directory, but it targets VS Code's extension architecture (settings.json, VS Code User profile, `code` binary detection). The Copilot CLI is a **different binary** (`copilot`, at `/opt/homebrew/bin/copilot`) with its own config at `~/.copilot/config.json` and `~/.copilot/mcp-config.json`.

The architecture has no provision for a separate Copilot CLI agent — it would be a distinct AgentID, distinct adapter, and distinct config paths.

## Affected Areas

| File | Why it's affected |
|------|-------------------|
| `internal/model/types.go` | Must add `AgentCopilotCLI` constant |
| `internal/catalog/agents.go` | Must register the new agent in `allAgents` slice |
| `internal/versions/versions.go` | May need a version const if Copilot CLI is npm-installed |
| `internal/agents/` (new) | New `copilotcli/` package with `adapter.go` |
| `internal/system/config_scan.go` | Must add detection for `copilot` binary + `~/.copilot/config.json` |
| `internal/skillregistry/registry.go` | May need to add `~/.copilot/skills/` (already present for VSCode, verify CLI uses same) |
| `internal/cli/install_test.go` | Test fixture for known agents must include new agent |
| `internal/tui/model_test.go` | Test fixture for known agents must include new agent |

## Key Findings

### 1. Interface Contract (`internal/agents/interface.go`)
The `Adapter` interface requires:
- **Identity**: `Agent()` → `model.AgentID`, `Tier()` → `model.SupportTier`
- **Detection**: `Detect(ctx, homeDir) → (installed, binaryPath, configPath, configFound, err)`
- **Installation**: `SupportsAutoInstall()`, `InstallCommand(profile)`
- **Config paths** (all return strings, no file I/O): `GlobalConfigDir`, `SystemPromptDir`, `SystemPromptFile`, `SkillsDir`, `SettingsPath`
- **Strategies**: `SystemPromptStrategy()`, `MCPStrategy()`
- **MCP**: `MCPConfigPath(homeDir, serverName)`
- **Optional capabilities**: `SupportsOutputStyles`, `SupportsSlashCommands`, `SupportsSubAgents`, `SupportsSkills`, `SupportsSystemPrompt`, `SupportsMCP`

### 2. Adapter Patterns

| Adapter | SystemPromptStrategy | MCPStrategy | Auto-install | Sub-agents | Key characteristic |
|---------|---------------------|-------------|--------------|------------|-------------------|
| Claude | `StrategyMarkdownSections` | `StrategySeparateMCPFiles` | Yes (npm) | Yes | Markdown sections with markers |
| VS Code | `StrategyInstructionsFile` | `StrategyMCPConfigFile` | No | No | `.instructions.md` files |
| Codex | `StrategyFileReplace` | `StrategyTOMLFile` | Yes (npm) | No | `agents.md` file replace |
| Antigravity | `StrategyAppendToFile` | `StrategyMCPConfigFile` | No | No | Appends to `GEMINI.md` |
| OpenCode | `StrategyFileReplace` | `StrategyMergeIntoSettings` | Yes (brew/npm) | No | Replaces `AGENTS.md` |

### 3. Copilot CLI Config Locations (verified)

```
~/.copilot/
├── config.json          # Global settings (JSON, managed automatically)
├── mcp-config.json      # MCP server config
├── skills/              # Global skills directory
├── session-store.db
├── session-state/
├── command-history-state.json
└── ide/
    └── (IDE-specific config, likely VS Code extension relay)
```

**Critical distinction**: VS Code Copilot adapter uses `~/.copilot/skills` but targets VS Code's User profile (`Library/Application Support/Code/User/` on macOS) for settings/MCP/prompts. The Copilot CLI uses `~/.copilot/config.json` and `~/.copilot/mcp-config.json` directly at the top level.

### 4. System Prompt for Copilot CLI

Copilot CLI reads `.github/copilot-instructions.md` at the **workspace level** (project root). There is **no known global system prompt file** equivalent to `CLAUDE.md`, `AGENTS.md`, or `GEMINI.md`.

Available injection strategies:
- `StrategyInstructionsFile` — write to `.github/copilot-instructions.md` (workspace-level, already used by VS Code adapter)
- `StrategyFileReplace` — risk of clobbering user content
- `StrategyAppendToFile` — would append to `.github/copilot-instructions.md` if it exists

Since Copilot CLI doesn't have a global system prompt file, we should use `StrategyInstructionsFile` pointing to `.github/copilot-instructions.md`, mirroring the VS Code adapter's approach but with the CLI-specific config paths.

### 5. MCP for Copilot CLI

`~/.copilot/mcp-config.json` — JSON config for MCP servers. This is similar to VS Code's approach but lives at the top level of `~/.copilot/` rather than in the VS Code User profile.

The `StrategyMCPConfigFile` strategy is appropriate — it writes to a dedicated `mcp.json` file. For Copilot CLI, we use `mcp-config.json`.

### 6. Skills for Copilot CLI

`~/.copilot/skills/` — already exists and is used by VS Code Copilot. Copilot CLI likely reads from the same location. We should set `SupportsSkills() = true` and `SkillsDir` → `~/.copilot/skills`.

### 7. Detection

Copilot CLI detection:
- Binary: `exec.LookPath("copilot")` — check PATH
- Config: `~/.copilot/config.json` — check file existence
- Strategy: similar to Codex (`lookPath` + `stat` config dir)

## Proposed Internal Structure

```
internal/agents/copilotcli/
├── adapter.go       # Adapter implementation
└── adapter_test.go  # Unit tests
```

### Recommended Adapter Config

```go
func (a *Adapter) Agent() model.AgentID {
    return model.AgentCopilotCLI  // New constant: "copilot-cli"
}

func (a *Adapter) Tier() model.SupportTier {
    return model.TierFull
}

func (a *Adapter) Detect(...) {
    // 1. lookPath("copilot") → binaryPath
    // 2. stat ~/.copilot/config.json → configFound
    // Return: installed, binaryPath, "~/.copilot", configFound
}

func (a *Adapter) GlobalConfigDir(homeDir string) string {
    return filepath.Join(homeDir, ".copilot")
}

func (a *Adapter) SystemPromptDir(homeDir string) string {
    return filepath.Join(homeDir, ".github")  // workspace-level
}

func (a *Adapter) SystemPromptFile(homeDir string) string {
    return filepath.Join(homeDir, ".github", "copilot-instructions.md")
}

func (a *Adapter) SkillsDir(homeDir string) string {
    return filepath.Join(homeDir, ".copilot", "skills")
}

func (a *Adapter) SettingsPath(homeDir string) string {
    return filepath.Join(homeDir, ".copilot", "config.json")
}

func (a *Adapter) SystemPromptStrategy() model.SystemPromptStrategy {
    return model.StrategyInstructionsFile
}

func (a *Adapter) MCPStrategy() model.MCPStrategy {
    return model.StrategyMCPConfigFile
}

func (a *Adapter) MCPConfigPath(homeDir string, _ string) string {
    return filepath.Join(homeDir, ".copilot", "mcp-config.json")
}

func (a *Adapter) SupportsAutoInstall() bool {
    return false  // Copilot CLI is desktop-installed; verify if CLI has npm install
}

func (a *Adapter) SupportsSkills() bool        { return true }
func (a *Adapter) SupportsSystemPrompt() bool  { return true }
func (a *Adapter) SupportsMCP() bool           { return true }
func (a *Adapter) SupportsSubAgents() bool      { return false }
```

## Integration Points

### Catalog
Add to `allAgents` in `internal/catalog/agents.go`:
```go
{ID: model.AgentCopilotCLI, Name: "Copilot CLI", Tier: model.TierFull, ConfigPath: "~/.copilot"}
```

### Model Types
Add constant in `internal/model/types.go`:
```go
AgentCopilotCLI   AgentID = "copilot-cli"
```

### System Config Scan
Add to `agents` slice in `internal/system/config_scan.go`:
```go
{Agent: "copilot-cli", Path: copilotcliGlobalConfigDir(homeDir)},
// where copilotcliGlobalConfigDir returns "~/.copilot"
```

### Registry Bootstrap
The registry is built from adapter instances. A `copilotcli.NewAdapter()` must be instantiated and passed to `NewRegistry()` at the call site (likely in a bootstrap or factory function). Pattern matches all existing adapters.

## Risks

1. **Naming collision**: `AgentCopilotCLI` vs `AgentVSCodeCopilot` — ensure clear disambiguation in UI and docs
2. **Skills overlap**: Both VS Code Copilot and Copilot CLI share `~/.copilot/skills/` — skill injection is shared, which is fine but must be documented
3. **MCP config collision**: VS Code Copilot uses VS Code User profile for MCP; Copilot CLI uses `~/.copilot/mcp-config.json` — no collision but different paths
4. **System prompt scoping**: Copilot CLI only supports workspace-level `.github/copilot-instructions.md` — no global override, meaning the persona/SOMETHING prompt is per-project
5. **Auto-install unknown**: Need to verify if Copilot CLI can be installed via npm/brew (the `SupportsAutoInstall()` question)
6. **Sub-agents**: Copilot CLI may have its own sub-agent concept — need to investigate

## Ready for Proposal

**Yes** — the exploration is complete. Next step: `sdd-propose` to formalize scope, approach, and rollback plan.

Key decisions to lock in the proposal:
1. AgentID value: `copilot-cli` (keep it simple)
2. Detection strategy: PATH (`copilot`) + config dir (`~/.copilot`)
3. System prompt: workspace-level `.github/copilot-instructions.md` (no global equivalent)
4. MCP: `~/.copilot/mcp-config.json` with `StrategyMCPConfigFile`
5. Skills: `~/.copilot/skills/`
6. Auto-install: `false` (pending verification)
7. Sub-agents: `false` (pending investigation)