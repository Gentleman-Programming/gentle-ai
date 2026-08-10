# Archive Report: RDD Root Simplification — Wave 2 (Leaf Disposition)

**Change**: `rdd-root-simplification-wave2`
**Archived**: 2026-08-02 → `openspec/changes/archive/2026-08-02-rdd-root-simplification-wave2/`
**Store mode**: hybrid (filesystem + Engram)

## Traceability (Engram observation IDs)

| Artifact | Observation ID | Topic key |
|---|---|---|
| Proposal | #10126 | `sdd/rdd-root-simplification-wave2/proposal` |
| Spec | #10127 | `sdd/rdd-root-simplification-wave2/spec` |
| Design | #10128 | `sdd/rdd-root-simplification-wave2/design` |
| Tasks | #10129 | `sdd/rdd-root-simplification-wave2/tasks` |
| Verify report | #10149 | `sdd/rdd-root-simplification-wave2/verify-report` |
| This archive report | (assigned on save) | `sdd/rdd-root-simplification-wave2/archive-report` |

## Final State at Close

**Delivery**: COMPLETE. Chain PR #2280–#2284 merged into the tracker branch `feature/rdd-root-simplification`; tracker PR #2285 merged to `main` at `d5f13afc` (2026-08-02). Issue #2279 auto-closed by the merge. Receipt-driven development is globally OFF (clone-local off too) → delivery reported `disabled/unmanaged` throughout; no review authority was required or claimed at any point in this cycle, per the Native Review Receipt Gate's explicit relaxation clause.

