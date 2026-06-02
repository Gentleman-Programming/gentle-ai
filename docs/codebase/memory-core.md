# Memory Core

[Back to Codebase Guide](../CODEBASE-GUIDE.md)

Gentle-AI wires Engram into agents; Engram owns the memory store. This page explains the boundary so maintainers do not confuse installer code with memory database code.

## Store responsibilities

| Responsibility | Owner in this repository | Actual memory owner |
|---|---|---|
| Install or download the `engram` binary | `internal/components/engram/install.go`, `download.go` | External Engram project/runtime |
| Add MCP config for agents | `internal/components/engram/inject.go` | Agent consumes the config |
| Run `engram setup` where supported | `internal/components/engram/setup.go`, CLI runtime | Engram CLI implements setup behavior |
| Document user commands | `docs/engram.md` | Engram CLI/MCP implementation |
| Store sessions, observations, prompts, relations, sync mutations | Not implemented here | Engram store |

## Memory entities

Gentle-AI docs and prompt assets refer to these Engram concepts, but their schema is not defined in this repo.

| Concept | Maintainer meaning |
|---|---|
| Sessions | Work periods that can be summarized and recovered later. |
| Observations | Saved decisions, discoveries, bug fixes, patterns, or artifacts. |
| Prompts | User prompts captured so later saves can attach intent. |
| Relations | Semantic links or conflict judgments between memories. |
| Sync mutations | Export/import changes used by Engram sync workflows. |

For command and MCP tool descriptions, link to [Engram Commands](../engram.md) instead of duplicating an API reference.

## Save and retrieve flow

```text
AI agent receives prompt
  |
  v
Gentle-AI-installed prompt tells agent to use Engram MCP tools
  |
  v
Agent calls `engram mcp --tools=agent` via configured MCP entry
  |
  +--> save: mem_save / mem_session_summary / related tools
  +--> retrieve: mem_context / mem_search / mem_get_observation
  |
  v
Engram runtime stores and searches memory outside gentle-ai source
```

## Memory invariants

- **MCP command must be stable**: `internal/components/engram/inject.go` prefers stable command paths and preserves existing absolute paths where needed.
- **Agent setup is capability-based**: `SetupAgentSlug` intentionally returns no setup target for agents that use direct config injection.
- **Prompt assets must stay accurate**: Engram instructions in `internal/assets/` must match the public behavior documented in `docs/engram.md`.
- **No schema invention**: do not document tables, HTTP routes, dashboard pages, or cloud internals unless source or external Engram docs confirm them.

## Contributor checklist

- [ ] Decide whether the change belongs to Gentle-AI wiring or Engram itself.
- [ ] Update MCP injection tests when changing config shape.
- [ ] Keep `docs/engram.md` as the user-facing command reference.
- [ ] Do not read or modify local `.engram/engram.db` as part of codebase docs or tests.

## Navigation

Previous: [Repository map](repository-map.md) | Next: [Interfaces](interfaces.md)
