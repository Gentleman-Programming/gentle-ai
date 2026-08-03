# Delta for RDD New-Lineage Activation

> **Re-base caveat**: this delta is layered on `openspec/changes/rdd-root-simplification-wave3/specs/rdd-new-lineage-activation/spec.md`, which has not yet archived to `openspec/specs/rdd-new-lineage-activation/spec.md`. If Wave 3 archives before Wave 5, re-diff this delta against the archived top-level spec before Wave 5 archives; the requirement text below is authoritative as of this writing.

## MODIFIED Requirements

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
