# Pi sync — ensure mcp.json for built-in MCP

Locator: `odd/tasks/pi-sync-mcp-json.md` · Engram topic: `odd/pi-sync-mcp-json/tasks` · Branch: `fix/pi-sync-mcp-json` · Issue: #5103

## Objective

Make `gentle-ai sync` leave a Pi `mcp.json` that Pi's built-in MCP reads, so post-sync verification passes and migrated hosts keep their servers.

## Problem / Why

Post-sync verification expects `<Pi agent dir>/mcp.json` (`internal/cli/run.go`, `componentPathsWithWorkspaceScoped`, Engram + `StrategyMCPConfigFile`), but sync never writes it for Pi: `injectWithOptions` delegates to `ProvisionEngramMCP`, which only prunes `pi-mcp-adapter`. Only `pi-engram init` (install path) writes `mcp.json`. Hosts that renamed `mcp.json` to `mcp-adapter.json` for adapter 3.x fail verification (#5103) and, after #5121 retired the adapter, have no MCP servers at all.

## Scope

- On Pi Engram provisioning (install and sync): when `mcp-adapter.json` exists, merge its `mcpServers` entries into `mcp.json` (create if absent); existing `mcp.json` entries win; never delete `mcp-adapter.json`.
- When neither file provides an Engram entry, ensure `mcp.json` gets one compatible with what `pi-engram init` writes.
- Tests for each case plus idempotency.

## Constraints

- Preserve unrelated keys and servers in `mcp.json`.
- Honor `PI_CODING_AGENT_DIR` via existing agent-dir resolution.
- Out of scope: Pi version floor.

## Tasks

- [x] T1 — Migrate `mcp-adapter.json` servers and ensure Engram entry in `mcp.json` during Pi provisioning (+ tests). Route: delegated (preparation + 2+ non-trivial files).

## Acceptance criteria

- Host with only `mcp-adapter.json`: after sync, `mcp.json` exists with its servers; verification passes.
- Host with both: servers only in `mcp-adapter.json` are added; `mcp.json` values unchanged.
- Host with neither: `mcp.json` has an Engram entry.
- Second run reports no change.

## Checks

- `go test ./internal/agents/pi/... ./internal/components/engram/...`
- `go test -timeout 40m ./internal/cli/...`
- `go vet ./...`, `go run ./internal/gofmtcheck`

## Progress

- Branch created from `main` (c4de51f2a). #5036 closed as resolved by #5121; #5103 labeled `status:approved`, `type:bug`.
- T1 done (delegated writer). `ProvisionEngramMCP` now runs `ensurePiMCPConfigFile` after the adapter pruning: missing `mcpServers` entries from `mcp-adapter.json` are merged into `mcp.json` (existing entries win, other keys kept, `mcp-adapter.json` never touched), then the `pi-engram init` Engram entry is added when absent. Malformed JSON or a non-object `mcpServers` returns an error naming the file, with no write. Commit: `e71fa33c3`.
  - Engram entry shape evidence: gentle-engram 0.1.15 (local npx cache) and 0.1.16 (`npm pack`, latest) `cli.js` `createEngramServerConfig` → `{"command":"node","args":["-e",MCP_LAUNCHER],"lifecycle":"lazy","directTools":false}`; `mcp-template.json` identical.
  - RED: `go test ./internal/agents/pi/ -run ProvisionEngramMCP` failed (new table cases, malformed cases, adjusted fresh-dir/override tests); `TestRunSyncPiEngramMigratesMCPAdapterConfigAndPassesVerification` against the old adapter failed with the #5103 error (`verify:sync:file:.../.pi/agent/mcp.json ... no such file or directory`).
  - GREEN: same tests pass; `TestInjectPiProvisioningWritesOnlyMCPConfigOnFreshHome` replaces the old no-write expectation.
  - Checks: `go build ./...`, `go vet ./...`, `go run ./internal/gofmtcheck`, `go test ./internal/agents/pi/... ./internal/components/...`, `go test -timeout 40m ./internal/cli/...` all pass (cli: ok 747.9s).
  - Risk tier: medium (ordinary behavior change covered by focused tests; writes a user config file only when entries are missing).
