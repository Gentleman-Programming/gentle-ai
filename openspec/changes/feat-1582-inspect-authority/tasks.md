# Tasks: feat-1582-inspect-authority

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~360–390 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr-default |
| Chain strategy | N/A |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: N/A
400-line budget risk: Low

## Phase 1: Infrastructure (types & helpers, NO walker logic)

- [x] 1.1 Add `CompactAuthorityInspectionSummary` struct to `internal/reviewtransaction/compact_store.go` after line 513. Fields: `TotalEdges int`, `ValidEdges int`, `InvalidEdges int`. Mirrors `status.go:77` pattern.
  - **Files**: `internal/reviewtransaction/compact_store.go`
  - **Acceptance**: Package compiles; `go vet ./internal/reviewtransaction/` clean.
  - **Lines**: ~5
  - **Citation**: Design §Component Responsibilities / Summary table

- [x] 1.2 Add `CompactAuthorityInspectionEdge` struct to `internal/reviewtransaction/compact_store.go` (same location). Fields: `PredecessorLineageID`, `PredecessorRevision`, `SuccessorLineageID`, `SuccessorRevision` (all `string`), `AnomalyClass string`, `ValidationError string`.
  - **Files**: `internal/reviewtransaction/compact_store.go`
  - **Acceptance**: Struct fields match spec JSON keys; `go vet` clean.
  - **Lines**: ~8
  - **Citation**: Design §Edge field table; spec R9

- [x] 1.3 Add `InspectionDiagnostic` struct to `internal/reviewtransaction/compact_store.go` (same location). Fields: `Code string`, `Path string`, `Message string`. Mirrors `AuthorityInventoryDiagnostic` (`status.go:77`).
  - **Files**: `internal/reviewtransaction/compact_store.go`
  - **Acceptance**: Struct compileable; `go vet` clean.
  - **Lines**: ~5
  - **Citation**: Design §InspectionDiagnostic; spec R5

- [x] 1.4 Add `CompactAuthorityInspection` struct to `internal/reviewtransaction/compact_store.go` (same location). Fields: `Summary CompactAuthorityInspectionSummary`, `Edges []CompactAuthorityInspectionEdge`, `Diagnostics []InspectionDiagnostic`. JSON keys in this order per spec R9.
  - **Files**: `internal/reviewtransaction/compact_store.go`
  - **Acceptance**: Struct compiles; field order is `Summary`, `Edges`, `Diagnostics`.
  - **Lines**: ~8
  - **Citation**: Design §Component Responsibilities; spec R9

## Phase 2: Implementation (walker + CLI registration, RED-first)

- [x] 2.1 RED: Write `TestInspectCompactAuthorityEmpty` in `internal/reviewtransaction/compact_inspect_test.go` (new file). `t.TempDir()` with no compact stores. Asserts `Summary{0,0,0}`, `Edges=nil`, `Diagnostics=nil`. Uses `initSnapshotRepo` (`snapshot_test.go:990`).
  - **Files**: `internal/reviewtransaction/compact_inspect_test.go` (new)
  - **Acceptance**: `go test ./internal/reviewtransaction/ -run TestInspectCompactAuthorityEmpty -v` fails with "undefined: InspectCompactAuthority".
  - **Lines**: ~25
  - **Citation**: Spec S1; Design §Test Layout

- [x] 2.2 RED: Write `TestInspectCompactAuthorityAllValid` in `internal/reviewtransaction/compact_inspect_test.go`. Two-record linear chain A→B via `newCompactTestState` + `RecoveryEscalated`. Asserts `total=1`, `invalid=0`, `Edges=nil`.
  - **Files**: `internal/reviewtransaction/compact_inspect_test.go`
  - **Acceptance**: Fails compile until `InspectCompactAuthority` exists.
  - **Lines**: ~30
  - **Citation**: Spec S2

- [x] 2.3 RED: Write `TestInspectCompactAuthorityUnchangedTarget` — reuses `poisonedRecoveryFixture` (`compact_reconcile_test.go:20`). Asserts 1 edge, `AnomalyClass=="unchanged_target"`, `ValidationError` starts with `"escalated recovery successor target has not changed"`.
  - **Files**: `internal/reviewtransaction/compact_inspect_test.go`
  - **Acceptance**: Fails until sentinel classification is wired.
  - **Lines**: ~25
  - **Citation**: Spec S3; proposal §Testing approach row 3

- [x] 2.4 RED: Write `TestInspectCompactAuthorityMalformedAuthorization` — reuses `preContractRecoveryFixture(t, repo, preContractFixtureAuthorization, nil)` (`compact_reconcile_test.go:639`). Asserts 1 edge, `AnomalyClass=="malformed_recovery_authorization"`.
  - **Files**: `internal/reviewtransaction/compact_inspect_test.go`
  - **Acceptance**: Fails until sentinel classification is wired.
  - **Lines**: ~25
  - **Citation**: Spec S4; proposal §Testing approach row 4

