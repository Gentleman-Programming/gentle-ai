# Pi native MCP — retire pi-mcp-adapter

Locator: `odd/tasks/pi-native-mcp.md` · Engram topic: `odd/pi-native-mcp/tasks` · Branch: `feat/pi-native-mcp`

## Objective

Stop provisioning `pi-mcp-adapter` for Pi and rely on Pi's built-in MCP support (Pi >= 0.99.0).

## Problem / Why

Pi 0.99.0 ships `builtin:mcp`, which reads `mcp.json`. Per Pi `docs/mcp.md`, any installed extension that registers `/mcp` (such as `pi-mcp-adapter`) replaces the built-in support. Gentle AI still installs the adapter, so it now shadows the native runtime. Pi < 0.99.0 has no built-in MCP, so dropping the adapter requires a Pi version floor.

## Scope

- Stop declaring `npm:pi-mcp-adapter` in Pi `settings.json` and the `pi-mcp-adapter` dependency in `<agentDir>/npm/package.json`.
- Actively retire existing adapter entries from both files on install/sync.
- CodeGraph Pi probe must not require `node_modules/pi-mcp-adapter/index.ts`.
- TUI install plan, docs (`docs/pi.md`, `docs/quickstart.md`), and tests updated.
- Uninstall keeps removing legacy adapter entries.

## Constraints

- Preserve unrelated Pi settings and dependencies.
- `mcp.json` stays owned by `pi-engram init` (gentle-engram).
- Out of scope: any Pi >= 0.99.0 version floor — the user is implementing it separately.

## Tasks

- [x] T1 — Pi adapter: drop adapter provisioning, retire existing entries (+ tests). Route: delegated (writer trigger: 2+ non-trivial files).
  - `ProvisionEngramMCP` (name kept, doc rewritten) now only prunes `pi-mcp-adapter` (all identity forms) plus legacy/retired packages from an existing `settings.json` and the `pi-mcp-adapter` dependency from an existing `npm/package.json`; missing files are never created, untouched files stay byte-identical, returned paths are only rewritten files.
  - `InstallCommand` no longer runs `pi install npm:pi-mcp-adapter`; `pi-engram init` now follows `gentle-engram`. New `pi.UninstallPackageSources()` keeps `pi remove npm:pi-mcp-adapter` in uninstall cleanup.
  - RED observed: new adapter/engram tests failed (undefined helpers; adapter still written). GREEN: `go test ./internal/agents/pi/ ./internal/components/engram/` ok.
  - Checks: `go build ./...` ok; `go test ./internal/agents/pi/... ./internal/components/... ./internal/tui/...` ok; `go test -timeout 40m ./internal/cli/` ok (748s; the default 10m timeout is too short for this package). Risk tier: medium (behavior change, focused tests).
- [ ] T2 — CodeGraph Pi probe: remove adapter-path precondition (+ tests). Route: delegated.
- [ ] T3 — TUI install plan, docs, and remaining test fixtures. Route: delegated.

## Acceptance criteria

- Fresh Pi provisioning writes no `pi-mcp-adapter` entry anywhere.
- Existing `settings.json`/`package.json` with the adapter lose it after sync; other entries survive.
- `go test ./...` passes.

## Checks

- `go test ./internal/agents/pi/... ./internal/components/... ./internal/tui/... ./internal/cli/...`
- `go vet ./...`

## Delivery

Strategy: `ask-on-risk`. Forecast: ~300–500 authored changed lines.

## Progress

- Branch created from `main` (af24bfd94).
- Scope change: version floor removed (user implements it elsewhere).
- Engram mirror: PENDING (save failed: multiple active runtime sessions).
