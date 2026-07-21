# Design: Inspect Authority (read-only edge enumeration, #1582)

## Context & Forces

`ReconcileInvalidRecoveryEdge` (`internal/reviewtransaction/compact_reconcile.go:78`) is gated at line 221 by `compactAuthorityLeaves`, which aborts on the first invalid edge (`internal/reviewtransaction/compact_store.go:487-488`). That gate is what #1452 plans to relax for batch reconciliation — but no caller can relax it safely until an enumeration surface exists that returns every invalid edge ahead of time.

This change is the prerequisite discovery surface. It is purely additive: a new exported walker and a new CLI subcommand, both read-only. The shared validation function `validateCompactRecoveryEdge` (`compact_store.go:318`) stays the single source of truth for per-edge classification, fed by the same two sentinels (`compact_store.go:42`, `compact_store.go:47`) that `compact_reconcile.go:142` and `compact_reconcile.go:169` already use.

Forces to balance:

- **Read-only invariant (R1, R8).** The walker must not mutate any file the strict gates reach (`review status`, `review start`, negotiated `review validate`). Touching `compactAuthorityLeaves` semantics would break R8.
- **Strict gate untouched (R8, locked).** The gate at `compact_reconcile.go:221` stays intact; relaxing it is #1452's territory.
- **Wire-format alignment with #1465.** `reconcile-authority` already accepts `"unchanged_target,malformed_recovery_authorization"` as the joined class (`compact_reconcile.go:17`, used at `compact_reconcile.go:61-64`). The enumerator must emit the same strings so #1452 can consume its output directly.
- **Determinism (R3).** Two runs against an unchanged inventory must produce byte-identical JSON. Sorting must happen after collection, not during, because intermediate collection order is not stable.
- **No parallel classification (R7).** Classification rides exclusively on `errors.Is` against the two sentinels; no new sentinels, no parallel validity predicates.

## Architecture Overview

```
gentle-ai review inspect-authority --cwd <repo>
            │
            ▼
RunReviewInspectAuthority(args, stdout)            internal/cli/review_inspect.go (NEW)
   │  - newReviewFlagSet (review.go:32)
   │  - --cwd binding via flags.String (mirrors review_reconcile.go:20)
   │  - encodeReviewJSON (review.go:568)
   ▼
InspectCompactAuthority(ctx, repo)                  internal/reviewtransaction/compact_store.go (ADD)
   │  - DiscoverCompactStores (compact_store.go:565)
   │  - validateCompactRecoveryEdge (compact_store.go:318)  ◄── shared source of truth
   │  - errors.Is vs errCompactRecoveryTargetUnchanged (compact_store.go:42)
   │  - errors.Is vs errCompactRecoveryAuthorizationInexact (compact_store.go:47)
   ▼
CompactAuthorityInspection{ Summary, Edges, Diagnostics }
   │
   ▼  (sorted: edges by successor.lineage asc, then successor.revision asc)
JSON: { "summary": {...}, "edges": [...], "diagnostics": [...] }    top-level keys in this order
```

`compactAuthorityLeaves` (`compact_store.go:474-513`) and its callers — `CompactAuthorityLeaves`, `RecoverCompactAuthority:235`, `StartCompactAuthority:648`, `explicitCompactSuccessor:781`, `ReconcileInvalidRecoveryEdge:221` — are **not** touched. The new walker runs in parallel; the strict path keeps its first-error semantics and the gate stays closed.

## Component Responsibilities

### `InspectCompactAuthority` — new, in `internal/reviewtransaction/compact_store.go`

Add after `CompactAuthorityLeaves` ends at `compact_store.go:472`. Pure walker, no I/O beyond reading already-loaded records.

```go
// IN-MEMORY DESIGN — DO NOT IMPLEMENT IN THIS PHASE
func InspectCompactAuthority(ctx context.Context, repo string) (CompactAuthorityInspection, error)
```

Body, in pseudocode:

