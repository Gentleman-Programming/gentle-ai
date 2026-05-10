# Proposal: Qwen Code Agent Integration

## Intent

Add Qwen Code (by Alibaba/Alibaba Cloud) as a fully supported AI coding agent in the Gentle AI ecosystem, with parity to Gemini CLI's feature set. Qwen Code is an emerging agent in the AI-assisted development space with robust sub-agent capabilities, slash commands, and a `settings.json`-based config model that aligns well with the existing Gentle AI adapter pattern.

## Scope

### In Scope
- `internal/agents/qwen/` — new adapter package: `Adapter` struct implementing `agents.Adapter` interface
- `internal/model/types.go` — `AgentQwenCode` constant in `AgentID` enum
- `internal/agents/factory.go` — register adapter in `NewAdapter()` and `NewDefaultRegistry()`
- `internal/catalog/agents.go` — catalog entry with `TierFull` and `~/.qwen` config path
- `internal/assets/qwen/sdd-orchestrator.md` — Qwen-specific SDD orchestrator with `~/.qwen/skills/` paths
- `internal/assets/assets.go` — add `all:qwen` to `//go:embed` directive
- `internal/components/sdd/inject.go` — `sddOrchestratorAsset()` case for `AgentQwenCode`
- `internal/components/permissions/inject.go` — `qwenCodeOverlayJSON` with `auto_edit` mode
- `internal/components/engram/setup.go` — `"qwen-code"` slug mapping
- `internal/system/config_scan.go` — qwen-code entry in `knownAgentConfigDirs()`
- `internal/cli/validate.go` — agent validation case
- `internal/tui/model.go` — TUI agent selection case
- Test updates: `adapter_test.go`, `inject_test.go`, `setup_test.go`, `install_test.go`, `model_test.go`, `registry_test.go`, `config_scan_test.go`

### Out of Scope
- Qwen Code installation on Windows (npm-based install covers all platforms)
- Output style / thinking mode integration (Qwen CLI does not expose these as configurable features)
- Native workflow files (Qwen uses slash commands, not Windsurf-style `.windsurf/workflows/`)
- Sub-agent injection (Qwen's `task` sub-agent is intrinsic to the agent, not configured by Gentle AI)
- Model provider selection (Qwen uses its own models; no multi-provider catalog like OpenCode)

## Capabilities

### New Capabilities
- `qwen-code-adapter`: Full agent adapter with detection, auto-install, config paths, strategies
- `qwen-sdd-orchestrator`: Agent-specific SDD orchestrator with `~/.qwen/skills/` path references
- `qwen-permissions`: Auto-edit mode with manual shell approval
- `qwen-engram-slug`: `"qwen-code"` slug for `engram setup` integration

### Modified Capabilities
- `gga`: None (spec unchanged)
- `enorm`: Engram setup gains `qwen-code` slug

## Approach

1. **Adapter parity with Gemini CLI**: Qwen Code uses the same config model as Gemini CLI — `~/.qwen/` config root, `settings.json` with `mcpServers` key, and a `QWEN.md` system prompt file. The adapter reuses `StrategyFileReplace` for system prompt and `StrategyMergeIntoSettings` for MCP config.

2. **Key differentiator from Gemini**: Qwen Code supports custom slash commands via `~/.qwen/commands/*.md`. The adapter implements `SupportsSlashCommands() = true` and `CommandsDir()` — a capability Gemini CLI does not have. This opens future enhancement opportunities for SDD slash commands.

3. **Install via npm**: `npm install -g @qwen-code/qwen-code@latest` — same pattern as Claude Code, Gemini CLI, and Codex. Linux sudo handling follows existing `NpmWritable` convention.

4. **Permissions overlay**: `"defaultMode": "auto_edit"` — auto-approve file edits, manual approval for shell commands. Matches Qwen Code's native permission model.

5. **Zero-breaking changes**: All additions are additive. No existing agent behavior is modified.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/agents/qwen/` | New | Adapter package (adapter.go, adapter_test.go) |
| `internal/assets/qwen/` | New | SDD orchestrator asset (sdd-orchestrator.md) |
| `internal/model/types.go` | Modified | `AgentQwenCode` constant |
| `internal/agents/factory.go` | Modified | Registration in factory and default registry |
| `internal/catalog/agents.go` | Modified | Catalog entry |
| `internal/assets/assets.go` | Modified | Embed directive |
| `internal/components/sdd/inject.go` | Modified | `sddOrchestratorAsset()` switch |
| `internal/components/permissions/inject.go` | Modified | `qwenCodeOverlayJSON` + switch case |
| `internal/components/engram/setup.go` | Modified | `"qwen-code"` slug |
| `internal/system/config_scan.go` | Modified | `knownAgentConfigDirs()` entry |
| `internal/cli/validate.go` | Modified | Agent validation case |
| `internal/tui/model.go` | Modified | TUI agent selection case |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Qwen Code npm package name changes | Low | Pinned to `@qwen-code/qwen-code@latest`; can be updated via single constant change |
| Config paths differ across OSes | Med | Uses `~/.qwen/` consistently (Qwen's documented cross-platform config root) |
| Qwen Code adds/removes features between versions | Med | Adapter declares capabilities at integration time; future updates may adjust flags |
| Engram `engram setup` may not recognize `"qwen-code"` slug | Low | Slug follows established convention; engram component updated in same change |

## Rollback Plan

1. Remove `internal/agents/qwen/` directory — pure deletion, no dependencies
2. Remove `internal/assets/qwen/` directory — pure deletion
3. Revert switch case additions in `sddOrchestratorAsset()`, `agentOverlay()`, `SetupAgentSlug()`
4. Remove `AgentQwenCode` constant from `model/types.go`
5. Remove catalog entry, factory registration, config scan entry, TUI case
6. All changes are additive — rollback is clean deletion with no risk to existing agents

## Dependencies

- Qwen Code npm package: `@qwen-code/qwen-code@latest` (Node.js 20+)
- Existing `agents.Adapter` interface — no modifications required
- Existing `StrategyFileReplace` and `StrategyMergeIntoSettings` patterns — reused verbatim

## Success Criteria

- [ ] `go build ./...` passes with zero errors
- [ ] `go vet ./...` reports no issues
- [ ] `go test ./internal/agents/qwen/...` — all tests pass (detection, install, config paths, capabilities)
- [ ] `go test ./internal/components/sdd/...` — SDD injection test for Qwen Code passes
- [ ] `go test ./internal/components/engram/...` — engram setup slug test passes
- [ ] `go test ./internal/cli/...` — install validation and default agent test pass
- [ ] `go test ./internal/agents/...` — registry test includes Qwen Code
- [ ] Qwen Code appears in TUI agent selection screen
- [ ] `gentle-ai install --agent qwen-code --dry-run` shows correct plan
