# RDD New-Lineage Activation Specification

## Purpose

Define the activation switch that gates new-lineage review transitions, the separation between legacy and new review evidence, the additive (non-cutover) shape of review-context evaluation, and rollback semantics that never strand an in-flight new lineage. A lineage and its receipt govern review activity only; ordinary repository and SDD policy govern delivery and archive readiness.

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

### Requirement: Shared Review-Context Evaluation Replaces The Additive Gate Branch

Each of the five integration hooks (`post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`) MUST evaluate review context for every lineage — legacy and new — through the single shared relation-algebra path defined by `rdd-receipt-only-gates`. Wave 5 performs the legacy-to-new review-context cutover Wave 3 deferred: the legacy branch is no longer a separate switch-keyed code path. The shared result is informational review evidence only; it MUST NOT authorize, deny, block, or route commit, push, PR, release, or archive delivery. Outcome equivalence for legacy candidates MUST be proven by the 35-cell boundary matrix, not by switch-off byte-equivalence of a preserved legacy branch.

#### Scenario: Cutover replaces the additive review-context branch

- GIVEN Wave 5 has landed
- WHEN any hook evaluates review context for a legacy candidate
- THEN it uses the same relation-algebra path as a new-lineage candidate, not an isolated legacy branch
- AND its result changes no ordinary delivery decision

#### Scenario: Outcome equivalence is proven by matrix, not byte diff

- GIVEN a legacy candidate evaluated before and after cutover
- WHEN the two review-context results are compared
- THEN equivalence is proven by the 35-cell boundary matrix, not by asserting the executed code path is byte-identical
- AND a mismatched or missing receipt remains review-lifecycle evidence only

> **Amendment (Wave 5 fix cycle 3, verify-report #10186 cycle 2, W-2): letter-vs-intent divergence closed by amendment, coordinator-accepted.** The matrix is 9/35 wired and every wired cell drives the compact/v2 path — zero cells drive legacy v1 or new-lineage v3 — so the scenario's historical letter was not satisfied by the matrix alone. Its intent — equivalent review-context results for a legacy candidate across all five hooks — is supported today by `TestEvaluateLegacyGateAllowsExactAtAllFiveGates` and its siblings (`TestEvaluateLegacyGateAllowsExactAndDeniesChanged`, `TestEvaluateLegacyGateValidatesReceiptFromAnInFlightCorrection`), which drive `EvaluateLegacyGate` directly. Their historical `allow` and denial names describe review-integrity outcomes only; they never authorize or deny delivery. The matrix remains the incremental, long-term vehicle this requirement points at, and its wired-cell count remains tracked in `tasks.md`.

### Requirement: Lineage-Scoped Review Evidence

An immutable, boundary-validated receipt of the correct lineage kind MAY be surfaced as matching review evidence. Its absence, staleness, ambiguity, or invalidity MUST remain visible and repairable inside the review lifecycle, but MUST NOT deny or otherwise control delivery. A legacy-only authority record MUST NEVER be treated as matching evidence for a new-lineage candidate. Post-cutover, legacy and new records use the same lineage-scoped relation rule; there is no separate per-hook `{legacy, new} x {exists, absent}` delivery branch table.

#### Scenario: Legacy authority alone is not evidence for a new-lineage candidate

- GIVEN only legacy authority exists for a candidate being evaluated as a new lineage
- WHEN review context is resolved
- THEN it reports no matching new-lineage evidence and offers only applicable review-lifecycle repair or explicit review work
- AND ordinary repository policy alone decides delivery

#### Scenario: Receipt context is lineage scoped across kinds

- GIVEN any lineage kind
- WHEN review context is resolved
- THEN an immutable, boundary-validated receipt is reported only when it matches that lineage under the shared relation check
- AND its presence or absence does not authorize, deny, block, or route delivery

### Requirement: Rollback Restores The Additive Branch, Never Invalidation Writes

Rollback for the review-context cutover is hook-scoped and one-directional: a hook MAY report a review-integrity mismatch or escalation (fail closed within the review lifecycle), but it MUST NOT revive legacy mutation such as invalidation writes, receipt-graph composition, or decline authorization. Reverting Wave 5 restores the Wave 3/4 additive-branch shape by re-adding the lineage-keyed branch; it MUST NOT be implemented by re-enabling any removed invalidation write. No rollback result becomes a delivery gate.

#### Scenario: Rollback re-adds the additive branch, not invalidation writes

- GIVEN Wave 5 is rolled back
- WHEN review-context hooks are restored to the additive-branch shape
- THEN no hook regains the ability to mutate authority or delete a receipt file
- AND ordinary delivery policy remains unchanged

#### Scenario: In-flight correction at cutover finalizes under the prior lifecycle

- GIVEN a correction opened before cutover
- WHEN it finalizes after cutover
- THEN it completes under the pre-cutover correction lifecycle, and its receipt remains available as review evidence through the new read-only path
- AND that evidence does not govern delivery
