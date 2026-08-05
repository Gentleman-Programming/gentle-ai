# RDD New-Lineage Activation Specification

## Purpose

Define the activation switch that gates new-lineage transitions, the coexistence precedence between legacy and new authority (Amendment C), the additive (non-cutover) shape of the gate branch, and rollback semantics that never strand an in-flight new lineage.

## Requirements

### Requirement: Distinct Env Switch, Default Off, Legacy Path When Disabled

The activation switch MUST be a dedicated environment variable, default OFF, distinct in identity and meaning from `GENTLE_AI_RDD_SHADOW` and from the user-owned RDD kill switch (adopted default). When off, every `start` MUST take the legacy path and create no new-lineage artifact.

#### Scenario: Default configuration takes the legacy path

- GIVEN the activation switch unset
- WHEN a new `start` is requested
- THEN it proceeds through the legacy state machine and creates no `review-state.json`/`review-receipt.json` new-lineage record

#### Scenario: Switch identity never overloads another switch

- GIVEN the activation switch, `GENTLE_AI_RDD_SHADOW`, and the RDD kill switch
- WHEN any one of the three is toggled
- THEN only its own scoped behavior changes; the other two are unaffected

### Requirement: Kill-Switch-Off Is Structurally Unfailable and Creates Nothing

When the user-owned RDD kill switch is off, the facade MUST create no artifact and MUST NOT be able to fail (Wave-4 precondition).

#### Scenario: Kill switch off produces no side effect

- GIVEN the RDD kill switch is off
- WHEN the facade is invoked at any observed call site
- THEN no artifact is created, and no error path is reachable

### Requirement: Cutover Replaces The Additive Gate Branch

Each of the five gates (`post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`) MUST evaluate every lineage — legacy and new — through the single shared relation-algebra path defined by `rdd-receipt-only-gates`. Wave 5 performs the legacy-to-new cutover Wave 3 deferred: the legacy branch is no longer a separate switch-keyed code path. Outcome equivalence for legacy candidates MUST be proven by the 35-cell boundary matrix, not by switch-off byte-equivalence of a preserved legacy branch.

(Previously: gates received a strictly additive branch keyed on lineage kind; the legacy branch stayed byte-identical when the switch was off; this wave explicitly ruled out a cutover.)

#### Scenario: Cutover replaces the additive branch

- GIVEN Wave 5 has landed
- WHEN any gate evaluates a legacy candidate
- THEN it uses the same relation-algebra path as a new-lineage candidate, not an isolated legacy branch

#### Scenario: Outcome equivalence proven by matrix, not byte-diff

- GIVEN a legacy candidate evaluated pre- and post-cutover
- WHEN the two verdicts are compared
- THEN equivalence is proven by the 35-cell gate boundary matrix, not by asserting the executed code path is byte-identical

> **Amendment (Wave 5 fix cycle 3, verify-report #10186 cycle 2, W-2): letter-vs-intent divergence closed by
> amendment, coordinator-accepted.** The matrix is 9/35 wired and every wired cell drives the compact/v2
> path — zero cells drive legacy v1 or new-lineage v3 — so the scenario's literal letter ("proven by the
> 35-cell gate boundary matrix") is not yet satisfied by the matrix alone. The scenario's INTENT — proven
> outcome equivalence for a legacy candidate across all five gates — IS satisfied today, by
> `TestEvaluateLegacyGateAllowsExactAtAllFiveGates` (Fix Cycle 1's C-B fix, `internal/reviewtransaction/legacy_projection_test.go`)
> and its siblings (`TestEvaluateLegacyGateAllowsExactAndDeniesChanged`,
> `TestEvaluateLegacyGateValidatesReceiptFromAnInFlightCorrection`), which drive the real
> `EvaluateLegacyGate` production path directly and prove `allow` at post-apply/pre-commit/pre-push/pre-pr/
> release for an exact legacy candidate, and correct denial for a changed one. The coordinator's own fix
> cycle 3 instruction (item 4, "amend the matrix-equivalence scenario in the delta spec... cite this
> message") accepts this named-test proof as satisfying the requirement's intent for now. The matrix
> remains the incremental, long-term vehicle this requirement still points at — its wired-cell count is
> tracked explicitly in `tasks.md`'s Fix Cycle 2/3 W-2 entries (8/35 → 9/35 as of Fix Cycle 2, release/exact)
> — but is not, itself, a blocking gap for this requirement while the named-test proof stands.

### Requirement: Unconditional Receipt Precedence (Amendment C Generalized)

Authority precedence generalizes to unconditional receipt precedence: an immutable, boundary-validated receipt of the correct lineage kind governs; absence of such a receipt denies regardless of legacy authority. A legacy-only authority record MUST NEVER authorize a new-lineage candidate, and — post-cutover — a legacy authority record is evaluated through the same receipt-precedence rule as new-lineage authority; there is no separate per-gate {legacy, new} x {exists, absent} branch table.

(Previously: precedence was decided by a per-gate matrix over {legacy authority, new-lineage authority} x {exists, absent}.)

#### Scenario: Legacy authority alone denies a new-lineage candidate

- GIVEN only legacy authority exists for a candidate being evaluated as a new lineage
- WHEN a gate checks authorization
- THEN it denies, even though legacy authority is present

#### Scenario: Receipt precedence is unconditional across lineage kinds

- GIVEN any lineage kind
- WHEN a gate checks authorization
- THEN it authorizes only from an immutable, boundary-validated receipt, using the same relation check regardless of lineage kind

### Requirement: Rollback Restores The Additive Branch, Never Invalidation Writes

Rollback for the cutover is gate-scoped and one-directional: a gate MAY deny (fail closed); it MUST NOT revive legacy mutation (invalidation writes, receipt-graph composition, or decline authorization). Reverting Wave 5 restores the Wave 3/4 additive-branch shape by re-adding the lineage-keyed branch; it MUST NOT be implemented by re-enabling any removed invalidation write.

(Previously: disabling the activation switch stopped new-lineage `start` calls only; already-created new lineages remained readable and could finalize.)

#### Scenario: Rollback re-adds the additive branch, not invalidation writes

- GIVEN Wave 5 is rolled back
- WHEN gates are restored to the additive-branch shape
- THEN no gate regains the ability to mutate authority or delete a receipt file

#### Scenario: In-flight correction at cutover finalizes under the prior lifecycle

- GIVEN a correction opened before cutover
- WHEN it finalizes after cutover
- THEN it completes under the pre-cutover correction lifecycle, and its receipt validates through the new read-only path
