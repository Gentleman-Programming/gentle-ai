# Proposal: Relax CLI unchanged-target recovery gate for `--disposition invalidated`

## Intent

`gentle-ai review recover` refuses ANY recovery when the successor target identity
equals the predecessor's, for every `--disposition`. The CLI-level gate in
`internal/cli/review_facade.go:518-519` (`RunReviewRecover`) is over-broad: it fires
for a base-diff / workspace-overlay predecessor before `RecoverCompactAuthority` runs,
preempting the disposition-aware package validation. This blocks legitimate
`--disposition invalidated` recovery of an unchanged target (e.g. redoing a review that
was invalidated by external evidence like a base advance or new CVE, against the same
candidate) even though the package-level `validateCompactRecoveryEdge` `RecoveryInvalidated`
case imposes NO changed-target requirement. Issue #1554 carries `status:approved` (approved
scope: the narrow `--disposition escalated` final-verification-retry contract); this slice
covers the independent, lower-risk `invalidated`-disposition sub-bug flagged as safe to fix
on its own in the issue's first comment, kept intentionally narrow.

## Scope

### In Scope
- Scope the CLI gate at `review_facade.go:518-519` so `recovery scope has not changed`
  no longer fires when `--disposition invalidated`.
- Failing-first test proving unchanged-target `invalidated` recovery on a base-diff/overlay
  predecessor is admitted (delegated to package validation), then made to pass.

### Out of Scope
- `--disposition escalated` and `validateCompactRecoveryEdge` `RecoveryEscalated` /
  `errCompactRecoveryTargetUnchanged` invariant (#1419/#1429 security invariant — MUST NOT weaken).
- `--disposition scope_changed` (see Approach — verified must stay gated).
- Any change to the `RecoveryDisposition` enum, the `v1` authorization binding schema, or `ReconcileInvalidRecoveryEdge`.
- The `--disposition escalated` / dedicated `review retry-final-verification` operation (separate, larger, tracked against the same issue).

## Capabilities

### New Capabilities
- `review-recovery-gating`: CLI-level pre-persist gating of `review recover` by disposition, and its delegation to package-level `validateCompactRecoveryEdge`.

### Modified Capabilities
None.

## Approach

Narrow the condition at line 518 so it does not apply when
`RecoveryDisposition(*disposition) == RecoveryInvalidated`, letting the existing
`RecoverCompactAuthority` → `validateCompactRecoveryEdge` path (which for `invalidated`
requires only a `StateInvalidated` predecessor) be the single authority. Keep the base-tree
guard at line 515-516 intact. `escalated` remains blocked by BOTH gates; other dispositions
unchanged.

**scope_changed verified — stays gated.** `compactRecoveryScopeChanged` (compact_store.go:303)
DOES require a real change, but compares `predecessor.CurrentSnapshot` vs `successor.InitialSnapshot`
on `CandidateTree`/`PathsDigest`/`BaseTree` — not equivalent to the CLI gate's
`snapshot.Identity` vs `predecessor.InitialSnapshot.Identity`. Relaxing it is not a provable
no-op, and "scope has not changed" is semantically correct for a scope-change disposition.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/cli/review_facade.go` (~518) | Modified | Exclude `invalidated` from the unchanged-target gate |
| `internal/cli/review_facade_test.go` (or sibling) | New | Test unchanged-target `invalidated` admitted; `escalated`/`scope_changed` still blocked |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Weakening #1419 escalated invariant | Low | Change touches only the `invalidated` branch; package gate untouched; regression test asserts escalated still blocked |
| Accidentally relaxing scope_changed | Low | Condition keyed exactly to `RecoveryInvalidated`; explicit test |

## Rollback Plan

Single-file, single-condition change: revert the `review_facade.go` edit and drop the added test. No data/schema/migration impact.

## Dependencies

- None. Independent of #1419's reconcile path (orthogonal per exploration).

## Success Criteria

- [x] Unchanged-target `review recover --disposition invalidated` on a base-diff/overlay predecessor succeeds.
- [x] Unchanged-target `escalated` and `scope_changed` recovery still rejected.
- [x] Base-tree mismatch guard still enforced for all dispositions.
- [x] Opened as PR 1 of a 2-PR stack against #1554.
