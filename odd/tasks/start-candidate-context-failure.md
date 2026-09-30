# Task: fix(review) — START rejects before authority when candidate context fails

**Goal:** `gentle-ai review start` must never declare `action: created` when `base_tree` or `candidate_tree` are empty, or when the frozen candidate context fails. Currently there are code paths where authority is created with null/empty values.

**Where:** `internal/reviewtransaction/snapshot.go`, `internal/cli/review_facade.go`, `internal/cli/review_start_context_test.go`

**Why:** Review agents report `base_tree: None` and `candidate_tree: None` in review-state after a successful START. This blocks all subsequent captures with `candidate view is missing or moved`.

**Where (rctx2):** `internal/cli/review_incident.go`, `internal/reviewtransaction/repository_context.go`, `internal/cli/review_repository_context_test.go`

**Why:** `repository_context_unavailable` does not differentiate between authority missing, git trust refusal, and revision stale. Community reporters (#2227, #2411, #2461) cannot distinguish the root causes.

## Status: Task 1 done. Task 2 in progress.

## Tasks

### 1. snapshot.go — guards on Build() for empty trees [DONE]

- **Scope:** `internal/reviewtransaction/snapshot.go`
- **What:** Add explicit validation after resolving baseTree/candidateTree: if either is an empty string, return an actionable error
- **Tests:** `internal/reviewtransaction/snapshot_empty_tree_guard_test.go` — 3 test functions covering empty base_ref, whitespace base_ref, empty trees on success, and TargetCurrentChanges with intended-untracked
- **Verification:** All `SnapshotBuilder` tests pass (44 tests, 44s). No regressions.
- **Implementation:** Added defensive guard in `build()` (line ~300) that returns `fmt.Errorf("empty base tree resolved for %s target; review the repository state and rerun with a valid base_ref")` when `strings.TrimSpace(baseTree) == ""`, and similar for candidateTree

### 2. review_facade.go — validate context before runReviewFacadeCompactAtomicStart

- **Scope:** `internal/cli/review_facade.go`, `internal/cli/review_start_context_test.go`
- **What:** The `renderReviewStartFrozenCandidateContext` block already returns `reviewStartContextError` on failure, but authority is created in `runReviewFacadeCompactAtomicStart` AFTER. Verify that if context fails, authority is never written.
- **Tests:** `internal/cli/review_start_context_test.go` — add test verifying authority is not created when context fails

### 3. rctx2 sub-codes

- **Scope:** `internal/cli/review_incident.go`, `internal/reviewtransaction/repository_context.go`
- **What:** Split `repository_context_unavailable` into:
  - `authority_unavailable` — compact authority not found for the lineage
  - `git_trust_refused` — `git rev-parse` fails due to trust setup
  - `revision_stale` — revision hash not found in object store
- **Tests:** `internal/cli/review_repository_context_test.go` — test for each new error code

### 4. rctx2 improved diagnostics

- **Scope:** `internal/cli/review_incident.go`
- **What:** `reviewOpaqueContextCause` must include specific remediation in the message per sub-code
- **Tests:** `internal/cli/review_opaque_typed_cause_test.go` — test for full message with remediation
