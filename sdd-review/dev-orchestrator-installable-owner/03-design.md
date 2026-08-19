# Design: dev-orchestrator as Installable, Change-Owning Peer

## Technical Approach

One stdlib-only leaf package, `internal/changeowner`, becomes the single authority for the ownership marker (format, parse, default, refusal error). Both engines import it — `internal/devorchestrator/intent` at write time, `internal/devorchestrator/orchestrator.go` at every subsequent phase-advance, and `internal/sddstatus` at gate time — so neither engine imports the other and the marker grammar cannot drift. Installation adds `dev-orchestrator` as a second `mode: primary` agent by merging an opt-in JSON fragment into the in-memory overlay bytes before the existing single write path, leaving `SDDModeID`, `normalizeSDDMode`, `sdd_mode.go`, `overlayAssetPath`, and the ratified `installer-picker-navigation` predicates untouched.

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|---|---|---|---|
| Marker location | `engine: <id>` line in the first artifact's YAML frontmatter (`explore.md`, else `proposal.md`) | Sidecar `.engine` file; `openspec/config.yaml` | Travels with the artifact `sddstatus` already reads and git already tracks; mirrors the existing `type: greenfield` / `gates: required` line conventions |
| Marker grammar | `regexp` `(?im)^\s*engine:\s*(gentle-orchestrator\|dev-orchestrator)\s*$`; unrecognized value ⇒ typed error, never a guess | YAML unmarshal of the whole frontmatter | Matches `gatesEnabledPattern` (status.go:2045) and `repositoryFieldPattern`; devorchestrator writes non-strict frontmatter, so full YAML parsing would be a new failure mode |
| Ownership authority | New leaf pkg `internal/changeowner` (stdlib only) imported by both engines | devorchestrator importing sddstatus (per `orchestrator_summary_report.md` convergence) | Convergence is explicitly out of scope; a leaf pkg gives one grammar with zero cycle risk (`sddstatus` already pulls `repository`/`reviewtransaction`) |
| Default | `changeowner.Resolve` returns `EngineGentle` when no marker is found — the ONLY place the default lives | Defaulting at each call site | One default, one test; legacy changes are byte-identical by construction |
| Enforcement points | TWO checkpoints, both calling the same `changeowner` predicate: (1) `RouteIntent` at creation/write time (hard refusal, no `MkdirAll`/`WriteFile`); (2) `GenerateContextForAgent` at every phase-advance invocation (read-only `Resolve` check before `context.Build`) | Enforcing only at creation | Creation-time-only leaves a gap: a phase-advance against an already-existing foreign-owned change (e.g. propose→spec) would otherwise proceed unchecked. `GenerateContextForAgent` already gained a strict-registry check in `be3909ab` (`agent.Registry` lookup, `fmt.Errorf("strict enforcement: ...")` on rejection) — this reuses that exact chokepoint and error style, so the ownership check rides the same code path that already runs on every agent invocation, not a new one |
| Peer install opt-in | `Selection.DevOrchestrator bool` mirroring `StrictTDD` (+ `SyncOverrides.DevOrchestrator *bool`, `InjectOptions.DevOrchestrator bool`, `--dev-orchestrator` flag). No new TUI screen — **decision accepted for v1**: TUI opt-in ships as flag/sync-override only; an interactive picker screen would ratify a delta to `installer-picker-navigation` and is deliberately deferred to its own future change, kept out of this one's blast radius | New `SDDModeID` value; new picker screen | A third enum value perturbs the `SDDMode == Multi` ModelPicker predicate; a new screen changes the ratified forward order `Claude→Kiro→Codex→SDDMode→ModelPicker→StrictTDD→OpenCodePlugins→DependencyTree` |
| Fragment merge | `filemerge.MergeJSONObjects(overlayBytes, fragmentBytes)` in memory, immediately after `assets.Read(overlayAssetPath(sddMode))` and BEFORE `inlineOpenCodeSDDPrompts` | A second `mergeJSONFile` call against `opencode.json` | Keeps exactly one atomic settings write, so idempotency, the `Changed` flag, and the step-5 post-check all keep working unmodified |
| Own-artifact overwrite | Same-engine re-route of its own change (e.g. `RouteIntent` called twice for a `dev-orchestrator`-owned change) is ALLOWED and idempotent — resolved by SPEC-003's "Same-engine write proceeds normally" scenario | Refusing all overwrites including same-engine | Only cross-engine writes are a collision risk; same-engine re-routing is normal, expected operation (e.g. re-running intent classification) |

