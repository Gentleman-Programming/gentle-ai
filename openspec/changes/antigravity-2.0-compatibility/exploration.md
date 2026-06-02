# Antigravity compatibility exploration

## Finding

The new Antigravity implementation is compatible with the desktop product, not only the CLI. A separate public `antigravity-cli` agent ID is unnecessary.

## Relevant surfaces

- CLI/Desktop config root: `~/.gemini/antigravity-cli/`
- Shared prompt file: `~/.gemini/GEMINI.md`
- Skills root: `~/.gemini/antigravity-cli/skills/`
- MCP config: `~/.gemini/antigravity-cli/mcp_config.json`
- Dynamic subagent tools: `define_subagent`, `invoke_subagent`

## Recommendation

Replace the old `antigravity` behavior with the new implementation and remove the separate public `antigravity-cli` target.

## Files to update

- `internal/agents/antigravity/adapter.go`
- `internal/assets/antigravity/sdd-orchestrator.md`
- `internal/agents/factory.go`
- `internal/model/types.go`
- `internal/catalog/agents.go`
- `internal/cli/*`
- `internal/tui/*`
- `internal/components/engram/*`
- `internal/components/sdd/*`
- tests and golden files for Antigravity
