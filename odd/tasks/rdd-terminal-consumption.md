# Feature: RDD terminal consumption

## Objective
Prevent an unchanged candidate whose approved review was acknowledged and burned from being offered as a fresh review by selectorless STATUS or the Claude Code Stop hook.

## Problem / Why
Issue #4405 documents repeated reviews after terminal acknowledgement. Authority removal left later STATUS unable to distinguish a consumed target from a never-reviewed target. Empty workspace STATUS also displayed a different identity from its derived committed-range START.

## Scope
- Preserve minimal non-authoritative terminal consumption evidence for an acknowledged target.
- Suppress duplicate STATUS/Stop-hook START offers for that exact target.
- Keep changed targets and genuinely new committed ranges reviewable.
- Publish the effective derived-range identity and projection consistently.
- Add acknowledgement, STATUS, and Stop-hook regressions.

## Constraints
- Authority burn remains irreversible. No authority, delivery authorization, or replay token survives in terminal evidence.
- No post-burn STATUS requirement for consumers.
- Repository/worktree and exact target binding remain checked.
- Technical artifacts in English.
- Strict TDD: explicitly activated by the delegation; runner is `go test` for the affected CLI and reviewtransaction packages.
- Delivery strategy: ask-on-risk. Original forecast: under 400 authored changed lines. Implementation plus negative controls exceeds that forecast; the user explicitly made 400 advisory. Tests were retained rather than compressed or omitted.
- No commit, push, or GitHub mutation by the implementation worker.

## Authorized edit surfaces
- `internal/cli/review_stop_hook.go`, `internal/cli/review_stop_hook*_test.go`
- `internal/cli/review_last_event_closure.go`, `internal/cli/review_last_event_closure*_test.go`
- `internal/cli/review_next_transition*.go`
- `internal/cli/review_facade.go`, `internal/cli/review_status_contract.go`
- `internal/reviewtransaction/compact_store*.go`
- `internal/reviewtransaction/compact_burn.go`, `internal/reviewtransaction/compact_burn_test.go`
- `internal/reviewtransaction/target_status*.go`, `internal/reviewtransaction/*terminal*.go`
- This feature document.

## Prerequisites
Parent supplied verified approval for #4405, current-main reproduction on b6308292, and a conflicting-PR audit. Closed/unmerged #3873 retains reusable authority and is not the accepted design. #4515 supplies the existing derived-range behavior. No remote operations were performed by this worker.

## Tasks
- [x] RDD-1: Add behavior-first regressions reproducing post-acknowledgement selectorless STATUS and Stop-hook duplicate START.
- [x] RDD-2: Implement minimal durable consumed-target evidence and route exact unchanged targets to a terminal/silent result without restoring authority.
- [x] RDD-3: Align derived committed-range transition identity/projection and add regression coverage for empty-live-projection cases.
- [x] RDD-4: Run focused tests, gofmt check, and relevant package verification; record evidence and work-unit commits. Native review approved and authority was acknowledged/burned; implementation committed as `d96f4d8e`.

## Acceptance criteria
- Exact acknowledged target gets an authority-free `stop` / `target_already_acknowledged`, not another START.
- Changed workspace targets and new committed ranges remain eligible.
- Stop stays silent across new sessions without a prior reminder or post-burn STATUS.
- Derived STATUS identity/projection and its executable START target agree.
- Existing acknowledgement refusal, concurrent burn, and fresh explicit START contracts remain valid.

## Implementation
- A small versioned tombstone records only schema, repository/worktree digest, target identity, and lineage. It lives outside authority directories and contains neither acknowledgement token nor authority revision.
- The existing acknowledgement locks protect publication before deletion. The tombstone becomes effective only when its authority directory is absent. Publication failure prevents burn; failed deletion leaves evidence inert and pending authority replayable.
- Both the core assessor and the selectorless facade consult exact consumption evidence before offering a fresh target. Stop needs no separate lifecycle change because it follows STATUS.
- Derived committed-range selection now precedes pending-acknowledgement lookup and consumption lookup. Removed the separate transition-only derived identity and its validator exception.
- Direct explicit START remains available; terminal evidence is not an authorization or an authority lockout.

