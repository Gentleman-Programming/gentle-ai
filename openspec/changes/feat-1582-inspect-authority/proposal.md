# Proposal: Inspect Authority (read-only edge enumeration)

## Intent

Gentleman-Programming/gentle-ai#1582 asks for a read-only enumeration that walks every compact-v2 recovery edge, collects all `validateCompactRecoveryEdge` failures instead of returning on the first, and exposes the result as `gentle-ai review inspect-authority --cwd <repo>`. Today, `compactAuthorityLeaves` (`internal/reviewtransaction/compact_store.go:474-513`) aborts on the first invalid edge at `compact_store.go:487-488`, so `reconcile-authority` (`internal/reviewtransaction/compact_reconcile.go:78`) can only ever surface one broken edge per call.

This change is the **hard prerequisite for #1452** ("batch reconcile of multiple invalid edges"). Until a complete enumeration exists, #1452 cannot safely relax the gate at `compact_reconcile.go:221` (`compactAuthorityLeaves` call inside `ReconcileInvalidRecoveryEdge`) because callers have no way to discover every edge that needs reconciling. The combined-anomaly edge shape accepted here is also the wire format #1465 just merged for `compactCombinedRecoveryAnomalies`, so this enumerator must classify by `errors.Is` against the same sentinels.

The deliverable is purely additive: a new exported walker, a new CLI subcommand, and a JSON report that is `reconcile-authority`-shaped so #1452 can consume it directly.

## Scope

### In scope (per the four authoritative product decisions)

