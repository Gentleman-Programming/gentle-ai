# Proposal: Complete Native Review Lifecycle

## Intent

Replace #1104's broad v2 platform with one low-token frozen-ledger lifecycle for every catalog-supported Gentle AI agent/runtime: one finite correction set, never an iterative loop.

## Scope

### In Scope
- Freeze accepted/blocking IDs after exactly one discovery review of immutable genesis paths.
- Permit one correction restricted to frozen IDs and genesis paths.
- Validate once with original acceptance tests and correction regression proof, without new discovery.
- Keep later observations non-blocking; reject, revert, or escalate original-criterion regressions.
- Deliver behavior once through native core and shared contracts to every supported runtime.

### Out of Scope
- v2 migration, journals/checkpoints, lineage management, or causality taxonomy.
- New seals/gates or expanded bundle/archive behavior.
- Broad permutations or another discovery/fix cycle.
- Per-agent lifecycle logic or verbose duplicated prompts.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `review-findings-ledger`: Make genesis scope immutable and ordinary review a single discovery, frozen blocking ledger, optional bounded correction, and targeted non-discovery validation lifecycle.

## Approach

Reuse v1 CAS, semantic chaining, retries, `review-resume`, receipts, gates, and bundles. Add genesis path enforcement, exact frozen-ID binding, targeted evidence, and follow-ups only in native core. Render one concise shared contract into agent assets. Drive parity tests from `catalog.AllAgents()` so stale or new adapters fail. Preserve the accepted `size:exception`.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/reviewtransaction/{transaction,snapshot}.go` | Modified | Frozen IDs, path boundary, correction/validation invariants |
| `internal/components/sdd/{boundedreview.go,bounded_review_contract_test.go}` | Modified | Shared contract rendering and catalog parity |
| `internal/cli/review.go` and native tests | Modified | Minimal inputs, follow-ups, lifecycle coverage |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Validation or correction expands scope | Medium | Enforce genesis paths, frozen IDs, and targeted test inputs before append |
| An adapter retains stale discovery/fix prose | Medium | Catalog-driven shared-asset assertions fail for every supported AgentID |

## Rollback Plan

Revert core and shared-contract commits together; retain existing append-only v1 stores without migration.

## Dependencies

- Approved #1104; existing v1 review lifecycle.

## Success Criteria

- [ ] Genesis paths never expand; correction maps exactly to frozen accepted/blocking IDs.
- [ ] One discovery review and at most one correction are enforced.
- [ ] Post-fix validation uses original tests plus focused regression proof without discovery.
- [ ] Later observations remain non-blocking; original-criterion regressions reject, revert, or escalate.
- [ ] Every `catalog.AllAgents()` runtime receives the same concise lifecycle contract; stale discovery/fix behavior fails parity tests.
- [ ] Existing retry, receipt, gate, and bundle behavior remains compatible.
