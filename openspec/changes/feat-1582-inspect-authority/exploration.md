# Exploration: feat-1582-inspect-authority

## Current State

`compactAuthorityLeaves` (`internal/reviewtransaction/compact_store.go:474`) enumerates every recovery successor in the compact-v2 authority graph and calls `validateCompactRecoveryEdge` on each. It returns on the first error (`compact_store.go:487-488`):

```go
if err := validateCompactRecoveryEdge(predecessor, record.State); err != nil {
    return nil, fmt.Errorf("invalid compact authority graph: %w", err)
}
```

This means callers — including `ReconcileInvalidRecoveryEdge` — can only report one broken edge at a time, regardless of how many exist in the graph.

`validateCompactRecoveryEdge` (`compact_store.go:318`) is the single validation function for all recovery edges. It returns two sentinel errors that `reconcile-authority` classifies as distinct anomaly classes:

- `errCompactRecoveryTargetUnchanged` (`compact_store.go:42`): `"escalated recovery successor target has not changed"`
- `errCompactRecoveryAuthorizationInexact` (`compact_store.go:47`): `"escalated recovery requires an exact maintainer authorization binding"`

The canonical anomaly class names accepted by `reconcile-authority` are defined in `compact_reconcile.go:17`:

```go
const compactCombinedRecoveryAnomalies = "unchanged_target,malformed_recovery_authorization"
```

The full switch in `ReconcileInvalidRecoveryEdge` (`compact_reconcile.go:141-188`) classifies an edge error using `errors.Is` against those sentinels and produces either an `InvalidRecoveryEdge` proof, a `MalformedRecoveryAuthorization` proof, or both.

The gate at `compact_reconcile.go:219-223` is the whole-graph `compactAuthorityLeaves` call that `#1452` will relax; it is out of scope for this change.

---

## Affected Areas

- `internal/reviewtransaction/compact_store.go` — `compactAuthorityLeaves` (lines 474-513) is the source of the first-error behavior; new enumeration function belongs here.
- `internal/reviewtransaction/compact_reconcile.go` — `ReconcileInvalidRecoveryEdge` (lines 78-243) consumes the anomaly class classification; the new enumeration must produce classes that are `errors.Is`-compatible with the same sentinels.
- `internal/cli/review_reconcile.go` — `RunReviewReconcileAuthority` (lines 18-60) establishes the CLI pattern: `newReviewFlagSet`, `parseReviewFlags`, `reviewHelpRequested`, `encodeReviewJSON`. A parallel `RunReviewInspectAuthority` goes here.
- `internal/cli/review.go` — `newReviewFlagSet` (line 32), `encodeReviewJSON` (line 568) are reused by all review subcommands.
- `internal/reviewtransaction/compact_reconcile_test.go` — existing fixtures (`poisonedRecoveryFixture`, `combinedRecoveryFixture`, `preContractRecoveryFixture`, `preContractFixtureAuthorization`) are reusable for new enumerator tests.
- `internal/reviewtransaction/status.go` — `markCompactGraph` (lines 357-426) also calls `validateCompactRecoveryEdge` per edge and records a single problem string per entry; the inspect enumerator is a superset of this capability.

---

## Approaches

### Approach A — New `InspectCompactAuthority` function in `compact_store.go`, new CLI in `review_reconcile.go`

Add a new exported function `InspectCompactAuthority(ctx context.Context, repo string) (CompactAuthorityInspection, error)` in `compact_store.go` that walks every recovery edge and collects all validation failures instead of returning on the first. Mirror the CLI pattern of `RunReviewReconcileAuthority` in `internal/cli/review_reconcile.go` as `RunReviewInspectAuthority`.

**Pros:**
- Minimal blast radius: `compactAuthorityLeaves` itself is untouched (no risk to existing callers).
- The enumeration function is testable in isolation from the CLI.
- `validateCompactRecoveryEdge` already produces the exact sentinel errors needed for `errors.Is` classification.
- Follows existing package layout: new public function alongside `CompactAuthorityLeaves`.
- CLI pattern exactly mirrors `RunReviewReconcileAuthority`, consistent with project conventions.

**Cons:**
- Duplicates some logic from `compactAuthorityLeaves` (graph walking, cycle detection) — but the duplication is justified because `compactAuthorityLeaves` is a graph-leaf detector (returns stores with no children), not an edge inspector.
- Requires a new exported type `CompactAuthorityInspection` in `compact_store.go`.

**Effort:** Medium.

---

### Approach B — Extend `compactAuthorityLeaves` with an enumeration callback option

Refactor `compactAuthorityLeaves` to accept an optional callback that fires on each edge validation result, collecting failures in a provided accumulator.

**Pros:**
- Reuses existing graph-walking code without duplication.

