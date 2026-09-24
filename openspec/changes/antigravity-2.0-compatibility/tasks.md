# Tasks

## 1. Public agent surface

- [x] Keep `antigravity` as the public agent ID.
- [x] Remove the separate `antigravity-cli` model constant, catalog entry, factory registration, CLI mapping, and TUI option.
- [x] Update config scanning so detected Antigravity config maps to `antigravity`.

## 2. Unified adapter and assets

- [x] Replace the legacy `internal/agents/antigravity` adapter with the unified implementation.
- [x] Remove `internal/agents/antigravitycli`.
- [x] Move dynamic SDD orchestration to `internal/assets/antigravity/sdd-orchestrator.md`.
- [x] Remove `internal/assets/antigravitycli`.

## 3. Engram and SDD behavior

- [x] Write Antigravity MCP config to `~/.gemini/config/mcp_config.json`.
- [x] Install Antigravity Engram plugin hooks under `<variantDir>/plugins/gentle-ai-engram/` (IDE → Desktop → CLI fallback).
- [x] Use `engram mcp --tools=agent` invocation (standard across all agents).
- [x] Ensure Antigravity SDD instructions use dynamic subagents.

## 4. Verification

- [x] Update unit tests and golden tests for unified `antigravity` behavior.
- [x] Run targeted Go tests.
- [x] Run `go test ./...`.
- [x] Run `git diff --check`.
