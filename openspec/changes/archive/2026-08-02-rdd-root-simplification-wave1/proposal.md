# Proposal: RDD Root Simplification — Wave 1 (Shadow Algebra)

## Intent

Wave 0 proved the root cause is split ownership: relation logic lives independently in `prepr.go`, recovery binding, the facade, and five gates, so every local fix leaves a sibling route on the old interpretation. Wave 1 builds the single replacement — a read-only candidate resolver, relation algebra, and authority-graph classifier — and measures it against today's live decisions **before** anything depends on it. Success is evidence, not behavior: a differential matrix with every divergence explained, and zero change to the live lifecycle.

## Scope

### In Scope

- Canonical `CandidateIdentity` resolver (`repository_id`, `base_tree`, `candidate_tree`, `changed_paths_modes_digest`, `policy_hash`) accepting staged, workspace, committed-range, and workspace-overlay selectors as variants.
- One relation function returning `exact | compatible_base_advance | provable_contraction | changed | unrelated | ambiguous | unknown`. `compatible_base_advance` delegates to `deriveBaseAdvanceCompatibility` (`internal/reviewtransaction/prepr.go:73`) as normative semantics (Amendment A); `provable_contraction` degrades to `changed` when any admitted finding references an excluded path (Amendment B).
- Read-only authority-graph classifier: `healthy | repairable | blocked`.
- Shadow harness observing start, status, and the five gates — records agreement/divergence, influences no outcome, sits behind a disable switch (the rollback boundary), and never blocks a human (freeze policy, Blocking budget).
- Differential matrix (exit evidence): selector × base movement × contraction × ambiguity × unknown.
- Direct characterization tests for `deriveBaseAdvanceCompatibility` — 4 callers, no covering tests (Wave 0 verify SUGGESTION-5) — before it becomes normative.

### Out of Scope

- Any mutation of live lifecycle decisions, authority, or receipts.
- New public operations, contract versions, persisted artifacts, or state values.
- Consumer/adapter changes, gate cutover, facade replacement, repair execution, deletions (Waves 2–7).

## Coverage

Wave-1 `absorbed-into-wave-1` backlog (17): gentle-ai #1234, #1453, #1523, #1530, #1531, #1532, #1563, #1590, #1736, #1740, #1758, #1762, #2160, #2169; gentle-pi #194, #197, #204. Wave 1 measures, it does not close them.

## Capabilities

### New Capabilities

- `rdd-candidate-identity`: canonical identity and selector normalization.
- `rdd-candidate-relation-algebra`: the seven-value relation function and its proof obligations.
- `rdd-authority-graph-classification`: read-only health classification.
- `rdd-shadow-evaluation`: harness, disable switch, differential matrix.

### Modified Capabilities

- None — no existing spec's required behavior changes.

## Approach

Add one read-only package that computes what the live system would decide, delegating (never reimplementing) the existing proof in `prepr.go`. Live call sites gain observation only. Divergence is the product: agreement proves the algebra is adoptable in Wave 3+; divergence names a real split-ownership defect with evidence. Freeze policy exempts tracker-chain wave work.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| new shadow package (`internal/…`) | New | Resolver, relation function, classifier, harness |
| `internal/reviewtransaction/prepr.go` | Modified | Read-only delegation seam only; no proof-logic change |
| `internal/reviewtransaction/{gate,compact_gate,compact_recovery_binding}.go`, review CLI | Modified | Outcome-neutral shadow observation call sites |
| Tests / bench | New | Characterization tests + differential matrix fixtures |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Shadow Git work (`merge-tree`, patch identity) slows live gates | Med | Off in live paths by default; matrix from deterministic fixtures; disable switch |
| A second relation implementation becomes a second truth | Med | Delegate to `prepr.go`; divergence is measured output, not silently reconciled |
| `ambiguous`/`unknown` have no live counterpart to diff | Med | Fixture rows marked "no live decision" rather than fabricated agreement |
| Unexported delegation forces package placement | Low | `sdd-design` decides the package boundary |

## Rollback Plan

Flip the disable switch: no shadow code path executes and every live decision is byte-identical. Full revert is deleting the shadow package and its call sites — no persisted artifact, contract version, or state value exists to migrate.

## Dependencies

- Wave 0 exit evidence (landed): design, ownership inventory, freeze policy, backlog disposition.
- Tracker `feature/rdd-root-simplification` continuing from `main`.

## Success Criteria

- [ ] Shadow on/off produces identical live outcomes, proven by test.
- [ ] Differential matrix covers all seven relations across four selectors, plus base movement, contraction, ambiguity, unknown; every divergence explained.
- [ ] `deriveBaseAdvanceCompatibility` has direct covering tests.
- [ ] Zero new public operations, contract versions, persisted artifacts, or state values.
- [ ] Shadow evaluation blocks no human and emits no user-facing stop.

## Proposal question round

Auto execution mode — asked here rather than interactively. Assumptions stand unless corrected.

1. **Shadow default**: ON in live paths (real-traffic divergence, added Git cost per gate) or OFF with a fixture-driven matrix? *Assumption: OFF in live paths; opt-in sampling.*
2. **Divergence destination**: test/bench output only, or a written record? A persisted record would add an artifact the design forbids. *Assumption: no persisted artifact.*
3. **Exit bar**: what divergence blocks Wave 2? *Assumption: any unexplained divergence on `exact`, `compatible_base_advance`, or `provable_contraction` blocks; explained ones are documented.*
4. **gentle-pi scope**: must the resolver cover Pi protocol-1.1 overlay selectors now (pi#194/#197/#204 are absorbed here)? *Assumption: gentle-ai selectors only; Pi is covered by the same algebra at the consumer wave.*
