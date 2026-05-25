# Design: Copilot CLI Adapter

## Technical Approach

Follow the existing adapter pattern (Codex detection + VS Code config strategies) to add GitHub Copilot CLI as a first-class agent. The adapter mirrors the VS Code Copilot layout (`~/.copilot` global root) but uses distinct paths for CLI-specific config files (`mcp-config.json` vs VS Code's `mcp.json`).

Detection uses binary-on-PATH + config-file stat (Codex pattern). Injection uses `StrategyInstructionsFile` for workspace-level system prompts and `StrategyMCPConfigFile` for MCP servers.

## Architecture Decisions

| Decision | Option | Tradeoff | Rationale |
|---|---|---|---|
| Detection strategy | `exec.LookPath("copilot")` + `stat ~/.copilot/config.json` | Requires config.json; fresh CLI installs may not have it | Copilot CLI is desktop-installed; we want certainty before claiming installed |
| MCP transport | stdio (local `npx` command) | Needs Node.js runtime on user's machine | Copilot CLI runs locally and uses stdio-based MCP; VS Code uses HTTP remote |
| Shared skills dir | `~/.copilot/skills/` (same as VS Code) | Namespace collision unlikely | Skills are agent-agnostic; intentional reuse avoids duplication |
| System prompt scope | `.github/copilot-instructions.md` (workspace-only) | No global system prompt | Copilot CLI reads instructions per-repo; matches documented behavior |

## Data Flow

```
User runs `gga install --agent copilot-cli`
         │
         v
    factory.go ──→ NewAdapter(AgentCopilotCLI)
         │
         v
  copilotcli.Adapter.Detect()
         │
    ┌────┴────┐
  found     not found
    │          │
    v          v
  inject    (still registers; installed=false)
    │
    ├─ SystemPrompt: .github/copilot-instructions.md
    ├─ Skills: ~/.copilot/skills/
    └─ MCP: ~/.copilot/mcp-config.json (stdio servers)
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/model/types.go` | Modify | Add `AgentCopilotCLI AgentID = "copilot-cli"` constant |
| `internal/agents/copilotcli/adapter.go` | Create | `Adapter` struct implementing `agents.Adapter` interface |
| `internal/agents/copilotcli/adapter_test.go` | Create | Unit tests: detection, paths, strategies, capabilities |
| `internal/catalog/agents.go` | Modify | Append Copilot CLI to `allAgents` slice |
| `internal/agents/factory.go` | Modify | Add `AgentCopilotCLI` to `defaultAgentIDs` and `NewAdapter` switch |
| `internal/system/config_scan.go` | Modify | Add `"copilot-cli"` entry to `knownAgentConfigDirs` |
| `internal/components/mcp/context7.go` | Modify | Add `CopilotCLIContext7OverlayJSON()` using stdio `mcpServers` schema |
| `internal/components/mcp/inject.go` | Modify | Branch on `AgentCopilotCLI` to use new overlay |

## Interfaces / Contracts

### Copilot CLI MCP Schema (stdio)

The MCP config uses the standard `mcpServers` key with stdio transport (local command execution), consistent with Claude Code and Cursor:

```json
{
  "mcpServers": {
    "context7": {
      "command": "npx",
      "args": ["-y", "--package=@upstash/context7-mcp@VERSION", "--", "context7-mcp"]
    },
    "engram": {
      "command": "engram",
      "args": ["mcp", "stdio"]
    }
  }
}
```

### Adapter Method Map

| Method | Value / Behavior |
|---|---|
| `Agent()` | `model.AgentCopilotCLI` |
| `Tier()` | `model.TierFull` |
| `Detect()` | `lookPath("copilot")` + `stat ~/.copilot/config.json` |
| `GlobalConfigDir()` | `~/.copilot` |
| `SystemPromptStrategy()` | `StrategyInstructionsFile` |
| `SystemPromptFile()` | `.github/copilot-instructions.md` (workspace-level) |
| `MCPStrategy()` | `StrategyMCPConfigFile` |
| `MCPConfigPath()` | `~/.copilot/mcp-config.json` |
| `SkillsDir()` | `~/.copilot/skills` |
| `SupportsAutoInstall()` | `false` |
| `SupportsSkills()` | `true` |
| `SupportsSubAgents()` | `false` |

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Detection logic (binary + config stat) | Table-driven tests with injected `lookPath` and `statPath` funcs (Codex pattern) |
| Unit | Config paths cross-platform | Assert exact paths with `filepath.Join` for all methods |
| Unit | Strategies and capabilities | Assert each method returns expected constant |
| Integration | `gga install --agent copilot-cli` | Golden tests verify `mcp-config.json` and `.github/copilot-instructions.md` written correctly |
| E2E | `gga detect` lists copilot-cli | Run detect against temp home with/without `copilot` binary and config |

## Migration / Rollout

No migration required. The adapter only writes config files Copilot CLI already owns. Uninstall removes injected Gentleman AI artifacts from `~/.copilot/mcp-config.json` and `.github/copilot-instructions.md` via existing uninstall service.

## Open Questions

- [ ] Does Copilot CLI `mcp-config.json` prefer `servers` or `mcpServers` key? (Pending verification against CLI docs — assumption is `mcpServers` based on stdio pattern.)
- [ ] Should Engram MCP server use stdio or HTTP? (Assuming stdio to match local CLI execution model.)