**Cons:**
- Complicates the signature of `compactAuthorityLeaves`, a widely-called function (directly called from 3+ call sites: `RecoverCompactAuthority:235`, `ReconcileInvalidRecoveryEdge:221`, `StartCompactAuthority:648`, `explicitCompactSuccessor:781`).
- Violates single responsibility; the leaf detector would now also be an inspector.
- Higher risk to existing callers.

**Effort:** High (refactoring a widely-called internal API).

---

### Recommendation

Approach A. A new top-level enumeration function is the cleanest path:

1. New `InspectCompactAuthority` in `compact_store.go` — walks the graph like `compactAuthorityLeaves` but collects all edge validation results.
2. New `RunReviewInspectAuthority` in `internal/cli/review_reconcile.go` — CLI surface mirroring `RunReviewReconcileAuthority`.
3. New `ReviewInspectAuthorityResult` in `internal/cli/review_reconcile.go` — JSON output type.

`compactAuthorityLeaves` is not touched. The existing gate at `compact_reconcile.go:219-223` is not touched.

---

## Proposed JSON Output Schema

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
      "predecessor_revision": "sha256:...",
      "successor_lineage_id": "def456",
      "successor_revision": "sha256:...",
      "anomaly_class": "unchanged_target",
      "validation_error": "escalated recovery successor target has not changed"
    },
    {
      "predecessor_lineage_id": "ghi789",
      "predecessor_revision": "sha256:...",
      "successor_lineage_id": "jkl012",
      "successor_revision": "sha256:...",
      "anomaly_class": "malformed_recovery_authorization",
      "validation_error": "escalated recovery requires an exact maintainer authorization binding (projection=workspace target_identity=sha256:...)"
    }
  ]
}
```

**Ordering:** Sorted by `predecessor_lineage_id` ascending (lexicographic, same as `sort.Slice` pattern used in `compactAuthorityLeaves:511` and `DiscoverCompactStores:599`).

**Anomaly class names** are exactly `unchanged_target` and `malformed_recovery_authorization` — matching the string values in `compactCombinedRecoveryAnomalies` (`compact_reconcile.go:17`). The `errors.Is` check in the enumeration function uses the sentinel errors (`errCompactRecoveryTargetUnchanged`, `errCompactRecoveryAuthorizationInexact`) to classify each failure; the string value written to JSON is the canonical name from `compactCombinedRecoveryAnomalies`.

---

## CLI Surface Proposal

```
gentle-ai review inspect-authority --cwd <repo>
```

**Flags (mirroring `RunReviewReconcileAuthority`):**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cwd` | string | `"."` | Repository path |

No other flags required (read-only enumeration, no mutation parameters).

**Output:** JSON to stdout on success. Non-zero exit on error.

**Error conditions:**
- Repository root cannot be resolved → descriptive error.
- No compact authority present → `{"schema":..., "operation":"review/inspect-authority", "repository":"...", "total_edges":0, "invalid_edges":0, "valid_edges":0, "edges":[]}` (empty, not an error).

---

## Test Surface Proposal (RED scenarios)

Table-driven tests in a new `compact_inspect_test.go` under `internal/reviewtransaction/`.

### Scenario 1: Empty graph
- **Given** no compact authority stores exist
- **When** `InspectCompactAuthority` is called
- **Then** result has `total_edges: 0`, `invalid_edges: 0`, `valid_edges: 0`, `edges: []`

### Scenario 2: All edges valid
- **Given** a valid linear chain A → B → C (all edges pass `validateCompactRecoveryEdge`)
- **When** `InspectCompactAuthority` is called
- **Then** result has `total_edges: 2`, `invalid_edges: 0`, `valid_edges: 2`, `edges: []`

### Scenario 3: One invalid edge (unchanged target)
- **Given** a chain with one poisoned recovery successor (unchanged target anomaly)
- **When** `InspectCompactAuthority` is called
- **Then** result has `total_edges: 1`, `invalid_edges: 1`, `valid_edges: 0`; `edges[0].anomaly_class == "unchanged_target"`

### Scenario 4: One invalid edge (malformed authorization)
- **Given** a chain with one pre-contract authorization edge
- **When** `InspectCompactAuthority` is called
- **Then** result has `total_edges: 1`, `invalid_edges: 1`; `edges[0].anomaly_class == "malformed_recovery_authorization"`

### Scenario 5: Multiple invalid edges (complete enumeration)
- **Given** a graph with three edges: two invalid (one unchanged target, one malformed auth), one valid
- **When** `InspectCompactAuthority` is called
- **Then** `invalid_edges: 2`, `edges` contains both anomaly classes; the third edge is counted as valid

