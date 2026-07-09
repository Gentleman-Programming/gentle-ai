# Exploration: Managed Block Footprint Diagnostic for `gentle-ai doctor`

Source issue: #1018 (status:approved)

## Current State

`gentle-ai doctor` lives entirely in `internal/cli/doctor.go` (417 lines) with tests in `internal/cli/doctor_test.go`. Wired from `internal/app/app.go:230-231`:

```go
case "doctor":
    return cli.RunDoctor(context.Background(), stdout)
```

Doctor takes **no CLI arguments today** — unlike `install`/`sync`/`uninstall`/`restore`, which all forward `args[1:]` to a `Parse*Flags` function. `RunDoctor(ctx, w)` runs four checks in a fixed sequence and appends `CheckResult` values into a flat `DoctorReport.Checks []CheckResult`:

1. `checkToolBinaries(pathDirs)` → `[]CheckResult` (PATH resolution + duplicate-binary detection per tool in `knownTools`)
2. `checkStateJSON(homeDir)` → validates `~/.gentle-ai/state.json`, cross-references `state.InstalledAgents` against a **hardcoded switch** `agentConfigDir(homeDir, agentID)` (duplicates logic already in `agents.Adapter`/`agents.Registry`)
3. `checkEngramReachable()` → HTTP GET to Engram's `/health` endpoint
4. `checkDiskSpace(homeDir)` → free-space check via `storage.AvailableBytes`

No `Check` interface or registration mechanism — checks are plain functions, called inline and appended to a slice in `RunDoctor`. Rendering (`renderDoctorReport`) is plain `fmt.Fprintf` text output (`[ok]`/`[!!]`/`[xx]` icons) — **not** Bubbletea/lipgloss TUI.

Overridable package-level vars (`lookPathFn`, `availableBytesFn`, `osUserHomeDirDoctor`, `doctorGOOS`, `httpGetFn`, etc.) are the established seam for unit-testing doctor checks without touching real filesystem/network — tests reassign these vars and `defer` restore them.

## Managed Block Mechanism (install/sync)

Blocks are tagged `<!-- gentle-ai:SECTION_ID --> ... <!-- /gentle-ai:SECTION_ID -->`, implemented in `internal/components/filemerge/section.go`:
- `openMarker`/`closeMarker` build marker strings.
- `InjectMarkdownSection(existing, sectionID, content string) string` replaces-or-appends **one known** sectionID at a time, plus orphan-marker repair (`stripOrphanMarkers`).
- **No existing "list/parse all sections in a file" utility** — net-new for this feature.

Section IDs are string literals scattered across components (no central enum/registry). Confirmed IDs: `persona`, `engram-protocol`, `sdd-orchestrator`, `strict-tdd-mode`, `trigger-rules`, `codegraph-guidance`. `internal/components/uninstall/service.go` builds the closest thing to a canonical list, but inline per-component, not exported.

Per-agent instruction-file paths come from `agents.Adapter.SystemPromptFile(homeDir string) string` (`internal/agents/interface.go`), resolved via `agents.NewDefaultRegistry()` (`internal/agents/factory.go`). **The new check should use this**, not a hand-rolled path switch like `checkStateJSON`'s `agentConfigDir` (an existing duplication smell).

"Configured agents" = `state.Read(homeDir).InstalledAgents []string` (`internal/state/state.go`) — same source `checkStateJSON` already reads.

## Token Estimation

No existing utility. A chars/4-style heuristic is net-new, small, pure — no conflicts.

## Test/Flag Patterns to Follow

- Flag parsing precedent: `sync.go`/`install.go`/`uninstall.go`/`restore.go` use `flag.NewFlagSet(name, flag.ContinueOnError)` + typed `*Flags` struct + `Parse*Flags(args []string) (*Flags, error)`. Doctor has none of this today — `--footprint` would be doctor's first flag, following the same shape.
- Test pattern: table-driven Go tests + var-swap-and-defer-restore for fs/network seams. Plain stdlib `testing`, no mocking framework.
- Per `openspec/config.yaml`: `go test ./...`, `go vet ./...`, `gofmt`; no golangci-lint config.

## Affected Areas

- `internal/cli/doctor.go` — new `checkManagedBlockFootprint(...)` check, wired into `RunDoctor`, extended rendering.
- `internal/cli/doctor_test.go` — new table-driven tests.
- `internal/components/filemerge/section.go` (or new sibling `section_scan.go`) — new **generic** multi-section marker parser (ID + raw content + line/char span).
- `internal/agents/registry.go` / `factory.go` — reuse `NewDefaultRegistry()` + `Adapter.SystemPromptFile(homeDir)`.
- `internal/state/state.go` — reuse `state.Read(homeDir).InstalledAgents`.
- `internal/app/app.go:230-231` — if a flag is added, `RunDoctor` signature changes from `(ctx, w)` to `(ctx, args, w)`.
- New small token-estimation helper (chars/4 or similar).

## Approaches Considered

1. **Always-on check, no flag** — smallest diff, but per-block detail is verbose for a single `CheckResult.Detail` line.
2. **Always-on compact check + opt-in `--footprint` flag for full breakdown** — matches issue text exactly, keeps default output terse, establishes doctor's first flag following existing `flag.NewFlagSet` pattern. Touches `app.go` call site + tests. **Recommended.**
3. **Standalone `gentle-ai footprint` subcommand** — the issue itself already rejected this (less discoverable than `doctor`). Not recommended.

## Recommendation

**Approach 2.** Always-on compact check (total blocks, agents covered, rough total token estimate; WARN/FAIL only on structural breakage like an orphan/unclosed marker) plus opt-in `--footprint` flag for the full per-agent/per-block table. Reuses `agents.Registry`/`Adapter.SystemPromptFile` (fixing rather than repeating the `agentConfigDir` duplication smell). Requires one net-new generic section-scanner in `filemerge` with natural reuse value beyond this issue (e.g. future orphan/malformed-marker validation).

Estimated shape as a single PR work unit: generic section-scanner (~60-100 lines + tests), doctor check + flag plumbing (~80-120 lines + tests), doctor render extension (~30-50 lines). Comfortably within the 400-line budget as one PR.

## Risks

- **No central registry of section IDs** — scanner MUST be generic (regex/scan for any `<!-- gentle-ai:X -->...<!-- /gentle-ai:X -->` pair), not a hardcoded whitelist, or it rots as soon as a new block is added.
- **`RunDoctor` signature change** (if flag added) touches `doctor_test.go` and `app.go` — must land in the same work unit.
- **Token estimate is inherently approximate** (chars/4 heuristic) — issue itself frames it as "rough," no tokenizer dependency needed.
- **Multi-file agents** — only markdown marker-based `SystemPromptFile` targets are in scope; JSON settings overlays are NOT marker-based and are out of scope for #1018.

## Note

No `.codegraph/` index exists for this repo; exploration used direct Read/Grep/Glob per the CodeGraph fallback clause.
