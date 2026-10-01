# Task: fix(review) — START rejects before authority when candidate context fails

**Goal:** `gentle-ai review start` must never declare `action: created` when `base_tree` or `candidate_tree` are empty, or when the frozen candidate context fails. Currently there are code paths where authority is created with null/empty values.

**Where:** `internal/reviewtransaction/snapshot.go`, `internal/cli/review_facade.go`, `internal/cli/review_start_context_test.go`

**Why:** Review agents report `base_tree: None` and `candidate_tree: None` in review-state after a successful START. This blocks all subsequent captures with `candidate view is missing or moved`.

**Where (rctx2):** `internal/cli/review_incident.go`, `internal/reviewtransaction/repository_context.go`, `internal/cli/review_repository_context_test.go`

**Why:** `repository_context_unavailable` does not differentiate between authority missing, git trust refusal, and revision stale. Community reporters (#2227, #2411, #2461) cannot distinguish the root causes.

## Status: All tasks done.

## Tasks

### 1. snapshot.go — guards on Build() for empty trees [DONE]

- **Scope:** `internal/reviewtransaction/snapshot.go`
- **What:** Add explicit validation after resolving baseTree/candidateTree: if either is an empty string, return an actionable error
- **Tests:** `internal/reviewtransaction/snapshot_empty_tree_guard_test.go` — 3 test functions covering empty base_ref, whitespace base_ref, empty trees on success, and TargetCurrentChanges with intended-untracked
- **Verification:** All `SnapshotBuilder` tests pass (44 tests, 44s). No regressions.
- **Implementation:** Added defensive guard in `build()` (line ~300) that returns `fmt.Errorf("empty base tree resolved for %s target; review the repository state and rerun with a valid base_ref")` when `strings.TrimSpace(baseTree) == ""`, and similar for candidateTree

### 2. review_facade.go — validate context before runReviewFacadeCompactAtomicStart — DONE

- **Scope:** `internal/cli/review_facade.go`, `internal/cli/review_start_context_test.go`, `internal/cli/review_empty_tree_guard_test.go`, `internal/cli/review_start_empty_tree_guard_test.go`
- **What:** The `renderReviewStartFrozenCandidateContext` block returns `reviewStartContextError` on failure. Added secondary empty-tree guards BEFORE `ValidateLiveSnapshot` and `runReviewFacadeCompactAtomicStart` so blank trees return `reviewStartContextError` before any authority is written.
- **Tests:** `internal/cli/review_empty_tree_guard_test.go` — 3 tests covering empty base tree, empty candidate tree, and authority-not-created-on-empty-tree. `internal/cli/review_start_empty_tree_guard_test.go` — 4 tests covering review-start refusals and normal flow.
- **Verification:** All empty-tree guard tests pass. No authority is created when trees are empty.
- **Implementation:** Added defensive checks in `runReviewFacadeStart` (after context rendering, before `ValidateLiveSnapshot`) that return `reviewStartContextError` when `BaseTree` or `CandidateTree` are empty strings. Guards are secondary to the `Build()` layer guard.
- **CodeRabbit resolution:** Addresses PR 5144 threads: (1) empty-tree checks moved before `ValidateLiveSnapshot`, (2) test exercises empty git output via `gitCommandContext`

### 3. rctx2 sub-codes — DONE

- **Scope:** `internal/cli/review_incident.go`, `internal/reviewtransaction/repository_locator.go`
- **What:** Split `repository_context_unavailable` into three granular codes:
  - `rctx2_binding_unusable` — structurally invalid or unverifiable rctx2 binding (malformed base64, invalid revision format)
  - `rctx2_resolution_failed` — binding was valid when issued but the underlying repository has changed (authority record removed, repository relocated)
  - `rctx2_identity_mismatch` — binding commits to a different repository than the one named by --cwd
- **Tests:** `internal/cli/review_rctx2_error_codes_test.go` — 3 test functions covering malformed handle, absent authority, and invalid revision format
- **Verification:** `TestRctx2GranularErrorCodes` (3 tests, 4s). `TestOpaqueRepositoryContextResolutionNamesDistinctCauses` updated. `TestRepositoryContextCaptureFromUnrelatedCWDClosesOnLastCapture` updated. `TestOpaqueContextErrorsDoNotExposeProviderPaths` updated.
- **Exported types:** `ErrInvalidReviewRepositoryContextV2`, `ReviewRepositoryContextV2ResolutionError`, `ReviewRepositoryContextIdentityError` from `repository_locator.go` to enable `errors.Is`/`errors.As` classification
- **Commit:** `818f8cf` on branch `fix/rctx2-diagnosis-upgrade`

### 4. rctx2 improved diagnostics — DONE

- **Scope:** `internal/cli/review_incident.go`
- **What:** `reviewOpaqueContextCause` includes specific remediation in the message per sub-code
  - `rctx2_binding_unusable`: "the rctx2 binding is structurally invalid" + action to correct and retry
  - `rctx2_resolution_failed`: "the rctx2 binding was valid when issued but the underlying repository has changed" + action to start a fresh native review
  - `rctx2_identity_mismatch`: "the rctx2 binding commits to a different repository" + action to correct and retry
- **Tests:** Covered by `TestRctx2GranularErrorCodes` assertions on `"structurally invalid"`, `"binding was valid when issued"`, and `"start a fresh native review"`
