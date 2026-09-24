# ga-1380 WU1: decision_required + review decide + continue-consumer

Claimed 2026-09-21 (issue comment "Taking this one" + Design note v2, 17:06Z). Branch
feat/1380-decision-required (worktree ~/gentleman/gentle-ai-1380) from origin/main a773ccfb.
Design note v2: posted on the issue; Alan's 20/07 direction absorbed (compact v2 engine only;
state+decide+consumer land together; carry forward #1433's CAS and finding-state handling).
Engine unchanged on main since the 21/09 grounding (verified: no commits to
internal/reviewtransaction since 2026-09-21).

## Design decisions resolving the map's open points (2026-09-23, orchestrator)

1. Decision-eligible set = EVERY non-empty unresolved set at CompleteReview
   (missing_refuter_outcome, insufficient_evidence, unknown_causality, AND the
   unresolved_severe_findings catch-all): the pause catches the whole residual
   complete-review escalation population; the specific cause is recorded in the Decision
   block. targeted_validator_rejected and correction_budget_exceeded stay terminal (they
   transition from correction paths, not complete-review).
2. Cause attribution moves to transition time: derive the escalation-cause classification
   inside/before the transition (EscalationEvidence stays escalation-gated for projection;
   the Decision block carries finding_ids + cause + attempted/missing evidence).
3. No compact receipt revival (API retired + guarded). Terminal projections = status +
   target_status + narration; escalated-after-decide renders exactly like
   escalated-after-complete-review.
4. decision_required is NOT abandon-eligible: explicitly exempt from
   compactAbandonTerminalState; the sanctioned exit is review decide (continue|stop).
5. Idempotent replay mirrors abandon's request-equality replay (lineage+revision+decision+
   actor+reason), NOT the store's byte-identical short-circuit (journaling makes states
   differ).
6. review decide registers as a VERB-ONLY operation row (capabilities enum stays pinned).
7. DecisionEpoch is its own field with its own Validate invariant; never advanceCapturePhase
   (that rewrites CapturePhaseRevision bindings).
8. authorityStatusForState: decision_required maps explicitly (active-family, non-terminal).

## Tasks

1. [x] RED: state + successor truth rows + lifecycle validation
   (StateDecisionRequired; complete-review -> decision_required; review/decide rows
   decision_required->reviewing and ->escalated; Validate case: decision block present,
   DecisionEpoch >= 1, DecisionHistory non-empty).
2. [x] RED: CompleteReview pause — unresolved set => decision_required with Decision block
   (finding_ids, reason evidence_inconclusive, cause, attempted_evidence, missing_evidence,
   recommendation, next_action, bounded_cost), not escalated; clean/admitted path unchanged.
3. [x] RED: compact_decide.go — CAS via Replace (CompactRevisionConflictError typed path),
   continue/stop transitions, mandatory actor/reason journaled BEFORE append,
   request-equality idempotent replay, closed decision vocabulary (continue|stop).
4. [x] RED: decide-stop produces an escalated record passing existing escalated lifecycle
   validation (Decision block + history retained; EscalationEvidence renders).
5. [x] RED: STATUS projection — decision section present IFF decision_required
   (coupling validation mirroring escalation's), typed envelope gentle-ai.review-decision/v1
   with two Choice.Invocation runnables (review decide --decision continue / stop twins with
   current expected-revision) + per-choice effect text; next_transition exempts
   decision_required from the terminal-stop collapse and names its exit.
6. [x] RED: CLI RunReviewDecide — flags --lineage/--expected-revision/--decision/--actor/
   --reason, refusal text naming where to read values (abandon precedent), dispatch
   case "decide" verb-only in review_operation_contract.go; narration keys registered both
   directions.
7. [x] GREEN per stage, then full regression: ./internal/reviewtransaction/... and
   ./internal/cli/... suites, envelope bytes round-trip test + schema/fixture for
   decision-v1 (consent precedent), gofmt/vet/build clean.
8. [x] Docs: this feature doc evidence; short docs update only if a natural review-commands
   doc home exists (no new docs surface invented).
9. [ ] Work-unit commit; RDD native review; PR (Closes #1380 WU1; size:exception expected
   and documented).

## Evidence

Engine (tasks 1-4): RED observed as build failure — undefined StateDecisionRequired /
CompactDecisionEntry / DecisionHistory / DecideCompactStore across the new tests
(`go test ./internal/reviewtransaction/ -run '...Decision...'` → [build failed]). GREEN:
6 new engine test functions pass (pause, clean-path unchanged, successor truth rows incl.
12 refusal rows + 2 accepted rows, Validate contract incl. decide-stop/decide-continue,
DecideCompactStore stop/replay/conflict/refusals, continue-repause epoch). Re-points:
compact_causality_test.go unresolved rows now expect StateDecisionRequired (comment records
the routing move); escalatedCompactAuthorityFixture resolves the pause through the
production DecideCompactStore stop so every consumer still holds a persisted escalated
authority.

Projection + CLI (tasks 5-6): RED observed — undefined TargetStatusResult.DecisionQuestion /
CompactDecisionQuestion (build failed). GREEN: projection coupling test (question present
IFF decision_required; gone after decide-stop/continue; invocations bound to the current
revision), stop code review_decision_required with collapse exemption + per-state case,
narration statement registered (vocabulary-clean, names both --decision twins), stop
invariant classification caller-continuable, decision-v1 schema + fixture byte-round-trip
(live type decode → re-marshal byte-equal), RunReviewDecide stop/continue/replay/refusals,
verb-only decide registry row + facade dispatch.

Regression (task 7): go test ./internal/reviewtransaction/ → ok; go test -timeout 40m
./internal/cli/... → ok; gofmt -l clean on all changed files; go vet ./... clean;
go build ./... ok. Consequential re-points the full suite surfaced (same class as the two
named ones): internal/cli escalatedRecoveryProjectionFixture now resolves the pause via
decide-stop before recover; 5 new decide refusals name their `gentle-ai` resolution and the
structural invariants carry refusal:by-design markers to satisfy the refusal ratchet;
narration statement reworded to keep `revision` inside a code span (vocabulary ban).

Docs (task 8): rows added to BOTH stop-continuation tables (docs/review-integration.md,
internal/assets/skills/_shared/review-ledger-contract.md) with the decide continuation and
no Terminal marker (caller-continuable, matching the invariant classification); no new docs
surface invented.

Pause-journal attribution (parent question): the engine-authored pause entry uses actor
"review/complete-review" — the exact operation string the committing Replace journals in the
store trace, which is this repo's only system-authority attribution convention. Decide
entries carry the human actor+reason bound to the expected revision. "Journaled before
append" is implemented as: the entry is fully validated and bound to the revision BEFORE the
CAS successor is constructed, and rides the single Replace write — no post-commit journal
mutation exists; the store trace records the operation after commit per #1854.