## Data Flow

    RouteIntent ──changeowner.AssertCanWrite(changeRoot, EngineDev)──┐
         │ ok: MkdirAll + stamp `engine: dev-orchestrator`           │ foreign → ErrForeignEngine (no MkdirAll, no write)
         ▼                                                          │
    openspec/changes/<id>/{explore,proposal}.md ◄────────────────────┘
         │
    GenerateContextForAgent ──agent.Registry lookup (be3909ab)──► unregistered agent → reject
         │ registered            │
         └─►changeowner.Resolve(changeRoot)──► foreign → reject ("strict enforcement: change %q is owned by %s")
         │ own/unmarked-default
         ▼
    context.Build(...) proceeds as before
         │
    sddstatus.Resolve ──changeowner.Resolve(changeRoot)──► Status.Engine
         │ Engine == dev-orchestrator → all Dependencies blocked,
         └─► nextRecommended "blocked-foreign-engine" + blockedReason

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/changeowner/owner.go` | Create | `Engine` type, `Parse`, `Resolve(changeRoot)`, `Stamp`, `AssertCanWrite`, `ErrForeignEngine`, `ErrUnknownEngine` |
| `internal/devorchestrator/intent/router.go` | Modify | Call `AssertCanWrite` before `MkdirAll` (line 59-63); refuse existing artifact whose marker is absent/foreign; emit `engine: dev-orchestrator` in the frontmatter next to `type:` (line 82) |
| `internal/devorchestrator/orchestrator.go` | Modify | `GenerateContextForAgent` calls `changeowner.Resolve(changeRoot)` immediately after the existing `agent.Registry` contract lookup (~line 160, right after the `be3909ab` strict-enforcement check); refuses with `strict enforcement: change %q is owned by %s` if foreign, same error style/exit path as the unregistered-agent rejection |
| `internal/sddstatus/status.go` | Modify | `Status.Engine string \`json:"engine,omitempty"\`` ; set after `status.TargetRepositories` (~line 703) and in the Engram path (~line 1117); foreign-engine gate blocks dependencies + `nextRecommended` |
| `internal/sddstatus/status_v1.go` | Modify | Additive `Engine string \`json:"engine,omitempty"\`` on `statusV1` + projection line |
| `internal/assets/opencode/sdd-overlay-devorchestrator.json` | Create | Overrides `agent["dev-orchestrator"]`: `mode: primary`, `hidden: false`, `tools.task: true`, `permission` grants for `dev-*` |
| `internal/components/sdd/inject.go` | Modify | `InjectOptions.DevOrchestrator`; gated `MergeJSONObjects` at line ~463-473 |
| `internal/model/{types.go,selection.go}`, `internal/state/state.go`, `internal/cli/{install.go,sync.go,dryrun.go}`, `internal/app/app.go` | Modify | Thread the bool exactly as `StrictTDD` is threaded (incl. dry-run printing the choice) |

