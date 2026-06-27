# Proposal: Headroom MCP Component

## Intent

Users lose 30-60% of LLM context window to redundant tool output, verbose logs, and unprocessed RAG chunks. Headroom compresses this data before it reaches the LLM, recovering 60-95% token budget via reversible compression (CCR pattern). For gentle-ai users with multiple MCP servers, this means fewer context-full errors and longer productive sessions.

## Scope

### In Scope
- Component constant, catalog entry, version pin, MVPGraph node
- MCP server file (`headroom.go`): pip-based, three tools (compress/retrieve/stats)
- Per-agent injection via existing `mcp.Inject()` — extends MCP injector
- TUI selection: component in FullGentleman + EcosystemOnly presets
- Install-time pip detection + auto-install (`headroom-ai[all]`)
- Doctor health check for headroom availability
- Target agents: Claude Code + OpenCode (extensible design)

### Out of Scope
- Proxy mode — MCP tools only (simpler, more portable)
- Agents beyond Claude Code + OpenCode — deferred
- Custom compressor config or headroom-defaults overrides
- Go SDK import integration — MCP only

## Capabilities

### New Capabilities
- `headroom-mcp`: Headroom context compression MCP server — install, configure, inject into supported agents

### Modified Capabilities
- None

## Approach

Seven-touchpoint pattern (identical to Context7):

1. `internal/model/types.go` — add `ComponentHeadroom`
2. `internal/catalog/components.go` — add to `mvpComponents`
3. `internal/versions/versions.go` — pinned version const with Renovate directive
4. `internal/planner/graph.go` — add `model.ComponentHeadroom: nil` to MVPGraph
5. `internal/tui/model.go` — add to `componentsForPreset` presets
6. `internal/components/mcp/headroom.go` — MCP server JSON + agent-strategy overlays (pip-based `headroom-ai[all]`)
7. `internal/cli/run.go` — `case model.ComponentHeadroom:` in `componentApplyStep.Run()`: detect pip, install if missing, call `mcp.Inject(homeDir, adapter)` per agent

Also: add `headroom` to `knownTools` in doctor.go, add backup paths in component path helper.

## Affected Areas

| Area | Impact |
|------|--------|
| `internal/model/types.go` | +ComponentHeadroom constant |
| `internal/catalog/components.go` | +mvpComponents entry |
| `internal/planner/graph.go` | +MVPGraph node (no deps) |
| `internal/versions/versions.go` | +Renovate-pinned version |
| `internal/tui/model.go` | +component in presets |
| `internal/components/mcp/headroom.go` | New file |
| `internal/components/mcp/inject.go` | +case headroom injection |
| `internal/cli/run.go` | +install + inject case |
| `internal/cli/doctor.go` | +headroom knownTools |
| `internal/cli/run_component_paths_test.go` | +backup path coverage |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| pip not available | Medium | Check pip/pip3 before install; guide user |
| Windows pip compatibility | Low | Pin known-good version; test in CI |
| HEADROOM not on PATH after pip | Low | `pip show headroom-ai` to resolve path (+gga pattern) |
| Python 3.12+ compat | Low | Verify headroom-ai supported Python range |

## Rollback Plan

Backup: pre-install snapshot covers all mcp config files. Uninstall: `gentle-ai uninstall` removes headroom mcp.json + runs `pip uninstall headroom-ai -y`. Manual: same pip command + delete per-agent headroom MCP files.

## Dependencies

- `headroom-ai[all]` on PyPI (Apache 2.0) — auto-installed via pip
- Python 3.10+ (headroom-ai requirement)

## Success Criteria

- [ ] `gentle-ai install --components headroom` installs + injects into Claude Code + OpenCode
- [ ] `gentle-ai doctor` shows headroom as pass
- [ ] TUI shows Headroom selectable in FullGentleman preset
- [ ] headroom_compress / retrieve / stats available as MCP tools post-install
- [ ] Existing Context7 installs unchanged
- [ ] Uninstall removes all headroom config
