# Interfaces

[Back to Codebase Guide](../CODEBASE-GUIDE.md)

Gentle-AI exposes a CLI and TUI. It configures MCP for agents. It does not expose a local HTTP API in this repository.

## Interface map

| Interface | Status in this repo | Primary files | Read for details |
|---|---|---|---|
| CLI | Implemented | `cmd/gentle-ai/main.go`, `internal/app/`, `internal/cli/` | [Usage](../usage.md) |
| TUI | Implemented | `internal/tui/model.go`, `internal/tui/router.go`, `internal/tui/screens/` | [Usage](../usage.md) |
| MCP | Configured, not hosted by Gentle-AI | `internal/components/engram/`, `internal/components/mcp/` | [Engram Commands](../engram.md) |
| Local HTTP API | Not present in this source tree | No dashboard/server package found | Use external Engram docs if needed |

## CLI flow

```text
argv
  -> app.Run
  -> cli.Parse*Flags
  -> Normalize*Flags
  -> planner.Resolve
  -> pipeline.Execute
  -> verify.Report
```

Use CLI packages for non-interactive behavior such as `install`, `sync`, `uninstall`, `restore`, `update`, and `upgrade`. Keep CLI docs focused on user workflows; do not replicate every internal struct.

## MCP flow

Gentle-AI writes agent-specific MCP configuration so agents can call external servers. Engram configuration usually invokes:

```text
engram mcp --tools=agent
```

OpenCode/Kilo Code use an array-style local MCP command in settings. Other agents use their adapter strategy: separate MCP files, merged settings, dedicated MCP config files, or TOML.

## TUI flow

The TUI is a Bubbletea state machine:

| TUI layer | Job |
|---|---|
| `Model` | Stores selected agents, components, cursor, async state, and screen-specific state. |
| `Screen` constants | Define the visible flow. |
| `router.go` | Defines simple previous/next navigation. |
| `screens/` | Renders individual screens. |
| async messages | Report pipeline, sync, upgrade, uninstall, and plugin registration results. |

## Local HTTP API boundary

No local HTTP server, route table, HTMX package, or dashboard package is present in this repository. If a future change adds one, it should include:

- [ ] A clearly named server package.
- [ ] Route ownership documented here.
- [ ] Authentication/session boundary documented in [Dashboard](dashboard.md).
- [ ] Links from user docs without duplicating full endpoint schemas.

## Navigation

Previous: [Memory core](memory-core.md) | Next: [Sync and cloud](sync-and-cloud.md)
