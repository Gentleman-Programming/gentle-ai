# Tasks: Headroom MCP Component

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 320–380 |
| 400-line budget risk | Low–Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | auto-forecast |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low–Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Foundation + Core + Integration + Testing | Single PR | All phases in one PR. `Inject()` signature change is backward-compatible — existing Context7 call sites add a third arg. ~350 lines fits within budget. |

## Phase 1: Foundation (Model + Catalog + Versions + Graph + TUI)

- [x] 1.1 Add `ComponentHeadroom ComponentID = "headroom"` to `internal/model/types.go`
- [x] 1.2 Add Headroom entry to `mvpComponents` in `internal/catalog/components.go`
- [x] 1.3 Add `HeadroomMCP` version const with `// renovate: datasource=pypi depName=headroom-ai` in `internal/versions/versions.go`
- [x] 1.4 Add `model.ComponentHeadroom: nil` to `MVPGraph()` in `internal/planner/graph.go`
- [x] 1.5 Add `model.ComponentHeadroom` to FullGentleman and EcosystemOnly presets in `internal/tui/model.go` `componentsForPreset()`

## Phase 2: Core (MCP Server JSON + Inject Refactor)

- [x] 2.1 Create `internal/components/mcp/headroom.go` with `DefaultHeadroomServerJSON()` (local command `"headroom"` + `args: ["mcp", "serve"]`), `OpenCodeHeadroomOverlayJSON()` (local type), `DefaultHeadroomOverlayJSON()` (mcpServers), `VSCodeHeadroomOverlayJSON()`, `OpenClawHeadroomOverlayJSON()`, `AntigravityHeadroomOverlayJSON()`, `KimiHeadroomOverlayJSON()` following the `context7.go` pattern — **note**: binary is `headroom` not `headroom-mcp`
- [x] 2.2 Generalize `Inject()` in `internal/components/mcp/inject.go` — add `componentID model.ComponentID` param, pass it to all strategy functions
- [x] 2.3 Refactor each strategy function in `inject.go` (`injectSeparateFile`, `injectMergeIntoSettings`, `injectMCPConfigFile`, `injectTOMLFile`, `injectYAMLFile`, `injectOpenClawMergeIntoSettings`) to dispatch JSON payload by `componentID`
- [x] 2.4 Update `injectTOMLFile` to handle headroom (local command via `UpsertCodexMCPServerBlock`)
- [x] 2.5 Update `injectYAMLFile` to handle headroom via `UpsertYAMLMCPServerBlock`

## Phase 3: Integration (Run + Doctor + Backup Paths)

- [x] 3.1 Add `case model.ComponentHeadroom:` in `internal/cli/run.go` `componentApplyStep.Run()` — detect pip/pip3, `pip install headroom-ai[all]`, then `mcp.Inject(home, adapter, ComponentHeadroom)` per adapter
- [x] 3.2 Update existing `case model.ComponentContext7:` in `run.go` to pass `model.ComponentContext7` as third arg to `mcp.Inject()`
- [x] 3.3 Add `"headroom"` to `knownTools` slice in `internal/cli/doctor.go`
- [x] 3.4 Add headroom MCP config file paths to `componentPathsWithWorkspaceScoped()` in `internal/cli/run.go` under a `case model.ComponentHeadroom:`
- [x] 3.5 Add `case model.ComponentHeadroom:` to `componentPathDirScoped()` for `homeDir` routing (headroom is global, like Context7)

## Phase 4: Testing

- [x] 4.1 Update all `Inject()` call sites in `internal/components/mcp/inject_test.go` to pass the third `componentID` argument (`model.ComponentContext7` for existing tests, `model.ComponentHeadroom` for new ones)
- [x] 4.2 Create `internal/components/mcp/headroom_test.go` with 8 tests — JSON shapes (`"command": "headroom"`), all 7 overlay variants, copy-safety
- [x] 4.3 Add inject tests for Headroom: 18 tests covering idempotent inject into Claude Code, OpenCode, OpenClaw, Codex, VS Code, Antigravity, Kimi, Hermes
- [x] 4.4 Add headroom backup path coverage + update install_test.go expectations
