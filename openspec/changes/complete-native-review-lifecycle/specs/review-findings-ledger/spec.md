# Delta for review-findings-ledger

## ADDED Requirements

### Requirement: Immutable genesis scope

Genesis MUST persist the complete canonical path set of the reviewed candidate. A correction MUST change only paths in that immutable set and MUST be rejected before append when it expands scope.

#### Scenario: Out-of-scope correction is rejected
- GIVEN frozen genesis paths
- WHEN a correction changes a path outside it
- THEN it is rejected before append
- AND no correction budget is consumed

### Requirement: Shared catalog lifecycle parity

The shared minimal frozen-ledger contract MUST apply to every `catalog.AllAgents()` runtime. Agents MUST NOT define discovery or fix semantics. Catalog-driven parity coverage MUST fail for omitted, divergent, or stale agent contracts.

#### Scenario: Catalog agent receives shared lifecycle
- GIVEN every `catalog.AllAgents()` runtime
- WHEN shared lifecycle parity runs
- THEN each runtime has identical minimal lifecycle semantics
- AND no runtime adds discovery or fix behavior

## MODIFIED Requirements

### Requirement: Initial findings freeze

Each selected lens MUST emit a neutral initial claim set with `id`, `lens`, `location`, `severity`, `claim`, `evidence_class`, `proof_refs`, and `status`. Full 4R has four sweeps. Findings MUST freeze after merge. Frozen accepted/blocking IDs MUST govern ordinary correction; later discovery MUST NOT reopen the candidate.

(Previously: finding freeze did not constrain ordinary correction.)

#### Scenario: Full 4R selects four lenses once

- GIVEN high risk classification
- WHEN initial review runs
- THEN risk, readability, reliability, and resilience each run once
- AND no lens reopens the original diff after findings freeze

### Requirement: One ordinary correction transaction

Ordinary 4R MUST permit one correction. It MAY contain atomic work units; each MUST map exactly to frozen accepted/blocking IDs, stay in immutable genesis scope, and record focused-test and runtime evidence (or N/A) plus independent rollback boundary.

(Previously: correction lacked immutable scope and exact frozen ledger.)

#### Scenario: Atomic work units share one budget

- GIVEN three independent fixes address frozen IDs
- WHEN the parent launches their correction transaction
- THEN the native fix counter increases once
- AND work-unit count cannot create another correction budget

### Requirement: One non-iterative ordinary scoped validator

After correction, the shipped CLI lifecycle MUST run one detached read-only validator that receives frozen ledger and immutable fix delta, verifies only original acceptance criteria/tests and correction regression evidence, and returns only `approve | escalate`. It MUST NOT conduct discovery or launch another correction or validator. A post-freeze observation MUST be non-blocking and MUST NOT alter ledger, scope, budget, or correction; proof correction caused an original-criterion failure MUST deny approval and reject, revert, or escalate.

(Previously: validator could append proven fix-caused defects.)

#### Scenario: Fix-caused defect escalates

- GIVEN correction regression evidence fails an original acceptance criterion
- WHEN scoped validation completes
- THEN approval is denied and the transaction rejects, reverts, or escalates
- AND no second ordinary correction starts

### Requirement: Independent final verification

One independent final verification MUST check requirement/scenario counts, task completion, current test/build evidence, frozen-ledger resolution, target identity, and counters. Passing frozen ledger and original criteria/tests MUST permit progress. A contradiction MUST escalate and MUST NOT restart review, refuter, correction, or discovery.

(Previously: passing frozen-ledger and criterion evidence was insufficient.)

#### Scenario: Final contradiction does not loop

- GIVEN scoped validation approved the fix delta
- WHEN runtime verification discovers a contradiction
- THEN final verification fails and the transaction escalates
- AND no ordinary review or correction budget is reopened

## Unchanged v1 Behavior

The v1 repository-derived CAS store, exact append retry, `review-resume`, receipts, gates, and bundles remain unchanged. Existing internal v1 callers MAY retain the legacy boolean transition for compatibility, but the shipped CLI MUST NOT use it for targeted validation. This delta MUST NOT introduce v2 schemas, journals, retirement/discovery, causality taxonomy, new seals/gates, bundle migration, or another broad review.
