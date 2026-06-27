# Design: Headroom MCP Component

## Technical Approach

Add Headroom as an optional MCP component following the same seven-touchpoint pattern as Context7, with one key structural difference: Headroom is a **local pip-based Python process** while Context7 is a **remote HTTP endpoint**. This impacts the MCP server JSON structure (command vs URL), install flow (pip detection + pip install precede injection), and the OpenCode overlay (local type instead of remote).

No new abstractions are introduced — the existing `mcp.Inject()` function is extended to accept a component ID so it dispatches to the correct JSON payloads.

## Architecture Decisions

### Decision: Generalize `mcp.Inject()` with a component parameter

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Hardcode Headroom methods alongside Context7 | Duplicates 5 strategy methods per component | ❌ |
| Add `componentID` param to `Inject()` | One function, one switch, less boilerplate for future MCP components | ✅ |

`Inject()` signature changes from `(homeDir string, adapter agents.Adapter)` to `(homeDir string, adapter agents.Adapter, componentID model.ComponentID)`. The inner strategy functions (`injectSeparateFile`, `injectMergeIntoSettings`, etc.) receive the component and select the correct JSON payload. The `context7.go` helpers remain alongside new `headroom.go` helpers; the dispatcher in `inject.go` becomes generic.

### Decision: Headroom uses local MCP for all agents

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Remote HTTP MCP | Requires hosting a headroom cloud service — out of scope | ❌ |
| Local command (Python process) | Portable, no infra, matches headroom-ai architecture | ✅ |

Headroom runs as a local subprocess (`headroom-mcp` command, installed via pip). Every agent overlay uses `command` + `args`, never `type: "remote"` or `url`. This is the primary difference from Context7's OpenCode/Kimi/Antigravity overlays.

### Decision: Pip detection + auto-install in componentApplyStep, before mcp.Inject

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Install in separate prepare step | Over-engineered for one pip install | ❌ |
| Inline in componentApplyStep case | Matches GGA install pattern, keeps flow local | ✅ |

The Headroom case in `componentApplyStep.Run()` checks for `headroom-mcp` on PATH via `exec.LookPath`. If absent, checks for `pip`/`pip3` via `exec.LookPath`, runs `pip install headroom-ai[all]`, then proceeds to `mcp.Inject()`. This mirrors GGA's inline install-then-inject flow.

### Decision: Pin headroom-ai version from PyPI

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Pin a SemVer range | Renovate can still bump but range may drift | ❌ |
| Pin exact version | Predictable, Renovate handles bumps | ✅ |

Add `HeadroomMCP = "0.3.0"` with `// renovate: datasource=pypi depName=headroom-ai` (exact version TBD — confirmed during implementation).

## Injection Flow

```
componentApplyStep.Run()
  │
  ├─ [Headroom case]
  │   ├─ LookPath("headroom-mcp")
  │   │   └─ not found → LookPath("pip") or LookPath("pip3")
  │   │       └─ found → runCommand("pip", "install", "headroom-ai[all]")
  │   │       └─ not found → return error "pip not found"
  │   │
  │   └─ mcp.Inject(homeDir, adapter, ComponentHeadroom)
  │       └─ Inject() dispatches by MCPStrategy + componentID
  │           ├─ StrategySeparateMCPFiles → DefaultHeadroomServerJSON()
  │           ├─ StrategyMergeIntoSettings → DefaultHeadroomOverlayJSON()
  │           │   └─ OpenCode/KiloCode → OpenCodeHeadroomOverlayJSON()  (local type)
  │           ├─ StrategyMCPConfigFile → DefaultHeadroomOverlayJSON()
  │           │   └─ VS Code → VSCodeHeadroomOverlayJSON()
  │           │   └─ Kimi → KimiHeadroomOverlayJSON()
  │           │   └─ Antigravity → AntigravityHeadroomOverlayJSON()
  │           ├─ StrategyTOMLFile → injectHeadroomTOMLFile()
  │           └─ StrategyMergeIntoYAML → injectHeadroomYAMLFile()
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/model/types.go` | Modify | Add `ComponentHeadroom ComponentID = "headroom"` |
| `internal/catalog/components.go` | Modify | Add Headroom entry to `mvpComponents` |
| `internal/versions/versions.go` | Modify | Add `HeadroomMCP` version const with Renovate PyPI directive |
| `internal/planner/graph.go` | Modify | Add `model.ComponentHeadroom: nil` to MVPGraph |
| `internal/tui/model.go` | Modify | Add `model.ComponentHeadroom` to FullGentleman and EcosystemOnly presets |
| `internal/components/mcp/headroom.go` | **Create** | MCP server JSON + agent-strategy overlays (pip-based local command) |
| `internal/components/mcp/inject.go` | Modify | Generalize `Inject()` with componentID param; add headroom dispatch |
| `internal/cli/run.go` | Modify | Add `case model.ComponentHeadroom:` — pip detect/install + inject |
| `internal/cli/doctor.go` | Modify | Add `"headroom-mcp"` to `knownTools` |
| `internal/cli/run_component_paths_test.go` | Modify | Add headroom backup path coverage |
| `internal/components/mcp/inject_test.go` | Modify | Update `Inject()` call sites (new componentID param) |

## Interfaces / Contracts

### Headroom server JSON (Claude Code, separate file pattern)

```json
{
  "command": "headroom",
  "args": ["mcp", "serve"]
}
```

### Headroom overlay JSON (OpenCode/KiloCode merge pattern)

```json
{
  "mcp": {
    "headroom": {
      "type": "local",
      "command": "headroom",
      "args": ["mcp", "serve"]
    }
  }
}
```

### Headroom overlay JSON (Cursor/Antigravity/Kimi mcp.json pattern)

```json
{
  "mcpServers": {
    "headroom": {
      "command": "headroom",
      "args": ["mcp", "serve"]
    }
  }
}
```

### Inject() signature change

```go
// Before: Context7-only
func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error)

// After: generic, component-dispatched
func Inject(homeDir string, adapter agents.Adapter, componentID model.ComponentID) (InjectionResult, error)
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Headroom server JSON helpers | `headroom_test.go` — verify JSON includes `"command": "headroom-mcp"` |
| Unit | Headroom overlay JSON helpers | Validate each overlay format per agent strategy |
| Unit | Inject with ComponentHeadroom | Idempotent inject per adapter type, same pattern as `inject_test.go` |
| Unit | Pip detection + fallback | Mock `exec.LookPath` and `runCommand` in `run.go` tests |
| Integration | Install flow | Test `componentApplyStep.Run()` with ComponentHeadroom, mock adapters |
| Integration | Backup paths | `componentPaths` returns headroom MCP config paths per adapter |

## Migration / Rollout

No migration required. Headroom is additive — no existing configs change. The `Inject()` signature changes, so Context7 call sites in `run.go` are updated to pass `model.ComponentContext7`.

## Open Questions

- [ ] Headroom-ai PyPI package exact command name: verify `headroom-mcp` is the correct binary name (or if `python -m headroom_mcp` is needed). The design assumes `headroom-mcp` — confirm during implementation.
- [ ] Headroom-ai version to pin: start with latest at implementation time.
- [ ] Hermes YAML overlay: does Hermes need a special Headroom block like Context7's `UpsertHermesContext7Block`? Likely yes — add `injectHeadroomYAMLFile()` with a generic `UpsertHermesMCPServerBlock("headroom", ...)` if the Hermes helpers are currently Context7-specific.
