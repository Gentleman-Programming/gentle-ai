# Proposal: Distinguish Unachievable Lens Attempts from Unattempted Slots

## Intent

Distinguish unachievable reviewer lens attempts from unattempted slots via validated non-admitted attempt evidence, retry accounting, truthful operational reason codes, and strict finalize blocking, resolving infinite re-offer loops and misleading corruption stops (Issue #3442, PR #3520).

## Scope

### In Scope
- Validated provider evidence bound to `(lineage, target, revision, lens, selected order)`.
- Durable persistence and discovery of attempted/unachievable slots in `internal/reviewtransaction` without admitting them as completed reviewer results.
- Per-slot retry accounting tracking attempts against maximum retry policy.
- Distinct truthful operational reason code (`unachievable_reviewer_attempt`) and continuation in `newReviewNextTransition` and `reviewFinalizeNextTransition`.
- Updates to `docs/review-integration.md`, golden files, and invariant tables (`internal/cli/review_stop_invariant_test.go`).
- Strict `review.finalize` and receipt issuance blocking when any required lens is unachieved.
- Black-box host/provider regression test suite.

### Out of Scope
- Altering retry policies for targeted validation / verification evidence.
- Changing passing lens result admission criteria.
- External provider fallback routing beyond configured retry bounds.

## Capabilities

### New Capabilities
None

### Modified Capabilities
- `rdd-review-core-transitions`: Add `unachievable_reviewer_attempt` stop transition, non-admitted attempt discovery, retry accounting, and finalize blocking.
- `review-findings-ledger`: Persist and discover non-admitted reviewer attempt evidence bound to lens slot identity.

## Approach

- Extend `internal/reviewtransaction` with durable attempt persistence and slot discovery for unachievable reviewer attempts without admitting them into completed result slots.
- Implement per-slot retry tracking against a configurable attempt threshold.
- Update `internal/cli/review_next_transition.go` and finalize transitions to emit `unachievable_reviewer_attempt` instead of `captured_artifacts_unverifiable` or infinite collect re-offers.
- Enforce strict blocking in `review.finalize` preventing terminal receipt issuance if any required lens slot is unachieved.
- Update documentation, invariant classification tables, and black-box regression tests.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/reviewtransaction` | Modified | Attempt persistence, retry accounting, and non-admitted slot discovery |
| `internal/cli/review_next_transition.go` | Modified | Route unachievable attempts to `unachievable_reviewer_attempt` |
| `internal/cli/review_artifact.go` | Modified | Non-admitted attempt evidence capture & validation |
| `internal/cli/review_facade.go` | Modified | Finalize blocking and receipt prevention on unachieved slots |
| `internal/cli/review_stop_invariant_test.go` | Modified | Invariant table update for `unachievable_reviewer_attempt` |
| `docs/review-integration.md` | Modified | Document operational reason code and continuations |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Attempt replay or store tampering | Low | Cryptographically bind attempt evidence to `(lineage, target, revision, lens, order)` |
| Flaky re-offer loops on transient provider faults | Low | Bounded retry accounting before terminal stop transition |

## Rollback Plan

Revert PR slice. Transaction store schema maintains backwards compatibility as unadmitted attempt records are ignored by older versions.

## Dependencies

- Issue #3442 / PR #3520

## Success Criteria

- [ ] Provider refusals / unachievable attempts persist valid attempt evidence without completed admission.
- [ ] STATUS reports `unachievable_reviewer_attempt` once retries exhaust instead of infinite re-offers or `captured_artifacts_unverifiable`.
- [ ] `review.finalize` and receipt issuance strictly fail when any required slot is unachieved.
- [ ] Black-box host/provider regression tests pass cleanly.