1. `stores, err := DiscoverCompactStores(ctx, repo)` (returns empty slice for missing v2 root per `compact_store.go:573-575`).
2. For each store, `record, loadErr := store.Load()`. On `loadErr != nil`, append an `InspectionDiagnostic{Code: "load_failure", Path: store.Dir, Message: loadErr.Error()}` and `continue`. Do not abort.
3. Iterate every record whose `record.State.Recovery != nil`. For each successor record `s`:
   - `predecessor := records[s.State.Recovery.PredecessorLineageID]` (missing predecessor → diagnostic with code `"missing_predecessor"`, do not count the edge).
   - `edgeErr := validateCompactRecoveryEdge(predecessor, s.State)`.
   - If `edgeErr == nil` → `valid_edges++`.
   - If `edgeErr != nil` → classify via §"Validation Flow"; `invalid_edges++`.
4. Sort `edges` by `(successor.lineage asc, successor.revision asc)`. Sort `diagnostics` by `path asc` (mirrors `sortAuthorityReport` at `status.go:448-456`).
5. Return `CompactAuthorityInspection{Summary, Edges, Diagnostics}`.

It returns `error` only for repository-root resolution failures (`reviewAuthorityRoot`); never for per-edge or per-store errors.

### `CompactAuthorityInspection` (Go shape — design only)

| Field | Type | Notes |
|---|---|---|
| `Summary` | `CompactAuthorityInspectionSummary` | Nested object with `total_edges`, `valid_edges`, `invalid_edges`. |
| `Edges` | `[]CompactAuthorityInspectionEdge` | Invalid edges only, sorted. |
| `Diagnostics` | `[]InspectionDiagnostic` | Load failures, sorted by path. |

| Summary field | Type | Notes |
|---|---|---|
| `TotalEdges` | `int` | `ValidEdges + InvalidEdges`. |
| `ValidEdges` | `int` | Count of edges where `validateCompactRecoveryEdge` returned `nil`. |
| `InvalidEdges` | `int` | `len(Edges)`. |

| Edge field | Type | Notes |
|---|---|---|
| `PredecessorLineageID` | `string` | From `predecessor.State.LineageID`. |
| `PredecessorRevision` | `string` | From `predecessor.Revision`. |
| `SuccessorLineageID` | `string` | From `successor.State.LineageID`. |
| `SuccessorRevision` | `string` | From `successor.Revision`. |
| `AnomalyClass` | `string` | One of the three canonical strings (see §"Validation Flow"). |
| `ValidationError` | `string` | The `edgeErr.Error()` string, byte-preserved. |

`InspectionDiagnostic` mirrors `AuthorityInventoryDiagnostic` (`status.go:77-80`) plus a stable `code` field so callers can branch:

| Diagnostic field | Type | Notes |
|---|---|---|
| `Code` | `string` | Stable identifier: `"load_failure"`, `"missing_predecessor"`, etc. |
| `Path` | `string` | The lineage directory (`status.go:77` precedent). |
| `Message` | `string` | Human-readable detail (mirrors `status.go:78` `Problem`). |

### `RunReviewInspectAuthority` — new, in `internal/cli/review_inspect.go`

```go
// IN-MEMORY DESIGN — DO NOT IMPLEMENT IN THIS PHASE
func RunReviewInspectAuthority(args []string, stdout io.Writer) error
```

Mirrors `RunReviewReconcileAuthority` (`internal/cli/review_reconcile.go:18-60`). The flag set is the smallest possible: one `cwd` flag bound via `flags.String("cwd", ".", "repository path")` — same string as `review_reconcile.go:20`. No positional arguments (R6). No `--contract`, no `--gate`, no negotiated mode.

Body:

1. `flags := newReviewFlagSet("review inspect-authority", stdout, "<description>")` (`review.go:32`).
2. `cwd := flags.String("cwd", ".", "repository path")`.
3. `parseReviewFlags(flags, args)` (`review.go:49`); honor `reviewHelpRequested(args)` (`review.go:59`).
4. Reject any positional arg (`flags.NArg() != 0`).
5. Resolve root: `root, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)`.
6. `report, err := reviewtransaction.InspectCompactAuthority(ctx, root)`.
7. If `len(report.Diagnostics) > 0` → emit the JSON anyway, then return a non-zero sentinel (per R5, "SHOULD exit non-zero").
8. `return encodeReviewJSON(stdout, report)` (`review.go:568`).