## Observed TDD and verification
1. RED: `go test ./internal/cli -run 'TestNegotiatedStatusReplaysPendingAcknowledgementWithoutLineageSelector|TestReviewStopHookSilentAfterAcknowledgementWithoutPostBurnStatus' -count=1` failed both new expectations: STATUS returned `fresh_target_ready` / `review.start`; Stop emitted a blocking reminder.
2. GREEN: The same command passed after consumption integration (CLI 1.709s).
3. RDD-3 RED: `go test ./internal/cli -run TestNegotiatedStatusDerivedCommittedRangePayloadValidatesAgainstPublishedSchema -count=1` failed because STATUS still projected an empty current-changes target.
4. RDD-3 GREEN: `go test ./internal/cli -run 'TestNegotiatedStatusDerivedCommittedRangePayloadValidatesAgainstPublishedSchema|TestNegotiatedStatusReplaysPendingAcknowledgementWithoutLineageSelector|TestReviewStopHookSilentAfterAcknowledgementWithoutPostBurnStatus' -count=1` passed (CLI 1.995s).
5. TRIANGULATE: `go test ./internal/cli ./internal/reviewtransaction -run 'TestNextTransitionDerivedRangeAcknowledgementStaysTerminal|TestAcknowledgeTerminalConsumption|TestAcknowledgeApprovedCompactAuthorityFailureKeepsPendingAuthority' -count=1` passed (CLI 1.257s; reviewtransaction 0.708s). Covers exact emitted START execution, pending acknowledgement, terminal Stop, new ranges, failed publication/deletion, malformed/foreign evidence, and no authority/token revival.
6. REFACTOR: `go test ./internal/cli ./internal/reviewtransaction -run 'StopHook|Acknowledge|TargetStatus|NextTransition|Negotiated.*(CommittedRange|EmptyCandidate|ReplaysPending)|CompactStoreCreateOrReplayAtomicStart' -count=1` passed (CLI 15.209s; reviewtransaction 42.355s), including ambiguous-range fallback and malformed-transition controls.
7. Final required check: `go test ./internal/cli ./internal/reviewtransaction -run 'StopHook|Acknowledge|TargetStatus|NextTransition' -count=1` passed (CLI 13.543s; reviewtransaction 39.803s).
8. Independent verification initially exposed five candidate-caused contract gaps: missing refusal classifications, narration, terminal classification, documentation, and shipped continuation for `target_already_acknowledged`. Those gaps were corrected without changing lifecycle behavior.
9. Independent corrected-candidate verification passed:
   - `go test ./internal/cli -count=1` (540.491s)
   - `go test ./internal/reviewtransaction -run 'TerminalConsumption|AcknowledgeTerminalConsumption|AcknowledgeApprovedCompactAuthorityFailureKeepsPendingAuthority' -count=1` (0.756s)
   - `go run ./internal/gofmtcheck` (no output)
   - `git diff --check` (no output)
10. The full `internal/reviewtransaction` suite retains macOS `/var` canonical-path/lock-fixture failures that reproduce on the unchanged base and do not touch candidate paths. Focused candidate tests pass.

An intermediate GREEN attempt exposed the contract's old unrelated-target STOP restriction; that validator was updated. A derived-range test initially called the generic decoded-result `Validate()` helper, which cannot reconstruct the private base-commit provenance after JSON decoding. The test now follows existing derived-range coverage: provider-side validation, published-schema validation, and execution of the exact emitted START. Independent decoded-result revalidation remains a limitation, not a weakened validator.

## Independent-verification corrections

This follow-up is limited to the parent-authorized seven files: `internal/cli/review_status_contract.go`, `internal/cli/review_narration.go`, `internal/cli/review_stop_invariant_test.go`, `internal/reviewtransaction/compact_terminal_consumption.go`, `docs/review-integration.md`, `internal/assets/skills/_shared/review-ledger-contract.md`, and this document.

