# Proposal: Copilot CLI Adapter

## Intent

GitHub Copilot CLI (`copilot` binary) is a distinct agent from VS Code Copilot — different binary, different config paths, different system prompt location. The current `vscode-copilot` adapter targets VS Code's extension architecture (settings.json, VS Code User profile). We need a dedicated adapter so `gga install --agent copilot-cli` correctly injects skills, MCP config, and system prompts for the CLI tool.

## Scope

### In Scope
- Add `AgentCopilotCLI` constant (`"copilot-cli"`) in `internal/model/types.go`
- New adapter package `internal/agents/copilotcli/adapter.go` implementing the `Adapter` interface
- Register in `internal/catalog/agents.go` (`allAgents` slice)
- Register in `internal/agents/factory.go` (`defaultAgentIDs` + `NewAdapter` switch)
- Add config scan entry in `internal/system/config_scan.go` (`knownAgentConfigDirs`)
- Unit tests: `internal/agents/copilotcli/adapter_test.go`

### Out of Scope
- Auto-install support (`SupportsAutoInstall() = false` — Copilot CLI is desktop-installed)
- Sub-agent support (`SupportsSubAgents() = false`)
- Changes to existing `vscode-copilot` adapter — it remains unchanged
- Changes to skill loading logic — shared `~/.copilot/skills/` already works

## Capabilities

### New Capabilities
- `copilot-cli-support`: Detection, config injection, skill loading, and MCP config for GitHub Copilot CLI

### Modified Capabilities
- None

## Approach

Implement the `Adapter` interface following the existing pattern (see `internal/agents/vscode/adapter.go`):

| Method | Value |
|--------|-------|
| `Agent()` | `model.AgentCopilotCLI` |
| `Tier()` | `model.TierFull` |
| `Detect()` | `exec.LookPath("copilot")` + `stat ~/.copilot/config.json` |
| `GlobalConfigDir()` | `~/.copilot` |
| `SystemPromptStrategy()` | `StrategyInstructionsFile` |
| `SystemPromptFile()` | `.github/copilot-instructions.md` (workspace-level) |
| `MCPStrategy()` | `StrategyMCPConfigFile` |
| `MCPConfigPath()` | `~/.copilot/mcp-config.json` |
| `SkillsDir()` | `~/.copilot/skills` |
| `SupportsAutoInstall()` | `false` |
| `SupportsSkills()` | `true` |
| `SupportsSubAgents()` | `false` |

Detection mirrors the Codex adapter pattern: PATH binary lookup + config file stat.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/model/types.go` | Modified | Add `AgentCopilotCLI AgentID = "copilot-cli"` constant |
| `internal/agents/copilotcli/` | New | New adapter package with `adapter.go` and `adapter_test.go` |
| `internal/catalog/agents.go` | Modified | Add Copilot CLI to `allAgents` slice |
| `internal/agents/factory.go` | Modified | Add to `defaultAgentIDs` and `NewAdapter` switch |
| `internal/system/config_scan.go` | Modified | Add entry to `knownAgentConfigDirs` |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Naming confusion with `vscode-copilot` | Low | Distinct AgentID (`copilot-cli` vs `vscode-copilot`), clear display names |
| Shared `~/.copilot/skills/` collision | Low | Both adapters target same dir intentionally — skills are agent-agnostic |
| `~/.copilot/config.json` may not exist on fresh install | Medium | Detection requires config.json; adapter still registers even if not found (installed=false) |
| Copilot CLI system prompt is workspace-only, not global | Low | `StrategyInstructionsFile` writes `.github/copilot-instructions.md` per-project — expected behavior |

## Rollback Plan

1. **Git revert**: `git revert <commit-hash>` — single change, no data migration, clean revert
2. **Uninstall verification**: After revert, confirm `gga uninstall --agent copilot-cli` is no longer accepted (agent removed from catalog)
3. **No data cleanup needed**: Adapter writes no persistent state beyond standard config files that Copilot CLI owns

## Dependencies

- None — uses existing `Adapter` interface and model types

## Success Criteria

- [ ] `gga install --agent copilot-cli` injects system prompt, MCP config, and skills correctly
- [ ] `gga detect` identifies Copilot CLI when `copilot` binary + `~/.copilot/config.json` exist
- [ ] `gga uninstall --agent copilot-cli` removes all injected Gentleman AI artifacts
- [ ] All tests pass: `go test ./...`
- [ ] No regression in existing `vscode-copilot` adapter tests
