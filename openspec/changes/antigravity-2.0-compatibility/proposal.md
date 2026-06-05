# Replace legacy Antigravity with unified support

## Summary

Replace the existing public `antigravity` installer target with the new Antigravity implementation that uses the Gemini-compatible CLI/Desktop configuration surface. Do not expose a separate public `antigravity-cli` agent ID.

## Motivation

The new Antigravity implementation works for the desktop product as well as the CLI. Keeping both `antigravity` and `antigravity-cli` would create a confusing installer choice and preserve legacy behavior that no longer matches the supported surface.

## Proposed changes

- Keep `antigravity` as the public agent ID.
- Remove the separate `antigravity-cli` public agent ID and adapter.
- Make `antigravity` write to `~/.gemini/antigravity-cli/` for settings, MCP config, plugins, and global skills.
- Keep global prompt/persona content in `~/.gemini/GEMINI.md`.
- Use dynamic subagent orchestration through `define_subagent` and `invoke_subagent`.
- Install the Engram MCP configuration and Antigravity plugin hook for the unified `antigravity` agent.

## Acceptance criteria

- `gentle-ai install --agent antigravity ...` uses the unified Antigravity behavior.
- `antigravity-cli` is not available as a separate catalog, CLI, or TUI agent option.
- Legacy Antigravity config under `~/.gemini/antigravity/` is no longer written by this support path.
- Tests and golden files cover the unified `antigravity` behavior.