Explicitly NOT touched: `internal/sddstatus/artifact_states.go` — that file is the artifact-readiness key registry (single authority for the v1 projection's required-key set, per the fix for Issue #2346); coupling the `engine:` frontmatter parser to it would recreate the exact drift risk that fix closed. The `changeowner` leaf package is the parser; `status.go`/`status_v1.go` only call into it.

## Interfaces / Contracts

```go
package changeowner

type Engine string
const (
    EngineGentle Engine = "gentle-orchestrator"
    EngineDev    Engine = "dev-orchestrator"
)

// Resolve reads explore.md then proposal.md. No marker anywhere ⇒ EngineGentle.
// An `engine:` line with an unrecognized value ⇒ ErrUnknownEngine (never guessed).
func Resolve(changeRoot string) (Engine, error)

// AssertCanWrite returns ErrForeignEngine when changeRoot exists and is owned by
// another engine. A non-existent changeRoot is writable by anyone.
func AssertCanWrite(changeRoot string, want Engine) error

// Stamp inserts `engine: <want>` into an artifact's frontmatter, idempotently.
func Stamp(frontmatter string, want Engine) string
```

Refusal message (all three call sites — RouteIntent, GenerateContextForAgent, sddstatus gate — same sentence): `change %q is owned by %s; %s must not write to it — ownership is stamped at change creation and is not switchable`.

## Refusal Symmetry

Symmetry is at the *predicate*, not the mechanism, because `RouteIntent`/`GenerateContextForAgent` are the only non-test Go call sites acting on `openspec/changes/<id>` for dev-orchestrator — gentle-orchestrator writes through agent tools, not a Go write path. dev-orchestrator refuses in Go at two points now (creation via `ErrForeignEngine`, and every phase-advance via the `GenerateContextForAgent` check), both with no side effects on refusal. gentle-orchestrator refuses at its gate: `gentle-ai sdd-status` reports `engine: dev-orchestrator`, blocks every dependency, and sets `nextRecommended: blocked-foreign-engine`, which every phase skill already consults before writing — the same level of guarantee every other SDD invariant in the system already relies on, not a new, weaker one. All three call sites share the one `changeowner` predicate, so one fix corrects all of them.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | `Parse`/`Resolve` default, unknown-value refusal, `Stamp` idempotency, explore-over-proposal precedence | table tests in `internal/changeowner` |
| Unit | `Status.Engine` unmarked ⇒ absent; `TestProjectStatusV1FreezesExactLegacyShape` stays green | existing sddstatus fixtures |
| Integration | gentle-authored `proposal.md` + `RouteIntent` ⇒ `ErrForeignEngine` AND file bytes + mtime unchanged | `intent/router_test.go` |
| Integration | `GenerateContextForAgent` refuses foreign-owned change with the strict-enforcement error, proceeds normally for own/unmarked-default change | `orchestrator_test.go` |
| Integration | opt-out install: merged `opencode.json` bytes equal a **golden fixture generated from current `main` BEFORE any `inject.go`/overlay code changes land** (`internal/components/sdd/testdata/golden/opencode-{single,multi}.json`); opt-in: `dev-orchestrator.mode == "primary"`, second inject ⇒ `Changed == false` | `internal/components/sdd/inject_test.go`; golden generation MUST be the first task in `sdd-tasks`, hard-ordered before any task touching `inject.go` or overlay JSON |
| Regression | `TestInstallNavigationRoundTrips`, `TestPickerBackRowRegression`, `pickerFlowSlice` rows unchanged | run as-is; zero edits to those files is the acceptance signal |

## Migration / Rollout

No migration. Historical artifacts are never rewritten. Rollback per layer: delete the fragment + flag (opt-out installs are byte-identical), drop the `omitempty` `Engine` field, revert the guard.

## Resolved Questions (were open, now decided)

- **TUI opt-in home**: RESOLVED — flag/sync-override only for v1 (user-confirmed). A TUI picker screen is deferred to a separate future change that deliberately ratifies a delta to `installer-picker-navigation`, kept out of this change's scope.
- **RouteIntent own-artifact overwrite**: RESOLVED — same-engine re-route is allowed and idempotent, per SPEC-003's "Same-engine write proceeds normally" scenario. Only cross-engine writes are refused.
- **Second enforcement point**: RESOLVED — `GenerateContextForAgent` also checks ownership (see Architecture Decisions / Data Flow / File Changes above), closing the phase-advance gap beyond creation-time-only enforcement.

## Provenance Notes (informational, already reconciled into the decisions above — not open)

- `artifact_states.go` is the artifact-readiness key registry, not a frontmatter parser — confirmed NOT the parser location; `internal/changeowner` is.
- The greenfield `artifactPath` ordering bug is already fixed on disk (`router.go` now computes `artifactPath` after the greenfield reassignment) and is now also covered as SPEC-004 regression guard, not new implementation work.
- `dev-orchestrator` already exists in `sdd-overlay-single.json:272` as `mode: subagent, hidden: true` — the opt-in fragment is an OVERRIDE (must set `hidden: false` explicitly), not a purely new additive key; `sdd-overlay-multi.json` must be checked for the same key before shipping.

---
*This is a read-only review copy exported from Engram (obs #2931) for human approval. The source of truth remains Engram — edits here do not sync back automatically.*
