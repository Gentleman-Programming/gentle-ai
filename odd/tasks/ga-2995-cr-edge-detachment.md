# ga-2995 follow-up: edge-detachment guard across historical disposition surfaces

Continuation of feature #2995 (PR #4899). Original odd/tasks/ga-2995-review-store-exit.md
was not retained in the worktree; full history lives in Engram (session summaries
obs-0f5547caa326e1ca, topic gentle-ai-2995-classification-honesty-forensic-seams) and in
the PR conversation.

## Context

CodeRabbit actionable finding on PR #4899 (review 2026-09-23T07:18Z, unaddressed by
commit 806b2dfa): the fail-closed edge-detachment guard exists only in the exact-selector
derivation path (historicalDispositionPlanForSelector, authority_disposition_plan.go).
Two sibling surfaces still advertise or mint a per-entry historical disposition for an
entry a retained recovery edge still references:

1. historicalDispositionExitLineages (compact_inspect.go) returns every historical entry
   when all diagnostics are historical, without checking report.Edges.
2. historicalAuthorityDispositionPlanRecord (selectorless N=1 gate,
   authority_disposition_plan.go) mints the plan without checking report.Edges; this both
   feeds selectorless `review repair` execution and re-advertises the exit through the
   len(historicalExits)==0 branch of SanctionedCompactRecoveryExits.

Reachable shape: a LOADED successor whose recovery provenance names a now-historical
predecessor. Edges are built from loaded successors only, so the predecessor side can
name a historical entry ("missing predecessor" edge problem). The existing comment "Real
stores cannot produce this shape today" is wrong for the predecessor side.

Principle violated: "this surface can never advertise a continuation that would then
refuse" (SanctionedCompactRecoveryExits doc, compact_inspect.go).

## Tasks

1. [in_progress] RED tests: inspection regressions (N=1 and N=2 edge-referenced) +
   selectorless derivation refusal; update stale guard-test comment.
2. [pending] Implement: exclude edge-referenced lineages in historicalDispositionExitLineages;
   not-eligible in historicalAuthorityDispositionPlanRecord; update 3 stale comments.
3. [pending] GREEN + full package suite + gofmt/vet (delegated to gentle-ai-worker).
4. [done] Work-unit commit 5971f43d + push on fix/2995-review-store-exit (2026-09-23).
5. [done] CodeRabbit reply posted (discussion_r4082914808); maintainer label comment
   posted (issuecomment-5795666701) requesting type:bug + size:exception.
6. [done] Native review: full-branch candidate stopped terminal at
   lens_context_budget_exceeded (preflight, no authority created). User chose the
   increment scope: START baseRef=806b2dfa committed-only -> lineage
   review-843eacd1c1b7fc99, tier medium, 1 lens (review-reliability), 233 lines.
   Approved + acknowledged/burned same day. One informational advisory finding:
   R3-successor-side-guard-coverage (compact_inspect.go:347-351, SUGGESTION) —
   separate later work, no correction opened.

## Evidence

- 2026-09-23, delegated to gentle-ai-worker (TDD): RED captured on all three new tests
  (exit still advertised; selectorless plan minted), then GREEN. Full package
  `go test ./internal/reviewtransaction/ -count=1` ok 319s; gofmt/vet clean.
  Parent re-verified the four key tests focused (0.55s, all PASS) and reviewed the diff.
- Files: compact_inspect.go (+26/-2, per-entry exclusion + shared
  historicalLineageEdgeReferenced helper), authority_disposition_plan.go (+17/-8,
  selectorless gate refusal + 3 comment truths), 2 test files (+121/-5).
- Worker deviations, all accepted: RecoveryInvalidated instead of RecoveryEscalated
  (Validate() requires authorization for escalated; inspectRecoveryCycle precedent),
  vacuity guard in the N=1 test, shared helper instead of duplicated loops.

- 2026-09-23: user re-confirmed size:exception at the grown measured count (2508 lines) instead of splitting the CR follow-up; decision recorded in the PR body (Size decision section, Re-confirmed paragraph).
