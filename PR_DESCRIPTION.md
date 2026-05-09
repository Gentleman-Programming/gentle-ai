# feat: Add Pi coding agent support

## Summary

This PR adds first-class MVP support for **Pi coding agent** (`pi-coding-agent`) as the 14th supported agent in the Gentle-AI ecosystem.

Pi is intentionally integrated as a **solo-agent** (no native sub-agents or MCP in the core), which is the correct abstraction given Pi's architecture: it ships with prompt templates, Agent Skills, and extensions — but not built-in sub-agent delegation or MCP config surfaces.

## What changed

### New files (11)

| File | Purpose |
|------|---------|
| `internal/agents/pi/adapter.go` | Pi adapter: detection (`pi` binary + `~/.pi/agent`), npm auto-install, config paths, honest capability flags |
| `internal/agents/pi/adapter_test.go` | Table-driven tests following the exact style of `codex/` and `qwen/` adapters |
| `internal/assets/pi/sdd-orchestrator.md` | Solo-agent SDD orchestrator tailored for Pi (no `mem_*` MCP calls, `openspec` as safe default) |
| `internal/assets/pi/prompts/sdd-init.md` | Pi prompt template: initialize SDD context |
| `internal/assets/pi/prompts/sdd-explore.md` | Pi prompt template: explore a topic |
| `internal/assets/pi/prompts/sdd-apply.md` | Pi prompt template: implement tasks |
| `internal/assets/pi/prompts/sdd-verify.md` | Pi prompt template: validate implementation |
| `internal/assets/pi/prompts/sdd-archive.md` | Pi prompt template: archive change |
| `internal/assets/pi/prompts/sdd-onboard.md` | Pi prompt template: guided walkthrough |
| `internal/assets/pi/prompts/sdd-new.md` | Pi prompt template: start a new change |
| `internal/assets/pi/prompts/sdd-continue.md` | Pi prompt template: continue next phase |
| `internal/assets/pi/prompts/sdd-ff.md` | Pi prompt template: fast-forward planning |

### Modified files (25)

**Core model & wiring:**
- `internal/model/types.go` — Added `AgentPiCodingAgent` and `TierPartial` support tier
- `internal/agents/factory.go` — Registered Pi adapter in `NewAdapter` and `defaultAgentIDs`
- `internal/catalog/agents.go` — Added Pi to `allAgents` with `TierPartial`, config path `~/.pi/agent`
- `internal/system/config_scan.go` — Added `~/.pi/agent` to `knownAgentConfigDirs` (also backfilled missing `openclaw`)
- `internal/cli/validate.go` — Added `pi-coding-agent` to `defaultAgentsFromDetection` switch
- `internal/tui/model.go` — Added Pi to `preselectedAgents` (also backfilled missing `kilocode`, `kimi`, `kiro-ide`, `openclaw`)

**Asset embedding & routing:**
- `internal/assets/assets.go` — Added `all:pi` to `//go:embed`
- `internal/assets/commands.go` — Routed `AgentPiCodingAgent` → `"pi/prompts"` for SDD slash commands
- `internal/components/sdd/inject.go` — Routed `sddOrchestratorAsset` to `"pi/sdd-orchestrator.md"`; fixed `injectMarkdownSections` to use agent-specific asset (also fixed missing `AgentClaudeCode` case)

**Safety guards (no fake MCP/Engram/Theme):**
- `internal/cli/run.go` — Added `SupportsMCP()` guards for `ComponentEngram` and `ComponentContext7`; skips `ComponentTheme` for Pi until a theme asset exists
- `internal/components/theme/inject.go` — Returns no-op for Pi with explanatory comment

**Tests:**
- `internal/agents/factory_test.go` — Added Pi to registry and factory resolution tests
- `internal/agents/registry_test.go` — Added Pi to `TestDefaultRegistryIncludesAllAgents`
- `internal/catalog/agents_test.go` — Added `TestAllAgentsIncludesPiCodingAgent` and `TestIsSupportedAgentAcceptsPiCodingAgent`
- `internal/system/config_scan_test.go` — Updated agent count from 12 → 14
- `internal/cli/install_test.go` — Updated default agent list and `makeDetectionWithAgents`
- `internal/cli/run_component_paths_test.go` — Added Pi-specific path assertions (system prompt, prompts, skills; rejects MCP/theme paths)
- `internal/components/sdd/inject_test.go` — Added `TestInjectPiWritesPromptTemplatesAndSkills` + fixed `TestSDDOrchestratorAssetSelection` expectations
- `internal/components/theme/inject_test.go` — Added `TestInjectSkipsPiUntilThemeAssetExists`
- `internal/assets/assets_test.go` — Added Pi files to expected list + `TestPiEmbeddedAssetLayout`
- `internal/tui/model_test.go` — Updated `knownAgents` map and preselection tests

**Docs & E2E:**
- `README.md` — "13 Supported Agents" → "14 Supported Agents"; added Pi row
- `docs/agents.md` — Added Pi matrix row (Skills: Yes, MCP: No, Delegation: Solo-agent, Slash Commands: Yes via prompt templates); added honest "Agent Notes" section
- `docs/platforms.md` — Added Pi config path
- `e2e/lib.sh` — Added `rm -rf "$HOME/.pi"` to `cleanup_test_env()`

## Design decisions

### Why solo-agent / `SupportsSubAgents=false`?

Pi core intentionally does not include sub-agents or plan mode. The optional `pi-subagents` package exists but is not auto-installed by Gentle-AI. Claiming full delegation would be misleading.

### Why `SupportsMCP=false`?

Pi has no native MCP config surface (no `mcpServers` key in settings, no dedicated MCP JSON file). MCP integration would require a Pi extension, which is out of scope for this MVP.

### Why no sudo on unknown Linux?

Termux and other unknown Linux profiles often lack `sudo`. The install command only uses `sudo` on known distros (Ubuntu/Debian/Arch/Fedora) with non-writable global npm.

### Why `APPEND_SYSTEM.md` instead of `AGENTS.md`?

Pi separates context files (`AGENTS.md`) from system prompt files (`SYSTEM.md` / `APPEND_SYSTEM.md`). Using `APPEND_SYSTEM.md` keeps Gentle-AI's managed instructions in the proper layer without clobbering user context files.

## Testing

```bash
go build ./cmd/gentle-ai
go test ./...
```

**Result:** ✅ All tests pass. The only pre-existing failure (`TestWithPostInstallNotesDoesNotChangeNonGGA` in `internal/cli`) also fails on clean `main` in this Termux environment and is unrelated to Pi.

## Checklist

- [x] Adapter follows existing patterns (codex/qwen/openclaw)
- [x] Table-driven tests for adapter identity, detection, install, paths, capabilities, strategies
- [x] No fake MCP/Engram/Context7 config generated for Pi
- [x] Theme injection safely skipped for Pi
- [x] All hardcoded agent counts updated (14 agents)
- [x] TUI preselection + CLI detection synced
- [x] Docs updated with honest capability matrix
- [x] E2E cleanup includes Pi config dir
- [x] `go build` passes
- [x] `go test ./...` passes (except 1 pre-existing unrelated failure)

## Follow-up ideas (not in this PR)

- Optional `pi-subagents` package integration for full SDD delegation
- Pi extension-based MCP/Engram bridge
- Pi-specific theme asset (`gentleman-kanagawa` for Pi)
- `PI_CODING_AGENT_DIR` override support
