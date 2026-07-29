# Design: Complete Native Review Lifecycle

## Technical Approach

Keep `gentle-ai.review-transaction/v1`, the repository-derived CAS chain, and existing CLI commands. Add only native immutable-genesis and targeted-validation rules, then expose them through the one embedded lifecycle contract already rendered into every agent. No adapter receives lifecycle code or a copied prompt.

## Architecture Decisions

| Decision | Choice | Rationale / rejected alternative |
|---|---|---|
| Genesis boundary | During `CompleteFix`, require canonical fix `Snapshot.Paths` to be a subset of the pre-fix `transaction.Snapshot.Paths`; repeat the comparison in `validateSuccessor`. Retain the genesis `PathsDigest` already stored on `Transaction`. | The start snapshot already contains the exact canonical set. A second scope model or v2 schema would duplicate authority; digest equality would reject valid subset fixes. |
| Frozen correction ledger | Keep existing equality among `FixFindingIDs`, method `ledgerIDs`, and fix-snapshot `LedgerIDs`. | Existing corroboration routing already produces the accepted/blocking correction set; no causality taxonomy is needed. |
| Ordinary validation | Extend `ScopedValidationResult` with compact original-criteria and correction-regression evidence/results plus structured non-blocking follow-ups. The public CLI always emits a distinct evidence-bearing CAS operation. Retain the internal v1 boolean transition only for concrete legacy consumers and fixtures. | This proves non-discovery validation at the shipped boundary without forcing unrelated v1 callers into this correction scope. |
| All-agent delivery | Edit only `internal/assets/skills/_shared/review-ledger-contract.md`; continue expansion through `boundedReviewContract`, `renderBoundedReviewAsset`, and `renderSDDOrchestratorAsset`. | `sddOrchestratorAsset` already maps every catalog agent to an agent-family or generic source. Editing those sources separately would create stale copies. |

## Data Flow

```text
catalog.AllAgents ─→ renderSDDOrchestratorAsset ─→ one shared contract

genesis paths + frozen IDs ─→ CompleteFix ─→ CAS successor check
original tests + regression proof + follow-ups ─→ targeted validation
                                                ├─ both pass: ready
                                                └─ either fails: escalated
```

Follow-ups never enter `Findings`, `Outcomes`, `FixFindingIDs`, scope, or counters. Even severe follow-ups are informational unless their evidence fails an original criterion; that failure escalates instead of reopening discovery or correction.

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/reviewtransaction/snapshot.go` | Modify | Add one canonical path-subset helper using existing normalized `Snapshot.Paths`. |
| `internal/reviewtransaction/transaction.go` | Modify | Enforce subset before mutation; add targeted ordinary evidence/follow-ups and derived result. |
| `internal/reviewtransaction/store.go` | Modify | Reject hash-valid successors that expand paths or violate targeted-validation immutability. |
| `internal/cli/review.go` | Modify | Decode the extended nested validation and record the targeted CAS operation; no command/schema surface. |
| `internal/reviewtransaction/{transaction,store}_test.go` | Modify | Native RED tests for scope, ledger, evidence, follow-ups, and tampering. |
| `internal/cli/review_test.go` | Modify | RED test for authoritative targeted-validation append and resume. |
| `internal/assets/skills/_shared/review-ledger-contract.md` and embedded agent assets | Modify | Replace fix-touched-line/new-defect language with original-test/regression-only validation and immutable scope; refresh shipped copies from the same semantic contract. |
| `internal/components/sdd/bounded_review_contract_test.go` | Modify | Iterate `catalog.AllAgents()` and enforce required and forbidden lifecycle clauses. |
| `internal/components/sdd/review_ledger_contract_test.go` | Modify | Reuse the same clause assertions for dedicated reviewer assets/OpenCode overlays. |

`internal/components/sdd/boundedreview.go` and per-agent prompt files remain unchanged: they are the existing shared expansion path, not customization points.

## Interfaces / Contracts

Ordinary `ScopedValidationResult` carries frozen ledger IDs, two SHA-256-bound check results (original criteria and correction regression), and an explicit follow-up array. Both evidence identities are mandatory on pass or fail. `Approved` is derived from the two results; mismatch or missing/stale evidence is rejected before append. Judgment Day retains its current bounded re-judgment behavior.

## Testing Strategy

Strict TDD order: first write RED tests for subset acceptance and added/renamed/deleted/untracked path rejection without budget/state mutation; exact frozen-ID binding; missing/stale/failed evidence; non-routing follow-ups; semantic-successor tampering; CLI append/resume; then catalog parity. The parity test must fail rendered output containing stale broad-review phrases such as `review only fix-touched lines`, `may append fix-caused defects`, or language permitting new findings, scope expansion, another validator, or another correction. Only then change production/assets. Run focused package tests, then `go test ./...` and `go vet ./...`.

## Threat Matrix

| Boundary | Applicability | Design response / RED test |
|---|---|---|
| Documentation-like paths | N/A — no executable classification changes | None. |
| Git repository selection | N/A — existing canonical root and `git -C` remain unchanged | Existing snapshot/store tests. |
| Commit state | N/A — index/worktree construction is unchanged | Existing staged/current-change tests. |
| Push state | N/A — no push automation | None. |
| PR commands | N/A — no PR command construction | None. |

## Migration / Rollout

No migration, feature flag, v2 schema, per-agent rollout, or changes to receipts, gates, bundles, retry, or `review-resume`. Roll back native and shared-contract changes together; existing CAS chains remain readable.

## Open Questions

None.
