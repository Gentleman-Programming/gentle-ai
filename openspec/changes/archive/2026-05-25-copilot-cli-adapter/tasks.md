# Tasks: Copilot CLI Adapter

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~300–360 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

## Phase 1: Foundation — Model & Types

- [x] 1.1 Add `AgentCopilotCLI AgentID = "copilot-cli"` to `internal/model/types.go` (line 19, after `AgentPi`)

## Phase 2: Adapter Package

- [x] 2.1 Create `internal/agents/copilotcli/adapter.go` — implement `Adapter` interface
  - `Agent()` → `model.AgentCopilotCLI`
  - `Tier()` → `model.TierFull`
  - `Detect()` → `exec.LookPath("copilot")` + `os.Stat(homeDir + "/.copilot/config.json")` (injectable for test)
  - `GlobalConfigDir(homeDir)` → `filepath.Join(homeDir, ".copilot")`
  - `SystemPromptStrategy()` → `model.StrategyInstructionsFile`
  - `SystemPromptFile(homeDir)` → `filepath.Join(homeDir, ".github", "copilot-instructions.md")` (workspace-level; no OS-specific logic needed — design specifies workspace-only)
  - `MCPStrategy()` → `model.StrategyMCPConfigFile`
  - `MCPConfigPath(homeDir, _)` → `filepath.Join(homeDir, ".copilot", "mcp-config.json")`
  - `SkillsDir(homeDir)` → `filepath.Join(homeDir, ".copilot", "skills")`
  - `SupportsAutoInstall()` → `false`
  - `SupportsSkills()` → `true`
  - `SupportsSubAgents()` → `false`
  - `SupportsMCP()` → `true`
  - `SupportsSystemPrompt()` → `true`
  - `InstallCommand()` → return `AgentNotInstallableError` (same pattern as vscode adapter)
- [x] 2.2 Create `internal/agents/copilotcli/adapter_test.go`
  - `TestStrategies` — assert `StrategyInstructionsFile` and `StrategyMCPConfigFile`
  - `TestDetectionFound` — stub `lookPath` and `statPath` to return success, assert `(true, _, _, _, nil)`
  - `TestDetectionNotFound` — stub `lookPath` to return `errNotFound`, assert `(false, ...)`
  - `TestDetectionMissingConfig` — `lookPath` succeeds but `stat ~/.copilot/config.json` fails → `(false, ...)`
  - `TestPaths` — assert `GlobalConfigDir`, `MCPConfigPath`, `SkillsDir`, `SystemPromptFile` match expected `~/.copilot/*` paths
  - `TestCapabilities` — assert `SupportsAutoInstall()=false`, `SupportsSkills()=true`, `SupportsSubAgents()=false`, `SupportsMCP()=true`
  - Use `t.TempDir()` + injectable stubs pattern (mirror Codex/VS Code adapter test style)

## Phase 3: Registration — Catalog & Factory

- [x] 3.1 Add `AgentCopilotCLI` to `internal/catalog/agents.go`
  - Append `{ID: model.AgentCopilotCLI, Name: "Copilot CLI", Tier: model.TierFull, ConfigPath: "~/.copilot"}` to `allAgents` slice
- [x] 3.2 Add `internal/agents/copilotcli` import to `internal/agents/factory.go`
- [x] 3.3 Add `model.AgentCopilotCLI` to `defaultAgentIDs` slice in `internal/agents/factory.go`
- [x] 3.4 Add `case model.AgentCopilotCLI: return copilotcli.NewAdapter(), nil` to `NewAdapter()` switch

## Phase 4: System Scan & MCP Injection

- [x] 4.1 Add `"copilot-cli"` entry to `knownAgentConfigDirs` in `internal/system/config_scan.go` — path = `~/.copilot` (reuses same helper as VS Code)
- [x] 4.2 Create `CopilotCLIMCPConfigJSON()` in `internal/components/mcp/context7.go`
  - Use `mcpServers` key with stdio transport (matching design spec)
  - Output: `{"mcpServers":{"context7":{"command":"npx","args":["-y","--package=@upstash/context7-mcp@VERSION","--","context7-mcp"]}}}`
  - Note: Open Question in design — if CLI uses `servers` key instead, add a discovery/verification task (see Phase 6)
- [x] 4.3 Branch `AgentCopilotCLI` in `injectMCPConfigFile` in `internal/components/mcp/inject.go`
  - Add: `if adapter.Agent() == model.AgentCopilotCLI { overlay = CopilotCLIMCPConfigJSON() }` after the `AgentKimi` branch

## Phase 5: Testing — Verification

- [ ] 5.1 Run `go test ./internal/agents/copilotcli/...` — all adapter tests pass
- [ ] 5.2 Run `go test ./internal/catalog/...` — verify `IsSupportedAgent("copilot-cli")` returns true
- [ ] 5.3 Run `go test ./internal/agents/...` — factory and registry tests pass (no regression)
- [ ] 5.4 Run `go test ./internal/components/mcp/...` — verify `Inject(home, copilotcli.NewAdapter())` merges `context7` into `~/.copilot/mcp-config.json` and is idempotent
- [ ] 5.5 Run `go test ./internal/system/...` — verify `ScanConfigs` includes `"copilot-cli"` entry
- [ ] 5.6 Run full `go test ./...` — no regressions

## Phase 6: Open Question — MCP Schema Discovery (conditional)

- [ ] 6.1 **If** Copilot CLI `mcp-config.json` uses `servers` key instead of `mcpServers`:
  - Update `CopilotCLIMCPConfigJSON()` in `context7.go` to use `servers` key
  - Add test: `TestInjectCopilotCLIWritesContext7ToMCPConfigFile` in `inject_test.go` asserting correct key
  - Document finding in PR and update design open question

## Implementation Order

1. **Types** (Phase 1) — add the constant first; all other work depends on `model.AgentCopilotCLI`.
2. **Adapter** (Phase 2) — implement the core logic in isolation; test it in isolation.
3. **Registration** (Phase 3) — wire into catalog + factory; tests confirm the wiring.
4. **MCP** (Phase 4) — add the overlay and injection branch; tested together.
5. **Verification** (Phase 5) — run full test suite to confirm no regressions.
6. **Schema discovery** (Phase 6) — only if needed based on actual Copilot CLI behavior.

Rationale: Model constant first so everything else can reference it. Adapter in isolation because it's the core logic. Registration next because it exposes the adapter. MCP last because it consumes the adapter. Tests layered to match.