- [x] 2.5 RED: Write `TestInspectCompactAuthorityCombinedAnomalies` — reuses `combinedRecoveryFixture` (`compact_reconcile_test.go:82`). Asserts 1 edge, `AnomalyClass=="unchanged_target,malformed_recovery_authorization"`.
  - **Files**: `internal/reviewtransaction/compact_inspect_test.go`
  - **Acceptance**: Fails until combined sentinel classification is wired.
  - **Lines**: ~25
  - **Citation**: Spec S5; design §Validation Flow

- [x] 2.6 GREEN: Implement `InspectCompactAuthority(ctx context.Context, repo string) (CompactAuthorityInspection, error)` in `internal/reviewtransaction/compact_store.go` after line 513. Reuses `DiscoverCompactStores` (`compact_store.go:565`), calls `validateCompactRecoveryEdge` (`compact_store.go:318`) per edge, classifies via `errors.Is` against `errCompactRecoveryTargetUnchanged` (`compact_store.go:42`) and `errCompactRecoveryAuthorizationInexact` (`compact_store.go:47`), collects load errors as `InspectionDiagnostic{Code:"load_failure"}`, skips missing predecessors with `Code:"missing_predecessor"`. Sorts edges by `successor.lineage asc, successor.revision asc`; sorts diagnostics by `path asc`. Returns empty report on no stores (mirrors `status.go:105`).
  - **Files**: `internal/reviewtransaction/compact_store.go`
  - **Acceptance**: All Phase 2 RED tests pass; `go test ./internal/reviewtransaction/ -run TestInspectCompactAuthority -v` green.
  - **Lines**: ~80
  - **Citation**: Design §InspectCompactAuthority pseudocode; spec R1–R4, R7; proposal Approach §1

- [x] 2.7 RED: Write `TestInspectCompactAuthorityMultipleInvalid` — two independent invalid successors under one predecessor. Asserts `invalid=2`, both entries present, ordered by successor lineage asc then revision asc.
  - **Files**: `internal/reviewtransaction/compact_inspect_test.go`
  - **Acceptance**: Fails until ordering is implemented.
  - **Lines**: ~30
  - **Citation**: Spec S6; design §Determinism Strategy

- [x] 2.8 RED: Write `TestInspectCompactAuthorityDeterminism` — calls `InspectCompactAuthority` twice on same fixture; asserts `bytes.Equal(report1JSON, report2JSON)`. Snapshot bytes via `testdata/inspect_authority_determinism.golden` using `os.ReadFile` comparison.
  - **Files**: `internal/reviewtransaction/compact_inspect_test.go`, `internal/reviewtransaction/testdata/inspect_authority_determinism.golden` (new)
  - **Acceptance**: Fails if JSON has non-deterministic field ordering or unstable sort.
  - **Lines**: ~25
  - **Citation**: Spec S7; design §Determinism Strategy; go-testing SKILL.md

- [x] 2.9 RED: Write `TestInspectCompactAuthorityReadOnlyInvariant` — uses `poisonedRecoveryFixture`. Snapshots `review-state.json` bytes and mtime via `os.Stat` before; after `InspectCompactAuthority` returns, asserts byte-equal and mtime-equal. Verifies R1.
  - **Files**: `internal/reviewtransaction/compact_inspect_test.go`
  - **Acceptance**: Fails if any file is mutated.
  - **Lines**: ~25
  - **Citation**: Spec S8; proposal R1

- [x] 2.10 RED: Write `TestInspectCompactAuthorityLoadError` — one lineage dir with non-JSON `review-state.json` plus one valid lineage. Asserts 1 diagnostic with `Code=="load_failure"`, valid lineage still counted, no panic, no abort.
  - **Files**: `internal/reviewtransaction/compact_inspect_test.go`
  - **Acceptance**: Fails if load error aborts or is silently dropped.
  - **Lines**: ~25
  - **Citation**: Spec S9; design §Failure Modes row 1

- [x] 2.11 GREEN: Create `internal/cli/review_inspect.go` with `RunReviewInspectAuthority(args []string, stdout io.Writer) error`. Mirrors `RunReviewReconcileAuthority` (`review_reconcile.go:18-60`): `newReviewFlagSet("review inspect-authority", stdout, ...)` (`review.go:32`), `--cwd` flag (`review_reconcile.go:20`), `parseReviewFlags` (`review.go:49`), reject positional args, resolve repo root, call `InspectCompactAuthority`, return non-zero if `len(report.Diagnostics)>0`, else `encodeReviewJSON(stdout, report)` (`review.go:568`).
  - **Files**: `internal/cli/review_inspect.go` (new)
  - **Acceptance**: Compiles; `go vet ./internal/cli/` clean.
  - **Lines**: ~45
  - **Citation**: Design §RunReviewInspectAuthority; spec R6

- [x] 2.12 RED: Write `TestRunReviewInspectAuthorityDispatch` in `internal/cli/review_inspect_test.go` (new). Calls `RunReviewInspectAuthority([]string{"inspect-authority", "--cwd", repo}, &buf)`. Asserts `err==nil`, stdout parses into `CompactAuthorityInspection` with top-level keys exactly `summary`, `edges`, `diagnostics`.
  - **Files**: `internal/cli/review_inspect_test.go` (new)
  - **Acceptance**: Fails compile until `RunReviewInspectAuthority` exists.
  - **Lines**: ~25
  - **Citation**: Spec S10; design §Test Layout / CLI

