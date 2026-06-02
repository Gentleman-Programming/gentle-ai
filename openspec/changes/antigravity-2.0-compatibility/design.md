# Unified Antigravity support design

## Decision

Use the existing public `antigravity` agent ID for the new Antigravity implementation. Remove the standalone `antigravity-cli` option.

## File layout

| Purpose | Path |
|---------|------|
| Agent ID | `antigravity` |
| Config root | `~/.gemini/antigravity-cli/` |
| Settings | `~/.gemini/antigravity-cli/settings.json` |
| MCP config | `~/.gemini/antigravity-cli/mcp_config.json` |
| Skills | `~/.gemini/antigravity-cli/skills/` |
| Shared prompt/persona | `~/.gemini/GEMINI.md` |
| Engram plugin | `~/.gemini/antigravity-cli/plugins/gentle-ai-engram/` |

## Runtime model

The Go adapter does not install static subagent files. Instead, the SDD orchestrator asset tells Antigravity to define phase subagents dynamically at runtime with `define_subagent`, then execute them with `invoke_subagent`.

## Implementation notes

- `internal/agents/antigravity` owns the unified adapter.
- `internal/assets/antigravity/sdd-orchestrator.md` owns the dynamic subagent prompt.
- `internal/model/types.go` keeps only `AgentAntigravity` for this surface.
- CLI and TUI validation accept `antigravity`; they do not expose `antigravity-cli`.
- Engram MCP uses the default `engram mcp` invocation and adds an Antigravity plugin hook for tool-hint injection.

## Compatibility

Existing `antigravity` users keep the same installer agent name. The backing config path changes to the supported Antigravity Desktop-compatible surface.
