## Exploration: Complete Native Review Lifecycle

### Current State
The native lifecycle already has the essential foundation: a repository-derived append-only CAS store, strict chain/successor validation, immutable initial snapshot fields, a frozen finding ledger, one ordinary fix batch, one scoped validation, terminal receipt generation, current-target gate evaluation, explicit resume, and bundle export/import. `CompleteFix` already binds a fix diff to the prior final candidate and exactly the frozen corroborated IDs; `ValidateFixDeltaResult` already permits only one ordinary scoped validation and escalates instead of starting another fix.

The current proposal/spec/design overextend that foundation. They introduce v2 schemas, operation journals, durable checkpoints, lineage discovery/retirement, causality taxonomy, merge dispositions, seals, gate contexts, archive cross-check bundles, and new command surfaces. Most are not necessary to enforce immutable scope, bounded correction, cheap retry recovery, or the existing quality bar.

### Affected Areas
- `internal/reviewtransaction/transaction.go` — reuse existing frozen findings, one-fix/one-validator counters, correction binding, escalation, and semantic successor checks; narrow the scoped-validator contract.
- `internal/reviewtransaction/snapshot.go` — add only immutable-genesis path-set comparison and reject a correction that adds paths outside it.
- `internal/reviewtransaction/store.go` — retain the repository-derived CAS store, exact-content append idempotency, and chain validation; do not add an operation journal.
- `internal/reviewtransaction/{receipt,gate,bundle}.go` — retain existing receipt/gate/bundle behavior only if already required by current lifecycle consumers; do not add seals, v2 formats, or new persisted context.
- `internal/cli/review.go` and existing tests — reuse `review-step`, `review-resume`, and validation output; add focused tests for freeze, bounded correction, target verification, and exact retry.
- `openspec/changes/complete-native-review-lifecycle/{proposal,design.md,specs/review-findings-ledger/spec.md}` — simplify the change contract before implementation.

### Approaches
1. **Minimal frozen-ledger lifecycle** — preserve existing v1 transaction/CAS mechanics; freeze the initial findings ledger, permit one correction only for frozen accepted/blocking IDs, then run targeted verification of the original acceptance criteria/tests plus a regression check for that correction.
   - Pros: Directly satisfies the maintainer policy; smallest schema/command/test change; existing one-fix/one-validator state machine already enforces the key budget.
   - Cons: New observations after freeze are recorded only as non-blocking follow-up text/JSON, not a new structured review entity.
   - Effort: Low

2. **Proposed v2 lifecycle platform** — add journals, operation IDs, retirement/discovery, taxonomy, seals, gate contexts, and portable-chain upgrades.
   - Pros: Broader forensic and recovery model.
   - Cons: Duplicates current CAS retry protection, increases schemas/commands/agent work, and turns targeted post-fix verification into a second review surface.
   - Effort: High

### Recommendation
Choose **Minimal frozen-ledger lifecycle**.

Keep/reuse: the canonical repository-derived CAS store, immutable initial snapshot identity, strict append/chain validation, existing exact-content append retry (`HEAD == identical revision`), frozen findings, ordinary one-fix and one-scoped-validation counters, receipt, gate result, and existing CLI JSON output.

Add only: (1) persist the immutable genesis path set/digest at start; (2) reject a correction before append when its changed paths are not a subset of genesis; (3) require correction ledger IDs to equal frozen accepted/blocking IDs; (4) redefine the single post-fix validator as targeted original-requirement/test verification plus regression proof; (5) record any post-freeze newly observed case as a non-blocking follow-up without changing ledger, scope, budget, or fix plan. If that observation proves the correction broke an original acceptance criterion, deny approval and require reject/revert/escalation—not scope expansion or another ordinary fix.

Delete/simplify from the current proposal: v2 migration and compatibility mapping; operation IDs, prepared/committed/output journals, and checkpoint schemas; lineage discovery and superseded/abandoned retirement; causality/introduced-by/merge-disposition taxonomy; seals, gate-context artifacts, and separate `review-seal`/`review-gate` producers; mandatory archive mirrors; expanded bundle bindings and clean-clone recovery work; external-evidence reopening; dual-scope persisted identities; and new broad black-box lifecycle permutations. A simple follow-up record is sufficient for a new post-freeze case. Existing exact append idempotency plus `review-resume` is sufficient cheap retry recovery.

Passing the frozen accepted/blocking ledger and the original acceptance criteria/tests is sufficient to proceed. The validator must not inspect the full diff as a new defect-discovery sweep.

### Risks
- A path-only subset check must use the same canonical snapshot/path derivation as genesis; otherwise renamed/deleted/untracked paths can evade the boundary.
- Existing `ValidateFixDeltaResult` accepts fix-caused findings; its contract must be narrowed so a post-freeze finding cannot become another correction input. It may only deny/escalate when it proves regression of an original acceptance criterion.
- The current proposal’s complete untracked-path disposition may be retained only if it is needed to make genesis scope complete; immutable evidence/audit records beyond that are not required for the minimal lifecycle.

### Ready for Proposal
Yes — replace the current broad lifecycle proposal with the minimal contract above before running spec/design. The downstream artifacts should explicitly state that freeze ends discovery, later observations never expand scope or trigger another fix, and rejection/revert/escalation is the sole response to a correction-caused failure of an original acceptance criterion.
