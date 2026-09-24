# Unified Antigravity support design

## Decision

Use the existing public `antigravity` agent ID for the new Antigravity implementation. Remove the standalone `antigravity-cli` option.

## File layout

| Purpose | Path |
|---------|------|
| Agent ID | `antigravity` |
| Config root | `<variantDir>` (IDE → Desktop → CLI fallback) |
| Settings | `<variantDir>/settings.json` |
| MCP config | `~/.gemini/config/mcp_config.json` |
| Skills | `<variantDir>/skills/` |
| Shared prompt/persona | `~/.gemini/GEMINI.md` |
| Engram plugin | `~/.gemini/config/plugins/gentle-ai-engram/` (when migrated) or `<variantDir>/plugins/gentle-ai-engram/` |

## Runtime model

The Go adapter does not install static subagent files. Instead, the SDD orchestrator asset tells Antigravity to define phase subagents dynamically at runtime with `define_subagent`, then execute them with `invoke_subagent`.

## Implementation notes

- `internal/agents/antigravity` owns the unified adapter.
- `internal/assets/antigravity/sdd-orchestrator.md` owns the dynamic subagent prompt.
- `internal/model/types.go` keeps only `AgentAntigravity` for this surface.
- CLI and TUI validation accept `antigravity`; they do not expose `antigravity-cli`.
- Engram MCP uses `engram mcp --tools=agent` (standard across all agents) and adds an Antigravity plugin hook for tool-hint injection.

## Compatibility

Existing `antigravity` users keep the same installer agent name. The backing config path changes to the supported Antigravity Desktop-compatible surface.
