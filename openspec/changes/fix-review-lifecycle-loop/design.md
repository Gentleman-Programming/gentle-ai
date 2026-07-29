# Design: Fix Review Lifecycle Loop

## Technical Approach

Patch the existing compact-v2 seams only. Receipts bind `FinalCandidateTree` to `CurrentSnapshot` and retain the initial path digest as the frozen authorized envelope. `EvaluateCompactGate` requires that exact final tree and permits only live paths within that envelope, including a subset after a full path revert. No schema, authority, gate set, or archive workflow changes.

## Architecture Decisions

| Decision | Choice and rationale | Rejected alternative |
|---|---|---|
| Final identity | `CompactState.Receipt` always uses `CurrentSnapshot.CandidateTree` with `InitialSnapshot.PathsDigest`; the latter represents the immutable genesis path envelope, not the correction delta. | Add receipt v3/`FinalScope`; unnecessary migration and consumer churn. |
| Persisted v2 | Regenerate historical and current v2 fields exactly without a candidate-defined legacy predicate; never rewrite state or receipt bytes. | Rewrite stored authority or relax receipt equality; both weaken immutable authority. |
| Denial action | Map `GateScopeChanged` to the existing `explicit-maintainer-action` value. Keep non-success exit behavior and make no allocation call. | Automatic lineage/review/budget creation or a lifecycle coordinator. |
| Archive contract | Enumerate only `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, and `release`; state that archive still requires structured status with `reviewGate.result: allow` and its approved receipt. | Add unsupported `--gate archive`, binding, or coordinator. |

The abandoned `stabilize-review-lifecycle` architecture—native authority resolution, durable recovery, hook coordination, portable recovery, archive binding, and new stores—is explicitly out of scope.

## Data Flow

```text
validated CurrentSnapshot tree + frozen genesis path envelope
        -> CompactState.Receipt(v2)
        -> persisted receipt equality
        -> EvaluateCompactGate rebuilds live snapshot
        -> allow | fail-closed maintainer action
```

Legacy persisted v2 follows only `loaded state -> historical receipt regeneration`; it does not become the emission path for a new correction.

## Interfaces / Contracts

- Keep `gentle-ai.review-state/v2`, `gentle-ai.review-receipt/v2`, `CompactReceipt`, and JSON fields unchanged.
- Keep supported `GateKind` values unchanged.
- Preserve `ReviewValidateResult.action`: `continue`, `explicit-maintainer-action`, or `stop`; remove `create-new-lineage` for `scope-changed`.
- Golden output is generated from `RenderTriggerRules`, not edited independently.

## File-Action Forecast

| File | Action | Forecast |
|---|---|---:|
| `internal/reviewtransaction/compact.go` | Modify receipt derivation/legacy predicate | 15–25 |
| `internal/reviewtransaction/compact_store_test.go` | Persisted-v2 reload/regeneration RED test | 35–55 |
| `internal/reviewtransaction/compact_gate_test.go` | Corrected intended-untracked allow/no-mutation RED test | 55–85 |
| `internal/cli/review.go`, `review_test.go` | Action mapping and table test | 15–25 |
| `internal/components/sdd/triggerrules.go`, `triggerrules_test.go` | Maintainer wording and forbidden-auto-action assertions | 10–18 |
| `internal/components/testdata/golden/trigger-rules-default.golden` | Regenerate | 2–4 |
| `internal/assets/skills/_shared/review-ledger-contract.md`, `internal/components/sdd/bounded_review_contract_test.go` | Correct gate list and archive assertion | 15–25 |

Total: 10 modified, 0 created/deleted, approximately 180–300 authored lines. Target `<400`; stop and re-plan before 800. Generated golden lines remain in snapshot identity but are excluded from authored-risk count.

## Testing Strategy

Strict TDD: first persist a manual historical v2 state/receipt fixture, reload both, regenerate, and assert receipt equality/unchanged bytes. Then prove corrected pre-push and pre-PR delivery accepts a subset of genesis paths while rejecting new paths, retaining exact tree and intended-untracked proof. Add CLI action-table, trigger positive/negative, golden, and embedded-contract tests. Run focused package tests, then `go test ./...` and `go vet ./...`.

## Threat Matrix

| Boundary | Applicability | Safe/failure behavior and RED tests |
|---|---|---|
| Documentation-like paths | N/A — no executable classification changes | Existing snapshot classification unchanged. |
| Git repository selection | N/A — no `git -C`, cwd, or path authority changes | Existing root resolution unchanged. |
| Commit state | N/A — no index/worktree semantics change | Existing staged/intended transition suite remains required. |
| Push state | N/A — no push/refspec logic change | Existing pre-push suite remains required. |
| PR commands | N/A — no command composition or `--head` logic change | Existing pre-PR suite remains required. |

## Migration / Rollout / Rollback

No migration or feature flag. Ship as one focused PR. Roll back by reverting it; schemas and stored bytes remain readable. Rollback restores the loop and old guidance, so retain the regression tests with the fix.

## Open Questions

None.
