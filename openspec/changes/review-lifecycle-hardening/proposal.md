# Proposal: Review Lifecycle Hardening

## Intent

Organic RDD recovery must release with zero known in-product traps. Eighteen reproduced defects still end the review lifecycle in a dead-end or a misleading terminal. Three block v2.2.0: pre-push published delivery misclassified as `authority_corrupted`, #1813 (one malformed lineage poisons the whole compact store; the kill switch is the only exit), and #1778 (25 s START deadline with no continuation). The remaining fifteen ship in the same batch by maintainer decision.

## Scope

### In Scope

| Group | Issues | Fix shape |
|---|---|---|
| A Error typing | published-delivery pre-push, 1699, 1666, 1807 | typed sentinels; surface the real cause instead of `authority_corrupted`/`operation_outcome_unknown` |
| B Executable contract | 1745, 1775, 1663, 1788, 1800 | transitions an agent can run literally; named schema and route |
| C Recovery graph | 1744, 1816, 1782 | eligible successors reach `validateCompactRecoveryEdge` |
| D Store robustness | 1813 | narrow exclusion of one invalid terminal lineage, with a user-visible diagnostic |
| E Limits | 1778 | START-specific deadline |
| F Platform fail-safes | 1781, 1804 | lock walk anchored at git common-dir; ENOTSUP fallback |
| G Target shapes | 1812, 1771, 1641 | typed refusal naming the proven in-product escape |

- Per-issue E2E traceability in `e2e/organicruntime`: one shared journey per group, one named subtest per issue carrying its number.

### Out of Scope

- 1787 budget plumbing, 1814 (needs SDD-runtime repro first), 1710 diff duplication, 1818 (no root cause).
- Building the features behind 1812/1641/1778/1745: index freezing, empty-base publication, configurable timeouts.
- New platforms, frameworks, or subsystems.
- The uncommitted #1825/#1827 fixes already in the tree.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `review-findings-ledger`: typed lifecycle failures, executable transitions, recovery admission, and store-discovery resilience.

## Approach

Short, robust fixes to existing code. Where a capability is genuinely missing, convert silent-broken behavior into a clear typed error or an executable form rather than building it. RED-first per issue, group by group in risk order E → A → B → G → F → C → D; C and D land last because they move fail-closed boundaries. Delivered in PR #1801 under the accepted `size:exception`.

## Affected Areas

`internal/cli/{review_facade,review_next_transition,review_schema,review_operation_contract}.go`, `internal/reviewtransaction/{gate,compact,compact_store,artifact_admission,secure_open_unix,publish_immutable_darwin}.go`, `e2e/organicruntime/`.

## Risks

| Risk | Mitigation |
|---|---|
| Group D exclusion reopens poisoning | narrow predicate plus surfaced diagnostic, never a silent skip |
| Group C shifts the fail-closed boundary deeper | confirm at design that `validateCompactRecoveryEdge` rejects every incompatible predecessor |
| Group A sentinels reclassify real corruption as benign | audit against the classification ladder in `review_facade.go` |
| Group F unverifiable on Linux | fail-safe default plus mocked-syscall units; community verifies |
| 1782/1663/1788/1800 not line-pinned | design pins exact anchors before RED tests |

## Rollback Plan

Each group is an independently revertible commit; `git revert` the group. Never force-push. The kill switch remains the runtime escape.

## Success Criteria

- [ ] Every one of the eighteen defects has a named E2E subtest carrying its issue number, RED before green.
- [ ] The three release blockers have an in-product continuation that does not require the kill switch.
- [ ] Pre-push before push, `--committed-only`, governing-receipt allow, and disabled/unmanaged stay byte-identical.
