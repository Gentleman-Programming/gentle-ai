## Exploration: fix-review-lifecycle-loop

### Current State
`CompactState.Receipt()` binds `FinalCandidateTree` from `CurrentSnapshot`, but binds `PathsDigest` from `InitialSnapshot`. A correction can therefore produce a receipt with a final tree and an initial-scope digest; `EvaluateCompactGate` compares both against the live final snapshot and returns `scope-changed`. The snapshot builder already includes intended-untracked files in its candidate tree and proves each path against that tree, so the correction must preserve that final-scope proof rather than invent a separate archive binding.

The clean target contains compact state/receipt v2 only. Previous evidence records a v2 persisted-receipt versus newer-generation mismatch; the minimal compatibility rule is therefore to preserve the persisted legacy v2 receipt shape when regenerating it and to bind a corrected/current receipt to its final scope. This change must not introduce a v3 schema, archive binding, or new authority store.

Gate evaluation is read-only, but the CLI serializes `scope-changed` as `create-new-lineage`. Rendered trigger rules repeat that automatic instruction. The existing prose already says gates do not launch lenses or budgets, so the smallest correction is to make every non-allow lifecycle denial require explicit maintainer action and lock that output down with tests.

The CLI accepts only `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, and `release`. The shared review-ledger contract nevertheless names `archive` as a `--gate <gate>` boundary. Remove that unsupported promise; archive continues to consume structured status and an existing allowed receipt, not a new CLI gate.

### Affected Areas
- `internal/reviewtransaction/compact.go` — make receipt identity use the authoritative final correction scope while retaining legacy-v2-compatible emission.
- `internal/reviewtransaction/compact_store_test.go` and/or `internal/reviewtransaction/compact_gate_test.go` — add persisted-v2 reuse and corrected final-tree/final-paths gate regressions, including intended-untracked proof semantics.
- `internal/cli/review.go` and `internal/cli/review_test.go` — replace the scope-changed auto-action and assert serialized denial actions.
- `internal/components/sdd/triggerrules.go`, `triggerrules_test.go`, and `internal/components/testdata/golden/trigger-rules-default.golden` — replace automatic lineage creation with explicit maintainer action in generated guidance.
- `internal/assets/skills/_shared/review-ledger-contract.md` and its focused asset assertion — remove archive from the generic `--gate <gate>` promise.

### Approaches
1. **Repair receipt identity and guidance at existing seams** — preserve legacy v2 receipt generation, bind current corrected receipts to the final snapshot, change the CLI/rendered action to explicit maintainer action, and narrow the shared boundary list.
   - Pros: fixes the observed gate loop without new lifecycle ownership; exercises existing compact, CLI, renderer, and asset seams.
   - Cons: does not automate a new lineage after a legitimate scope change.
   - Effort: Low.

2. **Add an archive gate and lifecycle coordinator** — create an archive gate/state, binding schema, and command path.
   - Pros: could model archive as a first-class lifecycle transition.
   - Cons: unsupported by the current CLI, expands authority/state-machine surface, and violates the explicit non-goals.
   - Effort: High.

### Recommendation
Choose approach 1. Keep the fix to receipt reconstruction, denial presentation, and the two existing guidance sources. Regression coverage should prove: (1) a stored legacy-v2 authority can be reloaded without a receipt-generation mismatch; (2) a completed correction with a final candidate plus intended-untracked file validates against its final paths digest; (3) a scope-changed response never directs automatic lineage creation; and (4) shared guidance never implies an archive gate exists.

Forecast: 8-9 files, approximately 180-300 authored changed lines (well below the 400-line target; stop and re-plan before 800). One focused PR is appropriate.

### Risks
- The clean baseline exposes only compact receipt schema v2. The proposal/design must use a real persisted-v2 fixture to define compatibility precisely; do not infer or add a v3 schema from abandoned branch artifacts.
- A legitimate scope change still needs a maintainer decision. This intentionally trades automatic recovery for fail-closed lifecycle behavior; a separately approved recovery workflow remains follow-up work.
- Archive remains gated by its existing structured status/allowed receipt contract, not by a new `review validate --gate archive` command.

### Ready for Proposal
Yes — propose the focused compact receipt, denial-action/guidance, and regression-test change only. Follow-up work may separately design maintainer-approved lineage recovery or archive lifecycle semantics if still needed.