- [x] 2.13 RED: Write `TestRunReviewInspectAuthorityCwdResolution` — fixture in `t.TempDir()`, `--cwd <tmp>`, asserts exit 0 and parseable JSON. Mirrors `review_reconcile_test.go` dispatch pattern.
  - **Files**: `internal/cli/review_inspect_test.go`
  - **Acceptance**: Fails if cwd resolution is wrong.
  - **Lines**: ~25
  - **Citation**: spec S10

- [x] 2.14 GREEN: Register `inspect-authority` dispatch case in `internal/cli/review_facade.go:320-321`. Insert `case "inspect-authority": return RunReviewInspectAuthority(args[1:], stdout)` after `reconcile-authority` case.
  - **Files**: `internal/cli/review_facade.go`
  - **Acceptance**: `go test ./internal/cli/ -run TestRunReviewInspectAuthorityDispatch -v` passes.
  - **Lines**: ~3
  - **Citation**: Design §CLI Dispatch; proposal Scope decision 2

- [x] 2.15 GREEN: Add `inspect-authority` to usage string at `internal/cli/review_facade.go:229`. Append to the existing pipe-separated list inside the `fmt.Fprintln` string. One-line edit.
  - **Files**: `internal/cli/review_facade.go`
  - **Acceptance**: `gentle-ai review inspect-authority --help` prints usage without panic.
  - **Lines**: ~1
  - **Citation**: Design §CLI Dispatch / Usage line; proposal §Registration

## Phase 3: Tests & Verification

- [x] 3.1 Run `go test ./internal/reviewtransaction/ -run TestInspectCompactAuthority -v` and confirm all 9 scenarios (S1–S9) pass.
  - **Command**: `go test ./internal/reviewtransaction/ -run TestInspectCompactAuthority -v`
  - **Acceptance**: All sub-tests pass; no failures.
  - **Lines**: 0 (verification only)
  - **Citation**: All spec scenarios

- [x] 3.2 Run `go test ./internal/cli/ -run TestRunReviewInspectAuthority -v` and confirm CLI tests pass.
  - **Command**: `go test ./internal/cli/ -run TestRunReviewInspectAuthority -v`
  - **Acceptance**: All CLI inspect tests pass.
  - **Lines**: 0

- [x] 3.3 Run full suite `go test ./...` and confirm green. Run `go vet ./...` clean. Run `gofmt -l` and confirm no changed files.
  - **Commands**: `go test ./... && go vet ./... && gofmt -l`
  - **Acceptance**: All three commands report zero issues.
  - **Lines**: 0

- [x] 3.4 Manual smoke: create a synthetic compact store in `t.TempDir()` with one valid edge, one `unchanged_target`-only edge, and one combined-anomaly edge. Run `gentle-ai review inspect-authority --cwd <tmp>` and verify JSON keys are exactly `summary`, `edges`, `diagnostics` (in that order), `anomaly_class` values match canonical strings, and `compact_reconcile.go:221` is byte-identical to `origin/main`.
  - **Command**: `gentle-ai review inspect-authority --cwd <tmp>` (synthetic repo)
  - **Acceptance**: JSON parses with correct key order; combined edge `anomaly_class=="unchanged_target,malformed_recovery_authorization"`; gate unchanged.
  - **Lines**: 0
  - **Citation**: spec R8, R9; proposal §Gate intact

## Phase 4: Docs & Artifact Finalization

- [x] 4.1 Verify usage string in `review_facade.go:229` lists `inspect-authority` and no extra flags appear. No other documentation changes.
  - **Files**: `internal/cli/review_facade.go`
  - **Acceptance**: Usage line correct; no unrelated changes.
  - **Lines**: 0

- [x] 4.2 Add `openspec/changes/feat-1582-inspect-authority/` files (`proposal.md`, `spec.md`, `design.md`, `tasks.md`) to the change folder — already done by respective SDD phases. No new docs invented.
  - **Files**: `openspec/changes/feat-1582-inspect-authority/` (already in place)
  - **Acceptance**: All four SDD artifacts present.
  - **Lines**: 0

## Acceptance

The implementation is complete when:

1. All 9 spec scenarios (S1–S9) pass as Go tests in `internal/reviewtransaction/compact_inspect_test.go` and `internal/cli/review_inspect_test.go`.
2. `go test ./...` is green across the entire repository.
3. `go vet ./...` is clean with zero warnings.
4. `gofmt -l` lists no changed files.
5. `compact_reconcile.go:221` is byte-identical with `origin/main` (gate untouched).
6. `gentle-ai review inspect-authority --cwd <synthetic>` emits JSON with top-level keys exactly `summary`, `edges`, `diagnostics` (in that order), and canonical `anomaly_class` strings including the combined joined form.
7. JSON output is byte-identical across two consecutive runs against the same synthetic inventory (determinism, spec R3).