- Added truthful refusal classifications without changing the refusal ratchet: inconsistent provider envelopes/canonical identities require a producer/caller fix; unsafe, malformed, or foreign consumption evidence requires maintainer inspection, not automatic deletion.
- Classified `target_already_acknowledged` as terminal and not a tool fault. Narration and both continuation tables say no further review is required and delivery remains ordinary policy. Explicit START is only an intentional independent review, never an automatic restart.
- Before edits, all five named CLI contract tests failed as reported. After edits, `go test ./internal/cli -run '^(TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign|TestReviewNarrationRegistryCoversEveryStopReasonCode|TestReviewNarrationNamesAUniversalOrBetterExit|TestEveryReviewStopReasonCodeHasADocsContinuation|TestEveryReviewStopReasonCodeHasAShippedContinuation|TestReviewStopInvariantReasonCodesAreClassified)$' -count=1` passed (0.067s). The additional narration-exit test was discovered by the first full run (540.628s); its missing explicit intentional START wording was corrected without weakening any test.
- Final `go test ./internal/cli -count=1` passed (542.502s, 900-second command budget).
- `go run ./internal/gofmtcheck` and `git diff --check` passed.

### Bounded base/environment comparison

Used the existing clean `sdd-preflight-authority` sibling worktree at the identical base `b6308292`; `git status --short` was empty and its `git diff HEAD -- internal/reviewtransaction go.mod go.sum` was empty. No checkout, temporary source copy, or base modification was needed.

1. Candidate `go test ./internal/reviewtransaction -run '^TestAcquireLocalStoreLockCreatesAndReopensWithoutChangingPermissions$' -count=1` passed (0.026s); this control did not reproduce the reported failure.
2. Candidate `go test ./internal/reviewtransaction -run 'Lock|Canonical' -count=1` failed (10.262s) in six tests with `/var` unsafe-component or `not a directory` errors.
3. From the clean base worktree, `go test ./internal/reviewtransaction -run '^(TestMaintenanceLockModesAndRelease|TestMaintenanceLockRejectsSymlinksAndStaleBytesAreNotOwnership|TestEnsureMaintenanceLockPathAcceptsCanonicalAbsolutePath|TestMaintenanceLockHonorsCancellation|TestMaintenanceLockIsReleasedWhenOwnerProcessExits|TestCompactStartLockAcquisitionIsBoundedAndCancellable)$' -count=1` failed in the same six tests with the same errors (0.015s).

Classification: these six representative macOS path/lock failures are base/environment failures, not candidate-caused. The verifier's remaining twelve unnamed failures were not individually rerun or classified. No path/lock implementation or fixtures were edited. RDD-4 remains incomplete pending parent review and commit.

## Progress / remaining checks
- Branch: `fix/rdd-terminal-consumption`; base `main` at `b6308292`.
- Native review `review-31fa0d525c1abf2d` approved the frozen candidate; acknowledgement burned authority at revision `sha256:ba337070c60793a82c4d9a789fcccf935976e6a1d548ea2bbfe6fb7bd88c3d9b`.
- Work-unit commit: `d96f4d8e` (`fix(review): suppress consumed target re-review`). No push, PR, merge, or other GitHub mutation has been performed yet.
- CodeGraph index/tools were unavailable within the delegated scope; inspected named source paths directly.
- Runtime evidence: Go entry-point integration tests with isolated Git repositories and hook payloads. No live Claude Code session or driven bench journey was run; these tests do not claim host-runtime E2E proof.
- Full unfiltered CLI suite now passes; the full reviewtransaction suite was not repeated. Bounded base comparison below confirms six representative path/lock failures predate this candidate.
- RDD mode is parent-reported enabled. No review transaction was started by this worker; independent review and any consent remain parent-owned.
- Rollback boundary: remove consumption publication/lookup and the effective-range projection change together with their tests. Existing authority burn remains independently intact; leftover tombstones are non-authoritative.
- Next: publish the branch, create the issue-linked PR, and merge it under ordinary repository policy after explicit remote authorization.