1. **New exported walker** `InspectCompactAuthority(ctx, repo) (Report, error)` in `internal/reviewtransaction/compact_store.go` that walks every recovery edge, calls the existing `validateCompactRecoveryEdge` (`compact_store.go:318`) per edge, and collects failures instead of returning early.
2. **New CLI subcommand** `RunReviewInspectAuthority` registered next to `RunReviewReconcileAuthority` at `internal/cli/review_facade.go:320-321`. Output is a single JSON report to stdout.
3. **Per-edge anomaly classification** matching what `reconcile-authority` already accepts (so the #1452 batch can consume the output directly): `unchanged_target`, `malformed_recovery_authorization`, and the combined `unchanged_target,malformed_recovery_authorization` for edges that fail both sentinels simultaneously (mirrors `TestReconcileCombinedRecoveryAnomaliesQuarantinesWithBothProofsAndRestoresAuthority`).
4. **Load-error surfacing** as a diagnostic entry in `diagnostics[]`, following the `AuthorityStatusReport.Diagnostics` pattern (`internal/reviewtransaction/status.go:91`). Load failures never abort the pass.
5. **Deterministic ordering** by `successor.lineage` ascending, then `successor.revision` ascending.

### Out of scope

- Any change to `compactAuthorityLeaves` semantics (`compact_store.go:474-513`). The gate at `compact_reconcile.go:221` stays intact — relaxing it is #1452's territory.
- Cycle detection (`compact_store.go:494-503`) and fork detection (`compact_store.go:490-493`). These graph-level anomalies are reported by `compactAuthorityLeaves`; this enumerator does not duplicate them. They remain #1452's call.
- Any mutation. `InspectCompactAuthority` and the CLI are strictly read-only.
- `errors.Is` classifier changes. The existing sentinels (`errCompactRecoveryTargetUnchanged` `compact_store.go:42`, `errCompactRecoveryAuthorizationInexact` `compact_store.go:47`) are the source of truth.

## Approach

Two additive pieces, no shared-callers refactor:

1. **`internal/reviewtransaction/compact_store.go`** — add `InspectCompactAuthority` after `CompactAuthorityLeaves` (which ends at `compact_store.go:472`). The function reuses `DiscoverCompactStores`, calls `validateCompactRecoveryEdge` on every successor/predecessor pair, classifies the resulting error via `errors.Is` against the two sentinels (mirroring `compact_reconcile.go:142` and `compact_reconcile.go:169`), and assembles a `CompactAuthorityInspection` value. Load errors are appended to a `Diagnostics []AuthorityInventoryDiagnostic` field (mirroring `status.go:77-91`).
2. **`internal/cli/review_reconcile.go`** (or a sibling `review_inspect.go`) — add `RunReviewInspectAuthority(args, stdout)` modeled on `RunReviewReconcileAuthority` (`review_reconcile.go:18-60`). Same flag-set reuse via `newReviewFlagSet` (`internal/cli/review.go:32`), same `encodeReviewJSON` (`internal/cli/review.go:568`). Register the dispatch case at `review_facade.go:320-321` and the usage line at `review_facade.go:229`.

`validateCompactRecoveryEdge` stays the shared source of truth. No new classification logic.

## Affected `internal/` packages

| File | Change | Purpose |
|---|---|---|
| `internal/reviewtransaction/compact_store.go` | Add `InspectCompactAuthority` + `CompactAuthorityInspection` after `compact_store.go:513` | Walker that collects all edge failures, classifies by `errors.Is`, returns the Report. |
| `internal/reviewtransaction/compact_inspect_test.go` | New test file (added in sdd-spec / sdd-tdd phases, not here) | RED scenarios from §"Testing approach" below. |
| `internal/cli/review_inspect.go` | New file with `RunReviewInspectAuthority` + `ReviewInspectAuthorityResult` | CLI surface mirroring `review_reconcile.go:18-60`. |
| `internal/cli/review_facade.go` | Edit line 229 (usage list) and line 320 (dispatch switch) | Register `inspect-authority` next to `reconcile-authority`. |
| `internal/cli/review_status_contract.go` | Possibly add `review.inspect-authority` audit channel near line 82 | Mirror existing audit-channel contract. |
| `internal/reviewtransaction/status.go` | **Unmodified** but referenced as the diagnostic-shape precedent (line 77, line 91) | Not edited; `AuthorityInventoryDiagnostic` is the shape reused. |
| `internal/reviewtransaction/compact_reconcile.go` | **Unmodified** at lines 17, 141-189, 221 | Source-of-truth classification and the gate that #1452 will relax. |

## JSON output schema

```json
{
  "schema": "gentle-ai.review-inspect-authority/v1",
  "operation": "review/inspect-authority",
  "repository": "<repo>",
  "total_edges": 12,
  "invalid_edges": 2,
  "valid_edges": 10,
  "edges": [
    {
      "predecessor_lineage_id": "abc123",
      "predecessor_revision":  "sha256:...",
      "successor_lineage_id":  "def456",
      "successor_revision":    "sha256:...",
      "anomaly_class":         "unchanged_target",
      "validation_error":      "escalated recovery successor target has not changed"
    },
    {
      "predecessor_lineage_id": "ghi789",
      "predecessor_revision":  "sha256:...",
      "successor_lineage_id":  "jkl012",
      "successor_revision":    "sha256:...",
      "anomaly_class":         "unchanged_target,malformed_recovery_authorization",
      "validation_error":      "escalated recovery successor target has not changed"
    }
  ],
  "diagnostics": [
    { "path": "<lineage-dir>", "problem": "<load-error message>" }
  ]
}
```

Field contract:

- `edges[]` — invalid edges only. Valid edges are reported only via the `valid_edges` count.
- `anomaly_class` — exactly one of `unchanged_target`, `malformed_recovery_authorization`, or the combined `unchanged_target,malformed_recovery_authorization` for edges that fail both sentinels in the same `validateCompactRecoveryEdge` call.
- `diagnostics[]` — load failures only, shaped like `AuthorityInventoryDiagnostic` (`status.go:77`: `Path string`, `Problem string`). No `anomaly_class`; never aborts the pass.
- `total_edges = valid_edges + invalid_edges`. A successful load of N lineages with M recovery successors yields `total_edges = M`.
- **Ordering:** `edges[]` sorted by `successor_lineage_id` ascending, then `successor_revision` ascending (tiebreaker). `diagnostics[]` sorted by `path` ascending (matches `sortAuthorityReport` at `status.go:448-456`).

## CLI surface

```
gentle-ai review inspect-authority --cwd <repo>
```

| Flag | Type | Default | Description |
|---|---|---|---|
| `--cwd` | string | `"."` | Repository path (mirrors `RunReviewReconcileAuthority` `review_reconcile.go:20`). |

No new flags. No positional arguments. No stdout decoration besides the JSON report. Non-zero exit on:
- repository root unresolvable;
- context cancellation;
- flag parse error.

Zero-edge graph is NOT an error — returns the schema-shaped empty report (mirrors `status.go:105` empty-init pattern).

Registration: append `case "inspect-authority":` next to `case "reconcile-authority":` at `review_facade.go:320-321`, and add `inspect-authority` to the usage list at `review_facade.go:229`.

## Anomaly classification alignment

The enumerator's `anomaly_class` strings are the canonical wire-format names already accepted by `reconcile-authority`:

| Class | Source string | Citation |
|---|---|---|
| `unchanged_target` | `"unchanged_target"` (joined form) | `compact_reconcile.go:17` `compactCombinedRecoveryAnomalies = "unchanged_target,malformed_recovery_authorization"` |
| `malformed_recovery_authorization` | `"malformed_recovery_authorization"` (joined form) | `compact_reconcile.go:17` same constant |
| Combined | `"unchanged_target,malformed_recovery_authorization"` | `compact_reconcile.go:17` — matches `TestReconcileCombinedRecoveryAnomaliesQuarantinesWithBothProofsAndRestoresAuthority` and the `compactCombinedReconcileAuthorizationBinding` append at `compact_reconcile.go:61-64` |

The classifier is `errors.Is` against the two sentinel errors already used at `compact_reconcile.go:142` and `compact_reconcile.go:169`:

- `errors.Is(edgeErr, errCompactRecoveryTargetUnchanged)` → class contains `unchanged_target`.
- `errors.Is(edgeErr, errCompactRecoveryAuthorizationInexact)` → class contains `malformed_recovery_authorization`.

When both `errors.Is` checks succeed (the combined case), the `anomaly_class` is the joined literal `"unchanged_target,malformed_recovery_authorization"`. When neither succeeds, the edge is recorded with `anomaly_class: "unchanged_target,malformed_recovery_authorization"` only if the prior sibling repair already produced both proofs (mirrors `compact_reconcile.go:155-167`); otherwise the unclassified error string is preserved in `validation_error` and `anomaly_class` is the closest single match. **No new sentinel errors are introduced.**

## Testing approach (RED scenarios — TDD only, no code in this phase)

Scenarios to encode in `internal/reviewtransaction/compact_inspect_test.go` and `internal/cli/review_inspect_test.go`. Each maps to one `t.Run` case derived from #1582 acceptance criteria plus the four product decisions.

| # | Scenario | Setup | Assertion |
|---|---|---|---|
| 1 | Empty graph | `t.TempDir()` with no compact stores | `total_edges=0`, `invalid_edges=0`, `valid_edges=0`, `edges=[]`, `diagnostics=[]`. |
| 2 | All edges valid | Linear chain A→B→C | `total_edges=2`, `invalid_edges=0`, `valid_edges=2`, `edges=[]`. |
| 3 | One invalid edge — `unchanged_target` | Reuse `poisonedRecoveryFixture` (`compact_reconcile_test.go`) | One entry in `edges`; `anomaly_class == "unchanged_target"`; `validation_error` starts with `"escalated recovery successor target has not changed"`. |
| 4 | One invalid edge — `malformed_recovery_authorization` | Reuse `preContractRecoveryFixture` | One entry; `anomaly_class == "malformed_recovery_authorization"`. |
| 5 | Two independent invalid edges (mirrors the #1452 repro) | Fixture with two anomalies | `invalid_edges=2`, `edges` carries both entries, ordering by successor lineage asc. |
| 6 | Combined-anomaly edge | Reuse `combinedRecoveryFixture` (`compact_reconcile_test.go`) | Single `edges` entry; `anomaly_class == "unchanged_target,malformed_recovery_authorization"`. |
| 7 | Deterministic ordering | Two invalid edges with different successor lineage IDs | Run `InspectCompactAuthority` twice; byte-identical JSON output. |
| 8 | Read-only invariant | Any fixture | After the call: file mtimes unchanged; `DiscoverCompactStores` re-load returns the same records; no quarantine, lock, or residue artifacts written. |
| 9 | Unloadable store → diagnostic | One lineage dir with non-JSON `review-state.json` plus one valid lineage | `diagnostics[]` contains one entry for the bad path; the valid lineage still contributes its edge counts; no abort. |
| 10 | CLI integration | `t.TempDir()` with the #1452 repro fixture | `gentle-ai review inspect-authority --cwd <tmp>` emits the JSON report; exit 0; stdout is parseable into the schema. |

Table-driven with `t.Run` per scenario (per `go-testing` skill); `t.TempDir()` for every filesystem case; no real home directory.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Graph-walking duplication vs. `compactAuthorityLeaves` (`compact_store.go:474-513`) | Medium | Both share `validateCompactRecoveryEdge` (`compact_store.go:318`) as the per-edge source of truth. Future changes to that function automatically apply to both. |
| API surface growth (`InspectCompactAuthority` is a new exported name) | Low | Single new export in `compact_store.go`; one new CLI command. Both are additive. No callers depend on them yet. |
| Load-error diagnostic scope ambiguity (successor vs. predecessor) | Low | `AuthorityInventoryDiagnostic.Path` (`status.go:77`) carries the failing lineage directory; the test scenario 9 pins the behavior. |
| Ordering drift if `successor.lineage` is not unique within the graph | Low | Tiebreaker on `successor.revision` (`compact_store.go:511` precedent). |
| Combined-class classifier may be misread as two entries | Low | Locked to a single joined string by product decision 2; mirrors `compact_reconcile.go:17`. |

## Rollback plan

This change is purely additive. No existing caller of `compactAuthorityLeaves`, `validateCompactRecoveryEdge`, or `ReconcileInvalidRecoveryEdge` is modified.

- **Code revert:** single-file revert of `internal/reviewtransaction/compact_store.go` (new walker only) and `internal/cli/review_inspect.go` (entire file) plus the two-line edit at `internal/cli/review_facade.go:229,320`.
- **Data revert:** none — `InspectCompactAuthority` is read-only and writes nothing.
- **Surface revert:** removing the dispatch case from `review_facade.go:320-321` and the usage entry at line 229 is sufficient to retire the CLI command.
- **Gate intact:** the gate at `compact_reconcile.go:221` is untouched, so #1582's rollback cannot regress `reconcile-authority` behavior.

## Prerequisite declaration

**#1452 cannot be unblocked until this change lands and is published.** Specifically:

- #1452's plan to relax `compact_reconcile.go:221` requires that callers (humans or automation) can enumerate every invalid edge before invoking `ReconcileInvalidRecoveryEdge` per edge.
- The #1465-merged combined-anomaly wire format is consumed by `InspectCompactAuthority` (per product decision 2 and the source citation at `compact_reconcile.go:17`). Without `InspectCompactAuthority`, #1452 has no read-only way to discover combined-anomaly edges ahead of the batch.
- Once `gentle-ai review inspect-authority --cwd <repo>` is shipped, #1452's batch reconciler can call it as its input pass.

This PR is intentionally minimal so it can be merged first; #1452 stacks on top.

## Out-of-scope follow-ups

- **#1452 batch reconcile** — relaxes `compact_reconcile.go:221` and adds `RunReviewReconcileAuthority --all-invalid`. Consumes this PR's JSON.
- **Cycle and fork detection** in `inspect-authority` output — currently skipped (product decision 4). If `markCompactGraph` (`status.go:357-426`) starts reporting cycle/fork entries in `AuthorityStatusReport.Diagnostics`, a follow-up change can add a `graph_problems` array.
- **Persistent snapshot of inspection results** — out of scope; the report is stdout-only and re-derivable.
- **Cross-worktree inspection** — out of scope; single `--cwd` repo only.