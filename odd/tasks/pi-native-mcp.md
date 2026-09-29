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
- [x] T2 — CodeGraph Pi probe: remove adapter-path precondition (+ tests). Route: delegated.
  - T1 commit: `7d51a6773`.
  - `probePiCodeGraphMCP` no longer stats `npm/node_modules/pi-mcp-adapter/index.ts`; the unused agent-dir variants collapsed into `probePiCodeGraphMCPContext`. Fail-closed tests now pin `PATH` to an empty dir so a host `codegraph` binary cannot satisfy them.
  - RED observed: 4 communitytool tests failed with "Pi MCP adapter extension is unavailable". GREEN: `go test ./internal/components/communitytool/` ok.
  - Checks: `go build ./...` ok; `go vet ./internal/components/... ./internal/cli/...` ok; `go test ./internal/agents/pi/... ./internal/components/... ./internal/tui/...` ok; `go test ./internal/cli/ -run 'CodeGraph|CommunityTool|Pi'` ok (full CLI suite runs at T3). Risk tier: medium.
- [x] T3 — TUI install plan, docs, and remaining test fixtures. Route: delegated.
  - T2 commit: `d6f8ad868`.
  - `piInstallCommands` drops `pi install npm:pi-mcp-adapter`; `docs/pi.md` and `docs/quickstart.md` describe Pi's built-in MCP (Pi >= 0.99.0) and the adapter retirement. Uninstall fixtures list the legacy adapter last, matching `pi.UninstallPackageSources()`.
  - RED observed: both Pi dependency-tree tests failed on the new "retired adapter absent" assertion. GREEN: `go test ./internal/tui/...` ok.
  - Checks: `go build ./...` ok; `go vet ./...` ok; `go test -timeout 40m ./internal/agents/pi/... ./internal/components/... ./internal/tui/... ./internal/cli/...` ok (cli 732s). Risk tier: medium (docs passive, TUI copy covered by tests).
  - Remaining `pi-mcp-adapter` hits are retirement logic, uninstall cleanup, docs explaining the retirement, and tests seeding/asserting its absence.

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
- T1–T3 done on `feat/pi-native-mcp`; next: parent review, then push/PR decision (user-owned).
