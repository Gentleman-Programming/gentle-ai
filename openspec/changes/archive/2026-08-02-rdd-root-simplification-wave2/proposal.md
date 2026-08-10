# Proposal: RDD Root Simplification — Wave 2 (Leaf Disposition)

## Intent

Authority recovery is currently a growing verb taxonomy: `review repair` executes exactly one class (`legacy_v1_historical_alias`, `authority_repair.go:24`), `review reconcile-authority` admits two historical edge anomalies (`compact_reconcile.go:79`), `review abandon` takes pristine successors. Every new corruption shape proposes another verb or class — #1892 (historical exact-binding edge), #2014 (content-mismatched binding), open PR #2111 (recovery binding whose target identity never persisted) — while #1656 shows the end state: an operator holding authority that no command can clear. Wave 1 shipped the read-only classifier (`healthy | repairable | blocked`) but executes nothing.

Wave 2 makes exactly one classified disposition executable, with the smallest provable blast radius: a leaf. Success is a generic plan shape plus a deliberately narrow executor — not a repair-verb catalogue.

## Scope

### In Scope

- Generic `DispositionPlan` (`repository_id`, `authority_inventory_revision`, `anomaly_class`, `ordered_seed_set`, `ordered_closure`, `expected_revisions`, `plan_digest`, `actor`, `reason`, `authorization`), derived deterministically from `InspectCompactRecoveryEdges` — the graph source of record.
- #1892 leaf-only executor: admits only a classified malformed leaf where `closure(S)` has cardinality one. Non-leaf refuses; no predecessor pointer is rewritten.
- Byte-preserving quarantine with forensic residue; re-inspection under maintenance lock and CAS; retained-graph revalidation before success; exact replay converges without moving an entry twice.
- Closed anomaly classification: unknown, mixed, and ambiguous stay `blocked`. No generic quarantine fallback.
- Route existing classified repair execution through the plan internally; supersede PR #2111's per-anomaly approach.
- Exit evidence: black-box repair (bench damaged-store axis), replay, crash, concurrency, refusal, and retained-graph validation.

### Out of Scope

- **#2014 descendant closure — explicitly deferred to Wave 6.** Multi-node closure gets no executor here.
- New public repair verbs; internal handlers stay private (design control-reduction row `Per-operation recovery verbs → MERGE`).
- Changes to the five gates, facade routing, or SDD.
- Legacy-path deletion (Wave 7); full closure of #1656's multi-lineage case.

## Coverage

`absorbed-into-wave-2`: gentle-ai #1656, #1892, #2014 (deferred), and PR #2111 (superseded). Wave 2 closes only the leaf case.

## Capabilities

### New Capabilities

- `rdd-authority-disposition-plan`: plan shape, deterministic closure derivation, plan digest, authorization binding.
- `rdd-leaf-disposition-execution`: cardinality-one admission, refusal rules, quarantine/residue, lock+CAS, replay, retained-graph revalidation.

### Modified Capabilities

- `rdd-authority-graph-classification` (created by Wave 1): `repairable` gains a derived plan; classification itself is unchanged.

## Approach

Reuse, do not re-derive. Wave 1's `shadowClassifyAuthorityHealth` already proves the leaf predicate (no other lineage recovers from the successor); Wave 2 promotes that read-only observation into a plan and gives it exactly one executor. The plan is the generic surface; the leaf constraint is a policy on admission, so Wave 6 adds closure cardinality without a new plan shape or verb.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/reviewtransaction/authority_repair.go` | Modified | Execution routed through the plan; classified admission |
| `internal/reviewtransaction/compact_inspect.go` | Modified | Read-only plan derivation seam; no inspection-semantics change |
| new disposition-plan file(s) (`internal/reviewtransaction/…`) | New | Plan shape, closure derivation, digest, authorization |
| `internal/cli/review_repair.go`, `review_next_transition.go` | Modified | Same verb, plan-bound authorization |
| `bench/axis_damaged_store.go` + tests | New/Modified | Black-box repair, replay, crash, concurrency, refusal journeys |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Plan looks generic but hard-codes leaf assumptions, blocking Wave 6 | Med | Closure is derived generically; cardinality is an executor admission check, not a plan-shape constraint |
| Executor becomes a de-facto new verb surface | Med | No new public verb; handlers private; plan-bound authorization only |
| Quarantine loses forensic bytes on crash | Low | Byte-preserving move with residue; crash and replay evidence required before exit |
| Concurrent operator + executor mutate under a stale inventory revision | Med | Re-inspect under maintenance lock; CAS on `expected_revisions`; refuse on drift |
| Superseding #2111 strands its reproduction evidence | Med | Its anomaly shape must be a classified class here, or Wave 2 does not supersede it |

## Rollback Plan

Disable the classified repair disposition: no plan is executed, inspection and classification remain read-only, and authority bytes are untouched. Quarantined entries retain residue, so a disabled executor never leaves an unrecoverable state. Full revert deletes the plan file(s) and the executor admission path.

## Dependencies

- Wave 1 exit evidence and its tracker→main merge (chain continues on `feature/rdd-root-simplification`).
- Design decision 4 (maintainer-bound disposition plan) as the authorization shape.

## Success Criteria

- [ ] A classified malformed leaf is repaired end-to-end through a plan, black-box, with residue preserved.
- [ ] A non-leaf, unknown, mixed, or ambiguous shape refuses with a named reason and a named exit (freeze policy, Blocking budget rule 3).
- [ ] Exact replay converges; no entry moves twice.
- [ ] Crash mid-execution and concurrent execution both leave a valid retained graph.
- [ ] Retained graph revalidates before success is reported.
- [ ] Zero new public repair verbs; no gate, facade, or SDD behavior changes.

## Proposal question round

Auto execution mode — asked here rather than interactively. Assumptions stand unless corrected.

1. **Authorization shape**: decision 4 says maintainer-bound plan. Is a short expiry (e.g. minutes) required, or does binding to `plan_digest` + `authority_inventory_revision` suffice? *Assumption: digest+revision binding, no wall-clock expiry — CAS already refuses stale plans, and an expiry adds a blocking failure mode the budget discourages.*
2. **Surface exposure**: CLI-exposed in Wave 2, or internal-only until Wave 3's facade? *Assumption: internal plan derivation plus the existing maintainer `review repair` verb behind explicit authorization — no new command.*
3. **#2111 supersession**: does its anomaly shape become a classified class here, or does Wave 2 close only #1892's? *Assumption: only if its shape classifies as a cardinality-one leaf; otherwise it defers to Wave 6 and its PR stays open.*
4. **#1656 expectation**: operators reading #1656 expect multi-lineage recovery. Is a leaf-only fix acceptable as a partial answer? *Assumption: yes, with #1656 remaining open and explicitly pointed at Wave 6.*
5. **Refusal UX**: must a refusal name the future wave that will handle the shape? *Assumption: it names the diagnosis and the escalation artifact, not a roadmap promise.*