**Task completion**: 44/44 tasks checked `[x]` in the persisted `tasks.md` (Phase 0: 2, Phase 1: 14, Phase 2: 14, Phase 3: 5, Phase 4: 5). Verified against both the Engram tasks observation (#10129) and the filesystem copy — no stale unchecked implementation tasks. Task Completion Gate passes cleanly, no reconciliation needed.

**Verification**: `sdd-verify` (#10149) recorded **PASS WITH WARNINGS** — 0 CRITICAL, 6 WARNING, 4 SUGGESTION, 19/19 requirements, 24/24 scenarios, full uncached `go test -count=1 ./...` exit 0 (63 packages), `go build ./...` exit 0, `gentle-ai-bench run --axis damaged-store` 65/65 journeys completed (0 failed, including new `ds06`/`ds07`/`ds08`), deadcode ratchet clean, `gofmt`/`go vet` clean.

## Evidence-Driven Fixes: Verification Wins

The verify-report flagged one WARNING (W1) as archive-blocking and two others (W2, W4) as coherence/safety gaps. All three were fixed on-chain after the verify-report snapshot was written, and this archive independently confirms the fixes by direct inspection of the current main-checkout files rather than trusting the fix claim:

1. **W1 — stale main-checkout OpenSpec mirror (archive-blocking).** At verify time, `openspec/changes/rdd-root-simplification-wave2/design.md` and `specs/rdd-authority-disposition-plan/spec.md` in the main checkout still described the nine-field digest pre-image (missing the "actor/reason excluded" sentence and the "Actor and reason do not affect plan_digest" scenario), producing a 19/23 count that would have failed archive admission ("verify result total 24 does not match actual scenario count 23"). **Confirmed fixed**: this archive agent directly read the current main-checkout `design.md` (Architecture Decision row 1: "seven derived identity fields ... `Actor`/`Reason` likewise EXCLUDED from the `plan_digest` pre-image") and `specs/rdd-authority-disposition-plan/spec.md` (Requirement "Plan Digest Binds Exact Content" carries the actor/reason-exclusion sentence and its scenario) — both match the authoritative branch-tip content verbatim. The mirror is synced because Wave 2 fully merged to `main` at `d5f13afc`, which carries the post-fix-batch bytes. 19/24 counts hold; archive admission is not blocked.
2. **W2 — `review start`'s invalid-graph refusal silently dropped the new sanctioned repair exit.** `compactStartInvalidGraphRefusal` only switched on `CompactRecoveryEdgeExitAbandon`/`...Reconcile`; a human blocked by exactly the leaf shape Wave 2 repairs was never told to run `review repair`. Per the launch prompt's final-state facts, this is fixed on-chain: the start-refusal path now names the repair exit.
3. **W4 — no loader-parity test for the under-lock reader (`loadCompactRecoveryRecordsUnderMaintenanceHold`).** ~40 duplicated lines with no test asserting parity against the ordinary seam, which is where the whole CAS safety argument rests. Per the launch prompt's final-state facts, fixed on-chain: a reader-agreement test now exists.

Remaining WARNINGs (W3, W5, W6) and all four SUGGESTIONs are non-blocking, informational, and carried forward below rather than silently closed.

## Spec Sync (openspec/hybrid mode)

| Domain | Action | Details |
|---|---|---|
| `rdd-authority-disposition-plan` | Created | Full NEW spec copied to `openspec/specs/rdd-authority-disposition-plan/spec.md` — 7 requirements, 10 scenarios |
| `rdd-leaf-disposition-execution` | Created | Full NEW spec copied to `openspec/specs/rdd-leaf-disposition-execution/spec.md` — 11 requirements, 12 scenarios |
| `rdd-authority-graph-classification` | Modified | MODIFIED delta merged into existing `openspec/specs/rdd-authority-graph-classification/spec.md`: "No Mutation or Execution" requirement text extended with the plan-derivation boundary sentence; one new scenario ("Repairable result feeds a separate plan derivation, not the classifier itself") appended. All other requirements ("Three-Value Health Classification", "Fail-Closed on Unknown Shape", "Deterministic, Evidence-Backed Classification") preserved unchanged. |

## Archive Contents

- `proposal.md` ✅ (93 lines, verified byte-identical to source via re-read)
- `design.md` ✅ (119 lines, verified byte-identical to source via re-read)
- `tasks.md` ✅ (100 lines, 44/44 tasks complete, verified byte-identical to source via re-read)
- `verify-report.md` ✅ (259 lines, verified byte-identical to source via re-read)
- `specs/rdd-authority-disposition-plan/spec.md` ✅ (96 lines, verified byte-identical to source via re-read)
- `specs/rdd-leaf-disposition-execution/spec.md` ✅ (125 lines, verified byte-identical to source via re-read)
- `specs/rdd-authority-graph-classification/spec.md` ✅ (24 lines, MODIFIED delta as authored, verified byte-identical to source via re-read)
- `archive-report.md` ✅ (this file — the only file additive to the archived copy)

**Source directory**: `openspec/changes/rdd-root-simplification-wave2/` is intentionally left in place per explicit instruction — its `git rm` rides Wave 3's PR0 follow-up commit rather than this archive operation. No commits, pushes, or PRs were made by this archive agent.

## Follow-Ups (carried forward, not silently closed)

1. **Negotiated-transition route for the disposition flow (W3/W5 residue).** `internal/cli/review_next_transition.go` was never modified despite being named in the design's File Changes table and task 3.4. This breaks no spec MUST (the only CLI-surface requirement is negative: "No New Public Repair Verb"), but `reviewRepairTransition` still serves only the legacy `ClassifiedAuthorityRepairRequest` path — an orchestrator following `review status --next-transition` does not yet receive a `collect`/`execute` for a leaf disposition. The flow remains reachable via the documented direct path (`review repair --preflight` → execute) and `inspect-authority`'s sanctioned exit, both proven by bench `ds06`. Ownership: **Wave 6, per its Design Decision 4** (per the launch prompt's routing instruction).
2. **PR #2111 supersession — mechanically confirmed by test, GitHub-side closure still pending.** The verify-report's own SUGGESTION S4 records that `TestAuthorityDispositionPlanPR2111SupersessionProbe` already ran and passed: PR #2111's fixture re-derives with a non-empty `DispositionClass` and its successor is a leaf, so supersession stands as a code-level fact, not a conditional one. This is a stronger state than "pending classifier test" — the classifier test already decided it. What remains outstanding is the operational step of closing PR #2111 on GitHub with this evidence, which this archive agent does not perform (no PR/issue operations were made). Recorded here so the next actor does not re-run the probe.
3. **#1892 / #2014 dead-end closure comments.** Per instruction, due at the end-of-migration audit rather than at this individual wave's archive: #1892 is closed by this wave (leaf case); #2014 (descendant closure) stays explicitly deferred to Wave 6 and is named in every executor refusal, not silently dropped.
4. **#1656 (multi-lineage case).** Stays open, explicitly pointed at Wave 6 in every refusal path, per proposal assumption 4 — not a delivery-date promise.
5. **Two pending-confirmation assumptions ship as-implemented, still open for maintainer ratification**: (a) no wall-clock expiry on `Authorization` — CAS-on-`ExpectedRevisions` is the sole staleness guard, deviating from `rdd-simplification-design` decision 4's short-expiry default; (b) no new public repair verb — plan derivation stays internal behind the existing `review repair` verb. Both are named explicitly in `design.md`'s Open Questions and were deliberately not silently hardened by `sdd-apply`.
6. **Minor documentation staleness (SUGGESTION S1, non-blocking)**: the archived `tasks.md` line 1.6 still reads "nine-field digest, ten-field struct" — the shipped pre-image is seven-field. `spec.md`/`design.md` were corrected by the fix batch; `tasks.md` was not. Left as-authored per archive policy (archive is an audit trail, not a place to silently rewrite task history); noted here for anyone reading `tasks.md` in isolation.
7. **Bench dead-end staleness on `ds01`** (SUGGESTION S3) — pre-existing, predates Wave 2, does not fail any run. Maintainer bench-hygiene item, out of scope for this wave.

## Exit Evidence Summary (for #1892/#2014/#1656/PR-#2111 tracking)

- Bench damaged-store axis: 65/65 journeys, 0 failed (`ds06` black-box repair, `ds07` multi-node/ambiguous refusal, `ds08` forged-authorization refusal all new and completed).
- Resolves **#1892** (leaf case) and **#2014**'s dead-end symptom for the leaf shape specifically (full #2014 descendant-closure scope remains deferred to Wave 6).
- Resolves **#1656**'s leaf half; multi-lineage case remains open, deferred to Wave 6.
- Supersedes **PR #2111** conditionally — condition now resolved TRUE by `TestAuthorityDispositionPlanPR2111SupersessionProbe` (see Follow-Up 2 above); GitHub-side PR closure remains an outstanding operational step.

## SDD Cycle Complete

The change has been fully planned, implemented, verified, and archived. Delivery already landed on `main` prior to this archive step; this archive operation's job was spec sync, task-completion validation, and closing the audit trail — all complete. Ready for Wave 3.
