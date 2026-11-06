# Pi sync — ensure mcp.json for built-in MCP

Locator: `odd/tasks/pi-sync-mcp-json.md` · Engram topic: `odd/pi-sync-mcp-json/tasks` · Branch: `fix/pi-sync-mcp-json` · Issue: #5103

## Objective

Make `gentle-ai sync` leave a Pi `mcp.json` that Pi's built-in MCP reads, so post-sync verification passes and migrated hosts keep their servers.

## Problem / Why

Post-sync verification expects `<Pi agent dir>/mcp.json` (`internal/cli/run.go`, `componentPathsWithWorkspaceScoped`, Engram + `StrategyMCPConfigFile`), but sync never writes it for Pi: `injectWithOptions` delegates to `ProvisionEngramMCP`, which only prunes `pi-mcp-adapter`. Only `pi-engram init` (install path) writes `mcp.json`. Hosts that renamed `mcp.json` to `mcp-adapter.json` for adapter 3.x fail verification (#5103) and, after #5121 retired the adapter, have no MCP servers at all.

## Scope

- On Pi Engram provisioning (install and sync): when `mcp-adapter.json` exists, merge its `mcpServers` entries into `mcp.json` (create only when there is a server to migrate); existing `mcp.json` entries win; never delete `mcp-adapter.json`.
- Never add an `engram` server to Pi `mcp.json`, and never remove a user-owned one (T2: Pi Engram is native-only).
- Post-sync verification must not require Pi `mcp.json` for the Engram component (T2).
- Tests for each case plus idempotency.

## Constraints

- Preserve unrelated keys and servers in `mcp.json`.
- Honor `PI_CODING_AGENT_DIR` via existing agent-dir resolution.
- Out of scope: Pi version floor.

## Tasks

- [x] T1 — Migrate `mcp-adapter.json` servers and ensure Engram entry in `mcp.json` during Pi provisioning (+ tests). Route: delegated (preparation + 2+ non-trivial files). **Partially superseded by T2:** the migration stays; the Engram-entry injection was removed.
- [x] T2 — Pi Engram is native-only: drop the Engram MCP entry injection, create `mcp.json` only to hold migrated servers, and fix #5103 at the verification layer (+ tests, docs). Route: delegated (2+ non-trivial files).

## Acceptance criteria

- Host with only `mcp-adapter.json`: after sync, `mcp.json` exists with its servers and no Gentle AI-added `engram` server; verification passes.
- Host with both: servers only in `mcp-adapter.json` are added; `mcp.json` values unchanged.
- Host with neither: no `mcp.json` is created; verification passes.
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
- Native review (base c4de51f2a, risk high, consent granted): lineage `review-437d9b60008c2978`, 4 lenses, approved and acknowledged (authority burned). Advisory (non-blocking): R4 malformed `mcp-adapter.json` hard-fails sync; R3 partial write not reported in returned paths; R1 `mcp.json` file mode; R3 launcher drift vs gentle-engram unguarded; R2 readability nits.
- Follow-up (gentle-engram): `pi-engram init` 0.1.16 still adds `npm:pi-mcp-adapter` to Pi `settings.json`.
- T2 (direction change, user-approved). Why: gentle-engram 0.1.17 (engram main 9dad41b, #1476) makes Pi Engram native-only — `pi-engram init` no longer registers Engram MCP and warns when `mcp.json` has `mcpServers.engram` ("Pi native-only agent writes are not guaranteed while this MCP path remains"). Gentle AI adding that entry (T1) would trigger the warning, and `gentle-engram@latest` is what install runs.
  - `internal/agents/pi/adapter.go`: removed `piEngramMCPLauncher`, `piEngramMCPServer`, `piEngramMCPServerName`; `ensurePiMCPConfigFile` → `migratePiMCPAdapterServers`, which returns before reading `mcp.json` when `mcp-adapter.json` has no servers, so `mcp.json` is created only for a migration and a malformed or non-object `mcp.json` errors only when servers must be written into it. `piEngramMCPConfigFile` → `piMCPConfigFile`.
  - `internal/cli/run.go` `verificationComponentPaths`: excludes the Pi `mcp.json` from Engram verification (same pattern as the Codex SDD-profile exclusion). `componentPathsWithWorkspaceScoped` is unchanged, so backups still cover a migration write (pinned by `TestPiEngramMCPConfigIsBackedUpButNotVerified`).
  - Doctor finding (not changed, outside scope): `checkEngramReachable` (`internal/cli/doctor.go`) reads `mcpServers.engram` from Pi `mcp.json` via `engram.ReadPersistedStdioCommands`; a Pi-only native host yields WARN "no persisted MCP configuration found" with a "run gentle-ai sync" remedy that cannot clear it. Not a failure; follow-up candidate.
  - RED: `go test ./internal/agents/pi/ ./internal/components/engram/` failed (5 pi tests, 2 inject tests: an engram server/`mcp.json` was written with nothing to migrate); `go test ./internal/cli/ -run TestRunSyncPiEngram` failed (both hosts). After the adapter change alone, `TestRunSyncPiEngramWithoutMCPConfigPassesVerification` still failed with the #5103 error (`verify:sync:file:.../.pi/agent/mcp.json ... no such file or directory`), proving the verification fix is needed.
  - GREEN: same tests pass after the adapter and `verificationComponentPaths` changes.
  - Risk tier: medium (removes a write; verification narrowed for one agent/file, backups unchanged).
- T2 commit: `f5aafec8f`. Parent spot check: pi/engram packages and `go test ./internal/cli/ -run PiSync|PiEngram` ok.
- Native review T2 (lineage `review-fb4706da61ee49ce`, medium, consent granted): reliability lens approved, acknowledged (authority burned). Advisory (non-blocking): R3 at adapter.go:468, adapter.go:530-533, run.go:3790-3793.
- Follow-up: `checkEngramReachable` (internal/cli/doctor.go) warns on native-only Pi hosts with a remedy (`gentle-ai sync`) that cannot clear it.
