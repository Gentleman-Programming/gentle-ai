# review-recovery-gating Specification

## Purpose

CLI-level pre-persist gating of `gentle-ai review recover` in `RunReviewRecover`
(`internal/cli/review_facade.go`), and its delegation to the package-level
`validateCompactRecoveryEdge` authority (`internal/reviewtransaction/compact_store.go`).
This capability governs which recovery attempts are rejected before
`RecoverCompactAuthority` ever runs, and which are deferred to it.

## Requirements

### Requirement: Unchanged-target `invalidated` recovery MUST be admitted to package validation

When the predecessor's initial snapshot kind is `TargetBaseDiff` or
`TargetBaseWorkspaceOverlay` and the CLI-recomputed successor target `Identity`
equals the predecessor's initial snapshot `Identity`, the CLI gate MUST NOT
reject the recovery with "recovery scope has not changed" when
`--disposition invalidated` is given. The request MUST instead be forwarded to
`RecoverCompactAuthority` → `validateCompactRecoveryEdge`'s `RecoveryInvalidated`
case, whose sole requirement is a `StateInvalidated` predecessor.

#### Scenario: Unchanged-target invalidated recovery succeeds

- GIVEN a predecessor lineage in `StateInvalidated` with initial snapshot kind
  `TargetBaseDiff` (or `TargetBaseWorkspaceOverlay`)
- WHEN `review recover --disposition invalidated` is run against the same
  `--base-ref`/candidate producing an identical target `Identity`
- THEN the CLI gate does not reject the request for "recovery scope has not changed"
- AND the recovery is admitted, validated only by `validateCompactRecoveryEdge`'s
  `RecoveryInvalidated` case, and persisted

#### Scenario: Base-tree mismatch still rejected for invalidated

- GIVEN the same invalidated predecessor as above
- WHEN `review recover --disposition invalidated` is run with a `--base-ref`
  whose resolved base tree differs from the predecessor's base tree
- THEN the CLI MUST still reject with "recovery base-ref does not match predecessor base"

### Requirement: `scope_changed` CLI gate MUST remain unchanged

The CLI unchanged-target gate MUST continue to reject
`--disposition scope_changed` recovery whenever the predecessor kind is
base-diff/overlay and the recomputed target `Identity` equals the
predecessor's initial snapshot `Identity`, exactly as before this change.

#### Scenario: Unchanged-target scope_changed still rejected at CLI level

- GIVEN a predecessor with initial snapshot kind `TargetBaseDiff` (or overlay)
- WHEN `review recover --disposition scope_changed` is run producing an
  identical target `Identity`
- THEN the CLI gate rejects with "recovery scope has not changed", unchanged
  from current behavior

### Requirement: `escalated` CLI and package gates MUST remain unchanged

The CLI unchanged-target gate MUST continue to reject
`--disposition escalated` recovery under the same conditions as today,
regardless of `--maintainer-authorization` validity. The package-level
`RecoveryEscalated` changed-target requirement
(`compactEscalatedRecoveryTargetChanged`, `errCompactRecoveryTargetUnchanged`)
MUST NOT be weakened.

#### Scenario: Unchanged-target escalated still rejected at CLI level

- GIVEN a predecessor with initial snapshot kind `TargetBaseDiff` (or overlay)
- WHEN `review recover --disposition escalated` is run with a fully valid
  `--maintainer-authorization`, producing an identical target `Identity`
- THEN the CLI gate rejects with "recovery scope has not changed", unchanged
  from current behavior

### Requirement: Base-tree mismatch guard MUST remain enforced for all dispositions

The `!*releaseScope && (baseDiff || overlay) && snapshot.BaseTree != predecessor
base tree` guard MUST continue to apply identically to `invalidated`,
`scope_changed`, and `escalated`.

#### Scenario: Base-ref mismatch rejected regardless of disposition

- GIVEN any predecessor with base-diff/overlay kind
- WHEN recovery is attempted with a `--base-ref` resolving to a different base
  tree, for any `--disposition` value
- THEN the CLI rejects with "recovery base-ref does not match predecessor base"

### Requirement: Regression coverage MUST prove `scope_changed`/`escalated` are untouched

The change MUST ship automated tests that positively assert unchanged-target
`scope_changed` and `escalated` recovery are still rejected at the CLI level
(not merely that `invalidated` now succeeds), plus a test proving unchanged-target
`invalidated` recovery is admitted.

#### Scenario: Test suite covers all three dispositions

- GIVEN identical unchanged-target base-diff/overlay setup fixtures
- WHEN the CLI-gate test suite runs for `invalidated`, `scope_changed`, and
  `escalated`
- THEN `invalidated` asserts success (or delegation past the CLI gate),
  `scope_changed` and `escalated` assert the "recovery scope has not changed"
  rejection
