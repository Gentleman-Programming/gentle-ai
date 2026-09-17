# 4680 lens context budget (generated summaries + transport-aware bound)

Claim: gentle-ai#4680, issuecomment-5716970034 (2026-09-17), direction by Alan-TheGentleman, reporter lockder.
Branch: fix/4680-lens-context-budget (worktree ~/gentleman/gentle-ai-4680, base origin/main 4816fccb).

## Goal
START must not freeze lineages whose lens context the reviewer transport cannot take.
Change 1 shrinks real candidates (generated files summarized, not materialized).
Change 2 makes the budget honest and early (transport-aware refusal, nothing persisted).

## Tasks
1. Freeze-time generated marking: `generated` field on `ChangedPathManifestEntry`, recognition extends `isGeneratedGoldenPath` to lockfiles (package-lock.json, pnpm-lock.yaml, yarn.lock, go.sum, Cargo.lock), decided at freeze time; verify persisted-fixture identity impact.
2. Summary sections: `reviewLensContextBlock` emits path/status/numstat/blob hashes (no hunks) for generated entries; `reviewLensContextInstructionText` names summarized paths, preserving the no-partial-evidence invariant.
3. Transport-aware budget: per-runtime bound; lens budget = min(MaxFrozenCandidateDiffBytes, runtime bound); `reviewLensContextBudgetProbe` refuses typed `lens_context_budget_exceeded` before authority persists.
4. Tests: review_lens_context_test.go + frozen_candidate_context_test.go; go test ./internal/... full; golangci patterns (errcheck, staticcheck QF1002) pre-checked.
5. RDD review + PR per branch-pr.

## Non-goals
Provider-model context limits on the pi relay side (belongs to #4611).
Removing the 4 MiB Git ceiling (MaxFrozenCandidateDiffBytes stays).