The result is the `CompactAuthorityInspection` value — no wrapping struct. `encodeReviewJSON` already uses `SetIndent("", "  ")` (`review.go:569-570`), giving the deterministic two-space indented shape R3 needs for byte-equality tests.

## Validation Flow

Per-edge classification (pseudocode — design only):

```
classifyEdge(predecessor, successor):
    err := validateCompactRecoveryEdge(predecessor, successor)
    if err == nil:
        return VALID, ""

    unchanged    := errors.Is(err, errCompactRecoveryTargetUnchanged)
    authorization := errors.Is(err, errCompactRecoveryAuthorizationInexact)

    switch {
    case unchanged && authorization:
        return INVALID, "unchanged_target,malformed_recovery_authorization"
    case unchanged:
        return INVALID, "unchanged_target"
    case authorization:
        return INVALID, "malformed_recovery_authorization"
    default:
        # Non-classified edge: keep the error message, mark as invalid with the
        # closest single sentinel when only one is present; otherwise use the
        # joined string and surface the unclassified error verbatim. This
        # mirrors compact_reconcile.go:155-167's "combined if both proofs can
        # be reconstructed" tolerance.
        return INVALID, "<unclassified-error-as-validation_error>"
    }
```

Both sentinel checks ride on `errors.Is` (R4, R7). The joined class is a single string literal `"unchanged_target,malformed_recovery_authorization"` — the same constant `compactCombinedRecoveryAnomalies` defined at `compact_reconcile.go:17`. The walker never mutates the store (R1), never short-circuits on the first error (R2), and never invents new sentinel errors (R7).

## Determinism Strategy

Sort keys come from already-stable fields:

- Primary: `successor.lineage` (`CompactRecord.State.LineageID`, a validated kebab-case identifier set at start-time).
- Tiebreaker: `successor.revision` (`CompactRecord.Revision`, a `sha256:` digest).

`DiscoverCompactStores` already sorts its output by `lineageID` (`compact_store.go:599`), and the walker iterates the map in lineage order to feed the records slice. The collector builds `edges` and `diagnostics` slices during the walk, then `sort.Slice` runs once at the end.

Two concurrent callers running the same inspection would interleave map iteration and could differ on map order — but the CLI registers this command under `RunReview(args, stdout)` (`review_facade.go:294`), which is invoked by `runReviewCommandContext` (`review_facade.go:277`) inside a single goroutine. Concurrent inspect calls are not supported; the cobra command is the single concurrency boundary. R3 is satisfied within that boundary. The orchestrator MUST NOT invoke `InspectCompactAuthority` from a goroutine; the design does not promise goroutine safety.

## CLI Dispatch

Two literal edits in `internal/cli/review_facade.go`. Neither file is opened in this phase — these are descriptions of the only changes the apply phase will make.

1. **Usage line (`review_facade.go:229`)** — append `inspect-authority` to the existing list inside the Fprintln string:

   ```text
   gentle-ai review <capabilities|start|finalize|validate|status|invalidate|abandon|recover|reclaim|reconcile-authority|inspect-authority|dispose-result|quarantine-legacy|quarantine-legacy-fix-scope|repair-legacy-alias|schema|bind-sdd> [flags]
   ```

2. **Dispatch case (`review_facade.go:320-321`)** — insert immediately after the `reconcile-authority` case, mirroring its shape exactly:

   ```go
   case "inspect-authority":
       return RunReviewInspectAuthority(args[1:], stdout)
   ```

No new dispatch entry in `runReviewCommandContext` (which routes the negotiated/timeout path; inspect-authority is non-negotiated and uses the synchronous path via `runReviewCommand`, identical to `reconcile-authority`).

## Test Layout

### New: `internal/reviewtransaction/compact_inspect_test.go`

