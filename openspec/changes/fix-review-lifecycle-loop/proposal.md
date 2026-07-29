# Proposal: Fix Review Lifecycle Loop

## Intent

Fix compact-v2 identity so corrected reviews validate final scope instead of looping on `scope-changed`. Keep denials fail-closed and gates accurate.

## Scope

### In Scope
- Bind the receipt to the final candidate tree and frozen genesis path envelope, including intended-untracked proof.
- Preserve legacy-v2 persisted receipt regeneration without migration.
- Make `scope-changed` output/rules require maintainer action; create no lineage, review, or budget.
- Remove `archive` from the generic gate promise; preserve existing archive checks.
- Add focused receipt, gate, CLI, renderer, golden, and asset tests.

### Out of Scope
- The abandoned broad `stabilize-review-lifecycle` architecture: native authority resolution, durable lifecycle/recovery, hook coordination, and portable recovery.
- Receipt v3/migration, archive binding/gate, lifecycle coordinator, or authority store.
- Automatic lineage/generation/budget/reviewer work or legitimate scope recovery.
- Archive readiness, review algorithms, or CLI gate-set changes.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `review-findings-ledger`: Final-scope identity, v2 compatibility, denial action, and gates.
- `organic-agent-trigger-rules`: Maintainer-only `scope-changed` action.
- `sdd-orchestrator-assets`: Embedded contract and assertion parity.

## Approach

Derive the final tree from the current snapshot while preserving the initial path digest as the maximum authorized envelope, lock legacy regeneration with a persisted-v2 fixture, update CLI/trigger wording, and narrow the shared gate list. Use one focused PR; target under 400 authored lines and re-plan before 800.

## Compatibility

- Keep schema v2 and field shape; persisted fixtures reload and regenerate as expected.
- Existing consumers remain valid; corrected receipts gain a final-scope-consistent digest.
- Archive continues through existing status/receipt validation, not a new CLI gate.

## Affected Areas

| Area | Impact |
|------|--------|
| `internal/reviewtransaction/compact.go`, focused tests | Receipt identity/compatibility |
| `internal/cli/review.go`, tests | Denial action |
| `internal/components/sdd/triggerrules.go`, tests, golden | Rendered guidance |
| `internal/assets/skills/_shared/review-ledger-contract.md`, assertion | Gate promise |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Legacy regeneration drifts | Medium | Persisted-v2 equality fixture |
| Final scope drops untracked proof | Medium | Final-tree/final-paths allow test |
| Archive contract weakens | Low | Assert current checks remain |

## Rollback Plan

Revert the PR. No migration or stored-state rewrite is needed; v2 remains parseable. The loop returns.

## Dependencies

- Existing compact-v2 and guidance seams only.

## Success Criteria

- [ ] Persisted legacy v2 reloads and regenerates as expected.
- [ ] Corrected final scope, including intended-untracked content, validates `allow`.
- [ ] `scope-changed` JSON/rules require a maintainer and contain no automatic lineage, review, or budget.
- [ ] Guidance omits an `archive` CLI gate while retaining the existing archive contract.
- [ ] Regressions pass; authored change stays under 400 lines, or stops before 800.

## Follow-ups

- Separately propose maintainer-approved lineage recovery or archive lifecycle semantics only if justified.
