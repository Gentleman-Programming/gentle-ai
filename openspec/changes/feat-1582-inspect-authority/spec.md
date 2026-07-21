# Inspect Authority Specification

Inspect Authority (#1582) provides a read-only, deterministic enumeration of compact-v2 recovery edges. It is the prerequisite discovery surface for #1452 batch reconciliation: callers must enumerate every invalid edge before any future reconciliation gate is relaxed.

## Requirements

### Requirement: R1 — Read-only behavior
The walker MUST NOT mutate the compact store; every record held before `InspectCompactAuthority` returns MUST still be held afterward. (Proposal: Scope decision 1; Out of scope—mutation.)

### Requirement: R2 — Completeness
Every recovery edge in the inventory MUST be counted exactly once in `valid_edges` or represented exactly once in `edges[]`; `total_edges` MUST equal their sum and `invalid_edges` MUST equal `len(edges)`. (Proposal: JSON output schema—Field contract.)

### Requirement: R3 — Determinism
Two runs against an unchanged inventory MUST produce byte-identical JSON, with invalid edges ordered by successor lineage ascending and successor revision ascending. (Proposal: Scope decision 5; JSON output schema—Ordering.)

### Requirement: R4 — Per-edge anomaly classification
Each `edges[]` entry MUST carry exactly one canonical `anomaly_class`: `unchanged_target`, `malformed_recovery_authorization`, or `unchanged_target,malformed_recovery_authorization`. The combined value MUST be used iff both sentinels fire on that edge. (Proposal: Scope decision 3; Anomaly classification alignment.)

### Requirement: R5 — Diagnostic surface for load errors
When a store cannot be read, the walker MUST append a `diagnostics[]` entry containing at least `code` and `message`, without panicking or aborting the pass. The command SHOULD exit non-zero. (Proposal: Scope decision 4; JSON output schema—Field contract.)

### Requirement: R6 — CLI surface
`gentle-ai review inspect-authority --cwd <repo>` MUST exist, print the Report JSON to stdout, register alongside `RunReviewReconcileAuthority` (`review_facade.go:320-321`) under `review`, and introduce no additional flags in this PR. (Proposal: Scope decision 2; CLI surface.)

### Requirement: R7 — Source-of-truth reuse
The walker MUST classify each edge by calling `validateCompactRecoveryEdge` exactly as `compactAuthorityLeaves` does, with no parallel validation implementation. (Proposal: Approach; Anomaly classification alignment.)

### Requirement: R8 — No relaxation of strict gates
This PR MUST NOT modify `compactAuthorityLeaves` validation semantics or any function reachable from `review status`, `review start`, or negotiated `review validate`; the gate at `compact_reconcile.go:221` MUST remain strict. (Proposal: Out of scope; Rollback plan.)

### Requirement: R9 — JSON shape stability
The Report MUST be a JSON object whose top-level keys, in order, are exactly `summary`, `edges`, and `diagnostics`. `summary` MUST contain `total_edges`, `valid_edges`, and `invalid_edges`; each `edges[]` entry MUST contain predecessor/successor lineage and revision, `anomaly_class`, and `validation_error`; `edges` MUST contain invalid entries only; `diagnostics` MUST contain load diagnostics with at least `code` and `message`. (Proposal: JSON output schema—Field contract.)

## Scenarios

### Scenario: S1 — Empty inventory
- GIVEN an inventory with no recovery edges
- WHEN `InspectCompactAuthority` runs
- THEN all summary counts are `0`, `edges=[]`, and `diagnostics=[]`

### Scenario: S2 — One valid edge
- GIVEN one valid recovery edge
- WHEN the walker runs
- THEN `total_edges=1`, `valid_edges=1`, `invalid_edges=0`, and `edges=[]`

### Scenario: S3 — Unchanged target only
- GIVEN one edge whose validation fires only `errCompactRecoveryTargetUnchanged`
- WHEN the walker runs
- THEN `edges[]` has one entry with `anomaly_class="unchanged_target"`

### Scenario: S4 — Malformed authorization only
- GIVEN one edge whose validation fires only `errCompactRecoveryAuthorizationInexact`
- WHEN the walker runs
- THEN `edges[]` has one entry with `anomaly_class="malformed_recovery_authorization"`

### Scenario: S5 — Both anomalies
- GIVEN one edge whose validation fires both sentinels
- WHEN the walker runs
- THEN one entry is emitted with `anomaly_class="unchanged_target,malformed_recovery_authorization"`

### Scenario: S6 — Two invalid edges
- GIVEN two independent invalid edges with distinct successors
- WHEN the walker runs
- THEN both appear once, ordered by successor lineage and then revision

### Scenario: S7 — Reversed input order
- GIVEN S6's inventory presented in reversed input order
- WHEN the report is serialized
- THEN its JSON bytes equal S6's bytes exactly

### Scenario: S8 — Read-only invariant
- GIVEN a store snapshot captured before inspection
- WHEN `InspectCompactAuthority` returns
- THEN a deep comparison shows the store snapshot is unchanged

### Scenario: S9 — Load error
- GIVEN one unloadable store
- WHEN the CLI runs without panic
- THEN `diagnostics[]` contains `code` and `message`, and the exit code is non-zero

### Scenario: S10 — CLI registration and shape
- GIVEN a repository containing an inspectable inventory
- WHEN `gentle-ai review inspect-authority --cwd <repo>` runs
- THEN stdout is parseable Report JSON, keys are exactly `summary`, `edges`, `diagnostics`, and no extra flag is required

### Scenario: S11 — Shared validation and strict gate
- GIVEN an edge invalid under `validateCompactRecoveryEdge`
- WHEN inspection and an existing strict review path evaluate it
- THEN inspection reports the same sentinel-derived class and the strict path remains unchanged

## Out of scope

Cycle/fork detection, batch reconciliation (#1452), mutation, and additional CLI flags are excluded.

## Acceptance

- Every scenario passes as a focused Go test using table-driven `t.Run` cases and `t.TempDir()` where filesystem state is needed.
- `go test ./...` is green.
- `go vet ./...` is clean.
- `gofmt` reports no diffs.