One test function per Scenario (S1-S9). All use `t.TempDir()` and `initSnapshotRepo(t)` (`snapshot_test.go:990`).

| Test | Scenario | Fixture | Assertion |
|---|---|---|---|
| `TestInspectCompactAuthorityEmpty` | S1 | Empty `t.TempDir()` | `Summary{0,0,0}`, `Edges=[]`, `Diagnostics=[]`. |
| `TestInspectCompactAuthorityAllValid` | S2 | Two-record linear chain via `newCompactTestState` + `RecoveryEscalated` | `total=1`, `invalid=0`, `Edges=[]`. |
| `TestInspectCompactAuthorityUnchangedTarget` | S3 | `poisonedRecoveryFixture` (`compact_reconcile_test.go:20`) | 1 edge, `AnomalyClass == "unchanged_target"`. |
| `TestInspectCompactAuthorityMalformedAuthorization` | S4 | `preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)` (`compact_reconcile_test.go:639`, constant at `:675`) | 1 edge, `AnomalyClass == "malformed_recovery_authorization"`. |
| `TestInspectCompactAuthorityCombinedAnomalies` | S5 | `combinedRecoveryFixture` (`compact_reconcile_test.go:82`) | 1 edge, `AnomalyClass == "unchanged_target,malformed_recovery_authorization"`. |
| `TestInspectCompactAuthorityMultipleInvalid` | S6 | Two independent invalid successors under one predecessor | 2 edges, ordered by successor lineage asc, then revision asc. |
| `TestInspectCompactAuthorityDeterminism` | S7 | Two-inventory fixture presented to two calls | `bytes.Equal(report1JSON, report2JSON)`. Snapshot the bytes into a `testdata/inspect_authority_determinism.golden` file; compare via `os.ReadFile`. |
| `TestInspectCompactAuthorityReadOnlyInvariant` | S8 | Any fixture (use `poisonedRecoveryFixture`) | Snapshot `review-state.json` bytes + `mtime` via `os.Stat` before; verify byte-equal + mtime-equal after. Verifies R1. |
| `TestInspectCompactAuthorityLoadError` | S9 | One lineage dir with a non-JSON `review-state.json` plus one valid lineage | 1 diagnostic, code `"load_failure"`, valid lineage still counted, no panic, no abort. |

All sub-tests use `t.Run(tt.name, ...)` per the `go-testing` skill. No real home directory, no network, no skipped integration cases — pure `go test`.

### New: `internal/cli/review_inspect_test.go`

Covers R6 (CLI registration) and S10 (S10 is CLI dispatch + JSON shape + key order). Two tests:

- `TestRunReviewInspectAuthorityDispatch` — call `RunReview([]string{"inspect-authority", "--cwd", repo}, &buf)`; assert `err == nil`, `stdout` parses into the `CompactAuthorityInspection` shape with top-level keys exactly `summary`, `edges`, `diagnostics`.
- `TestRunReviewInspectAuthorityCwdResolution` — set up the fixture in `t.TempDir()`, call with `--cwd <tmp>`, assert exit 0 and parseable JSON. Mirrors the dispatch test from `review_reconcile_test.go`.

A third optional test (`TestRunReviewInspectAuthorityDiagnosticExitNonZero`) pins the R5 "SHOULD exit non-zero on diagnostic" behavior using the S9 fixture and asserting `err != nil`.

## Failure Modes & Mitigations

| Mode | Behavior | Mitigation |
|---|---|---|
| Store unloadable | `loadErr != nil` for one store | Diagnostic appended with code `"load_failure"`; walker continues with remaining stores. R5. |
| Partial read (some valid, some corrupt) | Mid-walk corruption | Same as above — diagnostic and continue. Valid edges still counted. |
| Single-record corruption after the walk began | Any `loadErr` after successful walk start | Caught by the per-store try/continue; never aborts. |
| Missing predecessor (dangling `Recovery.PredecessorLineageID`) | Not an edge validation error in this walker | Diagnostic code `"missing_predecessor"`; edge not counted (no predecessor to validate against). Avoids false classification. |
| `ctx.Err()` cancellation mid-walk | Rare under cobra | Repo-root resolution already returns context errors (`reviewAuthorityRoot`). Per-record loads are synchronous reads; cancellation surfaces as the loader's `ctx.Err()`. Acceptable. |
| Empty inventory (no v2 dir) | `DiscoverCompactStores` returns `[]` (`compact_store.go:573-575`) | Walker returns empty report (mirrors `status.go:105` empty-init pattern). Not an error. |