### Scenario 6: Corrupt successor store does not crash enumeration
- **Given** one lineage directory exists but contains non-JSON content
- **When** `InspectCompactAuthority` is called
- **Then** result is returned with `diagnostic` entries describing the load failure; valid and other invalid edges are still reported

### Scenario 7: Deterministic ordering
- **Given** multiple invalid edges with different predecessor lineage IDs
- **When** `InspectCompactAuthority` is called twice on the same repo
- **Then** both calls return `edges` in identical order (sorted by `predecessor_lineage_id`)

### Scenario 8: Cycle detection does not crash
- **Given** a recovery cycle exists in the graph
- **When** `InspectCompactAuthority` is called
- **Then** the cycle is detected and reported as a graph-level diagnostic; individual cycle member edges are still classified

---

## Risks

1. **Graph-walking duplication**: `InspectCompactAuthority` will reimplement the loading and cycle-detection logic of `compactAuthorityLeaves`. Any future change to that logic must be propagated. Mitigation: both functions use the same `validateCompactRecoveryEdge` as the source of truth for per-edge validation; the duplication is in traversal topology only.

2. **Load errors during enumeration**: If a store is corrupt, `DiscoverCompactStores` already skips directories that fail to load `review-state.json` and are not "unpublished" (have stray `.atomic-` files only). A partially corrupt store that loads as JSON but is semantically corrupt must be surfaced in the inspection output without aborting the pass. Need to decide whether a load error produces a diagnostic entry or is included as an edge with a special `anomaly_class` value.

3. **New exported API surface**: Adding a new public function `InspectCompactAuthority` in `compact_store.go` increases the package's public contract. Any future change to its return type is a breaking change for callers outside the package (currently only the CLI will call it directly).

4. **Relationship with `markCompactGraph` in `status.go`**: `markCompactGraph` already calls `validateCompactRecoveryEdge` per entry and records one problem string per invalid edge. The new enumerator is a superset capability — more detailed per-edge output and complete enumeration. No conflict, but the relationship should be documented in the design phase.

---

## Rollback Considerations

Since this is a read-only inspection command with no mutation:
- **No rollback plan required** for the command itself.
- If the implementation introduces a new exported type or function that proves wrong in future phases, the fix is additive (new function) or renaming with a deprecation period — no data is altered.
- The CLI registration (`RunReviewInspectAuthority`) is a pure addition to the flag set in `review.go` — removing it is a one-line deletion.

---

## Open Questions for Orchestrator/User

1. **Load-error handling**: Should a store that fails to load (`store.Load()` returns error) be included in the output as a diagnostic entry, or skipped silently? The current `InventoryAuthority` (`status.go:239-242`) marks such entries as `AuthorityStatusInvalid` with the error as a problem string. The inspect command should probably include them. Confirm: should load failures appear as entries in `edges` with a special `anomaly_class` (e.g., `"unloadable"`), or as separate top-level `diagnostics` (matching `AuthorityStatusReport.Diagnostics`)?

2. **Combined anomaly edges**: An edge can have both `unchanged_target` AND `malformed_recovery_authorization` simultaneously (as demonstrated by `TestReconcileCombinedRecoveryAnomaliesQuarantinesWithBothProofsAndRestoresAuthority`). The current `compactCombinedRecoveryAnomalies` constant uses `"unchanged_target,malformed_recovery_authorization"`. Should the enumerator report this as a single entry with `anomaly_class: "unchanged_target,malformed_recovery_authorization"` (matching the wire format of the authorization binding), or as two separate entries? `reconcile-authority` handles both via the `compactCombinedReconcileAuthorizationBinding` function which appends `\nanomalies=unchanged_target,malformed_recovery_authorization`. Recommendation: single entry, combined class string. Confirm?

3. **Valid edge reporting**: The issue says "Valid edges reported as such (verifiable count)" — does this mean every valid edge should appear in the output with `anomaly_class: null` / omitted, or does the total/count fields suffice without per-valid-edge entries? Recommendation: include only invalid edges in the `edges` array; `valid_edges` count in the summary is sufficient for verification. Confirm?

4. **Graph-level anomalies**: Beyond per-edge anomalies, the graph can have cycle errors and fork errors (two successors to the same predecessor). These are detected by `compactAuthorityLeaves` but not by `validateCompactRecoveryEdge` alone. Should the inspection output include a separate `graph_problems` array for these structural anomalies? The issue mentions "one entry per invalid edge" — cycle/fork are edge-adjacent but not per-edge. Confirm whether structural graph errors are in scope.

5. **CLI flag set registration**: The `review` subcommand flag set is constructed in `review.go`. The orchestrator should confirm whether `RunReviewInspectAuthority` is registered in the same switch/if chain as the other review subcommands, or if there is a separate registration mechanism.