## Risks

| Risk | Likelihood | Mitigation tied to code location |
|---|---|---|
| Graph-walking duplication vs. `compactAuthorityLeaves` (`compact_store.go:474-513`) | Medium | Both share `validateCompactRecoveryEdge` (`compact_store.go:318`) as the per-edge source of truth. Any future tightening or loosening of that function applies to both. Explicit lock: this walker does NOT add cycle/fork checks — those remain in `compactAuthorityLeaves`. |
| API surface growth (`InspectCompactAuthority` is a new exported name) | Low | Single new export in `compact_store.go`; one new CLI command. Both additive. No callers depend on them yet. #1452 will be the first consumer. |
| Load-error diagnostic scope ambiguity (successor vs. predecessor path) | Low | `InspectionDiagnostic.Path` carries the failing lineage directory; `Code` distinguishes `"load_failure"` from `"missing_predecessor"`. Test scenario 9 pins the load-failure case. |
| Ordering drift if `successor.lineage` is not unique within the graph | Low | Tiebreaker on `successor.revision` (`compact_store.go:511` precedent for `compactAuthorityLeaves` line sort). `revision` is a `sha256:` digest and unique per record. |
| Combined-class classifier misread as two entries | Low | Single joined string literal `"unchanged_target,malformed_recovery_authorization"`; mirrors `compact_reconcile.go:17`. Documented in §"Validation Flow" and pinned by scenario S5. |
| Concurrent-call determinism claim overstated | Low | Cobra command is the only invocation site. The design explicitly does not promise goroutine safety; concurrent callers are out of contract. |
| Regression on `compact_reconcile.go:221` gate via shared classifier drift | Low | Gate uses `compactAuthorityLeaves` directly (`compact_reconcile.go:221`), not the new walker. The walker only adds surface; it cannot influence the strict path. Verified by R8 and the locked constraint. |

## Migration / Rollout

No data migration. No feature flag. No phased rollout.

Rollback (single commit, safe — no schema persisted):

1. Revert the new function `InspectCompactAuthority` and type definitions from `internal/reviewtransaction/compact_store.go`.
2. Delete `internal/reviewtransaction/compact_inspect_test.go`.
3. Delete `internal/cli/review_inspect.go` (entire file).
4. Delete `internal/cli/review_inspect_test.go`.
5. Remove the two-line edit at `internal/cli/review_facade.go:229` (usage list) and `internal/cli/review_facade.go:320-321` (dispatch case).

No data on disk is written by this change (R1). No audit records, no quarantine residues, no lock files. The gate at `compact_reconcile.go:221` is untouched, so rollback cannot regress `reconcile-authority` behavior. `git revert <merge-sha>` is sufficient.

## Effort Estimate

| Category | Size | Notes |
|---|---|---|
| Implementation (walker + CLI handler + struct) | Medium | One new exported function, one new CLI file, ~150 lines Go total. |
| Tests (walker + CLI) | Medium | 9 walker scenarios + 2-3 CLI scenarios. Reuses `poisonedRecoveryFixture`, `combinedRecoveryFixture`, `preContractRecoveryFixture` from `compact_reconcile_test.go`. |
| Docs / spec / changelog | Small | One new usage string in `review_facade.go:229`; no external API doc beyond the spec. |
| Review | Small | ≤400 line review budget per orchestrator; the diff is concentrated in two files plus tests. |

## Open Questions

None. All four authoritative product decisions are encoded in the proposal, the MUST keywords in the spec are binding, and the locked citations resolve cleanly. Any future deviation belongs in #1452.
